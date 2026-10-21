package com.digitrob.digitdns.data.stream

import com.digitrob.digitdns.core.network.AuthApi
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.Reachability
import com.digitrob.digitdns.core.network.SessionRefresher
import com.digitrob.digitdns.core.notifications.LiveStreamClient
import com.digitrob.digitdns.core.notifications.QueryEvent
import com.digitrob.digitdns.core.notifications.SseFrame
import com.digitrob.digitdns.core.notifications.SseFrameParser
import com.digitrob.digitdns.core.notifications.StreamEvent
import com.digitrob.digitdns.data.ConnectionStore
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.firstOrNull
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import okhttp3.sse.EventSource
import timber.log.Timber
import java.io.IOException
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import javax.inject.Inject
import javax.inject.Singleton

/** How the shared stream is doing, for a live indicator. */
enum class StreamStatus {
    IDLE,
    CONNECTING,
    LIVE,
    RETRYING,

    /** The credential was refused and could not be refreshed: the stream is stalled. */
    AUTH_FAILED,
    STOPPED,
}

/**
 * The one SSE connection to the active install, shared by everything that wants it.
 *
 * The server's stream carries *every* resolved query for the install, and each socket
 * costs a ticket plus a held connection; opening one per screen would multiply that
 * for identical data and risk the server's per-subscriber limits. So there is exactly
 * one socket here, fanned out to:
 *  - [queries] — a capped, newest-first ring the Live Query Log renders, and
 *  - [addQueryListener] — a raw feed the notification service filters.
 *
 * **Lifecycle.** [start] and [stop] are reference-counted: the log screen and the
 * notification service each hold a reference while they want the stream, and the
 * socket lives while at least one holds it. This is what lets the log screen stream
 * without the notification opt-in, and the service keep streaming after the screen
 * closes.
 *
 * **Server switches.** The stream is pinned to the active install's id. When the
 * active install changes, the socket is torn down and reopened against the new one —
 * otherwise a switch would leave the operator watching the previous box's traffic.
 *
 * The socket is torn down on [stop] or a non-retryable close (a revoked session); a
 * retryable drop backs off and reconnects. All of this runs on one IO scope so the
 * connect/backoff loop is serialised.
 */
@Singleton
class LiveQueryStream @Inject constructor(
    private val authApi: AuthApi,
    private val connectionStore: ConnectionStore,
    private val streamClient: LiveStreamClient,
    private val parser: SseFrameParser,
    private val sessionRefresher: SessionRefresher,
    private val reachability: Reachability,
) {
    private val _queries = MutableStateFlow<List<QueryEvent>>(emptyList())
    /** The recent query log, newest first, capped at [BUFFER_LIMIT]. */
    val queries: StateFlow<List<QueryEvent>> = _queries.asStateFlow()

    private val _status = MutableStateFlow(StreamStatus.IDLE)
    val status: StateFlow<StreamStatus> = _status.asStateFlow()

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val consumers = AtomicInteger(0)
    private var runJob: Job? = null
    private var stream: EventSource? = null

    private val queryListeners = CopyOnWriteArrayList<(QueryEvent) -> Unit>()

    /** The install id the live socket currently belongs to, for the switch watcher. */
    @Volatile private var boundConnectionId: String? = null

    /**
     * Register interest in the stream. The first caller opens the socket; later ones
     * join the existing one. Callers must pair this with [stop] (e.g. in `onCleared`,
     * or when a foreground service is torn down).
     */
    fun start() {
        if (consumers.incrementAndGet() == 1) {
            runJob = scope.launch { runLoop() }
        }
    }

    /** Release interest. The socket closes when the last consumer leaves. */
    fun stop() {
        if (consumers.decrementAndGet() <= 0) {
            consumers.set(0)
            teardown()
        }
    }

    /** Drop the buffered queries — the log's "clear". Does not touch the socket. */
    fun clearBuffer() {
        _queries.value = emptyList()
    }

    /**
     * Observe raw queries as they arrive, for the notification path. The listener is
     * called on the IO thread that reads the socket. Returns a remover to call when the
     * observer is done.
     */
    fun addQueryListener(listener: (QueryEvent) -> Unit): () -> Unit {
        queryListeners.add(listener)
        return { queryListeners.remove(listener) }
    }

    private suspend fun runLoop() {
        // Follow the active install: a switch tears the socket down and reopens it.
        scope.launch { watchActiveConnection() }
        scope.launch { watchReachability() }
        reconnectLoop()
    }

    /**
     * Show Online as soon as any authed call succeeds, not only once the SSE handshake
     * lands. The stream's own [StreamStatus.LIVE] still wins when it arrives; this only
     * lifts the pre-handshake IDLE/CONNECTING state so a screen opened on a reachable
     * install never reads "Connecting" for the length of a ticket mint plus a connect.
     */
    private suspend fun watchReachability() {
        reachability.hits.collect {
            val current = _status.value
            if (current == StreamStatus.IDLE || current == StreamStatus.CONNECTING) {
                _status.value = StreamStatus.LIVE
            }
        }
    }

    private suspend fun watchActiveConnection() {
        connectionStore.activeId
            .distinctUntilChanged()
            .collect { id ->
                if (id != boundConnectionId) {
                    Timber.i("active install changed; reconnecting live stream")
                    stream?.cancel()
                    stream = null
                }
            }
    }

    private suspend fun reconnectLoop() {
        var attempt = 0
        while (scope.isActive && consumers.get() > 0) {
            when (openOnce()) {
                OpenOutcome.OPENED -> {
                    attempt = 0
                    waitForClose()
                }

                OpenOutcome.AUTH_FAILED -> {
                    // The ticket could not be minted — a lapsed session or a refusal.
                    // Try the one silent re-login the rest of the app uses; only if that
                    // fails do we park in AUTH_FAILED and slow the retry right down, so a
                    // dead credential cannot become a ticket-minting storm.
                    if (sessionRefresher.refresh()) {
                        attempt = 0
                    } else {
                        _status.value = StreamStatus.AUTH_FAILED
                        attempt++
                        delay(AUTH_BACKOFF_MS)
                    }
                }

                OpenOutcome.RETRYABLE_FAIL -> {
                    attempt++
                    _status.value = StreamStatus.RETRYING
                    delay(backoffMillis(attempt))
                }
            }
        }
        teardown()
    }

    /** What one connect attempt resolved to, so the loop can react precisely. */
    private enum class OpenOutcome { OPENED, RETRYABLE_FAIL, AUTH_FAILED }

    @Volatile private var opened = false
    @Volatile private var retryAllowed = true

    private suspend fun openOnce(): OpenOutcome {
        _status.value = StreamStatus.CONNECTING
        val connection = connectionStore.connection.firstOrNull()
        val ticket = mintTicket()
        return when {
            connection == null || ticket == null -> OpenOutcome.RETRYABLE_FAIL
            ticket.isEmpty() -> OpenOutcome.AUTH_FAILED
            else -> connect(connection, ticket)
        }
    }

    /**
     * Mint a one-time stream ticket. Returns the ticket, an empty string when the
     * credential was refused (a verdict the loop answers with a refresh), or null when
     * the mint failed at the transport layer (retryable).
     */
    /**
     * Mint a one-time stream ticket. Returns the ticket string; an **empty** string
     * when the credential was refused (a verdict the loop answers with a session
     * refresh); or **null** when the mint failed at the transport layer (retryable).
     */
    private suspend fun mintTicket(): String? {
        // The two failure shapes are distinct: an ApiException (401/403) is a credential
        // verdict the loop answers with a refresh, while an IOException is a transport
        // blip worth a plain retry. Encode the difference at the source.
        var refused = false
        val response = try {
            authApi.sseTicket()
        } catch (e: ApiException) {
            Timber.i("sse ticket refused with HTTP %d", e.httpStatus)
            refused = true
            null
        } catch (e: IOException) {
            Timber.w(e, "sse ticket mint failed")
            null
        }
        return when {
            response != null -> response.ticket
            refused -> ""
            else -> null
        }
    }

    private suspend fun connect(connection: com.digitrob.digitdns.domain.model.Connection, ticket: String): OpenOutcome {
        boundConnectionId = connection.id
        opened = false
        retryAllowed = true
        val latch = CountDownLatch(1)

        val listener = object : LiveStreamClient.Listener {
            override fun onOpen() {
                opened = true
            }

            override fun onEvent(frame: SseFrame) = handleFrame(frame)

            override fun onClosed(retryable: Boolean, reason: String?) {
                retryAllowed = retryable
                if (reason != null) Timber.i("live stream closed: %s", reason)
                latch.countDown()
            }
        }

        stream?.cancel()
        stream = streamClient.open(connection.streamUrl(ticket), listener)
        // The listener runs on OkHttp's threads; block this IO coroutine until the
        // connection resolves, with a bound so a silent failure cannot hang forever.
        latch.await(CONNECT_LATCH_SECONDS, TimeUnit.SECONDS)
        if (opened) {
            retryAllowed = true
            _status.value = StreamStatus.LIVE
        }
        // Never opened: a 4xx from the stream route (a spent ticket, a revoked session)
        // is non-retryable and maps to the auth stall; a transport failure is retryable.
        return when {
            opened -> OpenOutcome.OPENED
            retryAllowed -> OpenOutcome.RETRYABLE_FAIL
            else -> OpenOutcome.AUTH_FAILED
        }
    }

    private suspend fun waitForClose() {
        // Poll the retry signal: the listener flips retryAllowed when the stream ends,
        // and there is no channel to await it on without extra machinery.
        while (consumers.get() > 0 && retryAllowed) {
            delay(RECONNECT_PROBE_MS)
        }
        if (!retryAllowed) {
            // A refusal (spent ticket, revoked session): do not hammer the server. A
            // refresh is attempted by the loop, which then re-opens on the next pass.
            Timber.i("live stream ended non-retryably; backing off")
        }
    }

    private fun handleFrame(frame: SseFrame) {
        when (val event = parser.parse(frame)) {
            is StreamEvent.History -> seed(event.events)
            is StreamEvent.Query -> publish(event.event)
            null -> Unit
        }
    }

    /** The history snapshot seeds the buffer, newest first, without notifying. */
    private fun seed(events: List<QueryEvent>) {
        _queries.value = events.asReversed().take(BUFFER_LIMIT)
    }

    private fun publish(event: QueryEvent) {
        _queries.value = (listOf(event) + _queries.value).take(BUFFER_LIMIT)
        queryListeners.forEach { runCatching { it(event) } }
    }

    private fun teardown() {
        runJob = null
        boundConnectionId = null
        stream?.cancel()
        stream = null
        _status.value = StreamStatus.STOPPED
    }

    /** Exponential backoff capped at [MAX_BACKOFF_MS]: 1s, 2s, 4s, … 30s. */
    private fun backoffMillis(attempt: Int): Long {
        val base = BASE_BACKOFF_MS shl (attempt - 1).coerceIn(0, MAX_BACKOFF_SHIFT)
        return base.coerceAtMost(MAX_BACKOFF_MS)
    }

    private companion object {
        /** The ring the log renders. Matches the panel's recent-log depth. */
        const val BUFFER_LIMIT = 200

        const val RECONNECT_PROBE_MS = 500L
        const val CONNECT_LATCH_SECONDS = 20L
        const val BASE_BACKOFF_MS = 1_000L
        const val MAX_BACKOFF_MS = 30_000L
        const val MAX_BACKOFF_SHIFT = 5

        /** A stalled credential retries slowly: no point storming a refused session. */
        const val AUTH_BACKOFF_MS = 60_000L
    }
}
