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
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.channels.ReceiveChannel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.firstOrNull
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import okhttp3.sse.EventSource
import timber.log.Timber
import java.io.IOException
import java.util.ArrayDeque
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import javax.inject.Inject
import javax.inject.Singleton

private const val BASE_BACKOFF_MS = 1_000L
private const val MAX_BACKOFF_MS = 30_000L
private const val MAX_BACKOFF_SHIFT = 5

/** A stalled credential retries slowly: no point storming a refused session. */
private const val AUTH_BACKOFF_MS = 60_000L

/** Exponential backoff capped at [MAX_BACKOFF_MS]: 1s, 2s, 4s, … 30s. */
private fun backoffMillis(attempt: Int): Long {
    val base = BASE_BACKOFF_MS shl (attempt - 1).coerceIn(0, MAX_BACKOFF_SHIFT)
    return base.coerceAtMost(MAX_BACKOFF_MS)
}

/**
 * Sleep for [millis], but return true immediately when [wake] is signalled. The
 * caller resets its attempt counter on a wake so the next failure starts a
 * fresh 1 s backoff rather than continuing a 30 s one.
 */
private suspend fun waitBackoff(wake: ReceiveChannel<Unit>, millis: Long): Boolean =
    withTimeoutOrNull(millis) { wake.receive() } != null

/**
 * The auth-failure branch of the reconnect loop, kept out of the loop so the
 * when/while nesting stays readable: try the silent re-login, then park in
 * AUTH_FAILED and back off slowly.
 */
private suspend fun handleAuthFailure(
    attempt: Int,
    refresh: suspend () -> Boolean,
    wake: ReceiveChannel<Unit>,
    onStalled: () -> Unit,
): Int {
    if (refresh()) return 0
    onStalled()
    return if (waitBackoff(wake, AUTH_BACKOFF_MS)) 0 else attempt + 1
}

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

    // The socket can deliver hundreds of frames a second. Replacing the whole UI list
    // for every frame allocated a new 200-entry list and woke every collector at wire
    // speed; this ring is published at most [UI_PUBLISH_INTERVAL_MS] so Compose sees
    // four coherent frames a second. Notification listeners still see every event
    // immediately in [publish].
    private val bufferLock = Any()
    private val bufferedQueries = ArrayDeque<QueryEvent>(BUFFER_LIMIT)
    private var bufferDirty = false
    private var bufferGeneration = 0L
    private var flushJob: Job? = null

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
        synchronized(bufferLock) {
            bufferGeneration++
            flushJob?.cancel()
            flushJob = null
            bufferedQueries.clear()
            bufferDirty = false
        }
        _queries.value = emptyList()
    }

    /**
     * Reopen the socket so the server re-sends its history snapshot and the buffer
     * is re-seeded. Used by the Live Query Log's manual refresh: the stream is
     * already live, but a pull or a Reload button should visibly re-anchor the
     * feed. Cancelling the current socket drives the reconnect loop's next pass.
     */
    fun reconnect() {
        if (reconnectRequested) return
        reconnectRequested = true
        retryWake.trySend(Unit)
        stream?.cancel()
        closeSignal.complete(Unit)
    }

    /**
     * Called when the app returns to the foreground. A process that was suspended
     * can wake with a dead socket and a retry loop still inside its backoff, which
     * is exactly the "Reconnecting" badge the operator sees after leaving the app
     * idle. Refresh the socket immediately instead of waiting out the backoff.
     *
     * The reconnect is unconditional once the stream has been open for a while:
     * an idle SSE connection produces no data frames, so there is no positive
     * signal that the socket survived Doze. A fresh ticket is cheap and the
     * server's history snapshot re-seeds the log.
     */
    fun onForeground() {
        if (consumers.get() <= 0) return
        val status = _status.value
        val stale = openedAtNanos == 0L ||
            System.nanoTime() - openedAtNanos >= FOREGROUND_RECONNECT_AFTER_NANOS
        if (status == StreamStatus.LIVE && !stale) return
        // Returning from Doze should not flash Connecting when the app already
        // had a working socket: the install is reachable, and the refresh below
        // either re-opens the stream or drops back to Retrying on a real failure.
        if (openedAtNanos != 0L &&
            (status == StreamStatus.RETRYING || status == StreamStatus.CONNECTING)
        ) {
            _status.value = StreamStatus.LIVE
        }
        Timber.i("foreground: refreshing live stream (status=%s)", status)
        retryWake.trySend(Unit)
        stream?.cancel()
        closeSignal.complete(Unit)
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
            when (current) {
                StreamStatus.IDLE, StreamStatus.CONNECTING -> _status.value = StreamStatus.LIVE
                StreamStatus.RETRYING -> {
                    // A successful authenticated call proves the install is
                    // reachable. Wake the loop, but throttle it: the dashboard
                    // polls every second, and a genuine SSE outage must still
                    // respect the backoff rather than become a retry storm.
                    val now = System.nanoTime()
                    if (now - lastAttemptAtNanos >= REACHABILITY_RETRY_AFTER_NANOS) {
                        // The install answered an authenticated call, so the
                        // badge is Online even while the SSE socket is being
                        // re-opened in the background.
                        _status.value = StreamStatus.LIVE
                        retryWake.trySend(Unit)
                    }
                }
                else -> Unit
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
                    attempt = handleAuthFailure(
                        attempt = attempt,
                        refresh = sessionRefresher::refresh,
                        wake = retryWake,
                        onStalled = { _status.value = StreamStatus.AUTH_FAILED },
                    )
                }

                OpenOutcome.RETRYABLE_FAIL -> {
                    attempt++
                    _status.value = StreamStatus.RETRYING
                    attempt = if (waitBackoff(retryWake, backoffMillis(attempt))) 0 else attempt
                }
            }
        }
        teardown()
    }

    /** What one connect attempt resolved to, so the loop can react precisely. */
    private enum class OpenOutcome { OPENED, RETRYABLE_FAIL, AUTH_FAILED }

    @Volatile private var opened = false
    @Volatile private var retryAllowed = true

    /** Set when a manual refresh asks the socket to reopen and re-seed history. */
    @Volatile private var reconnectRequested = false

    /** Completed by onClosed, reconnect() or teardown() to wake the reconnect loop. */
    @Volatile private var closeSignal = CompletableDeferred<Unit>()

    /**
     * Wakes the reconnect loop out of an exponential backoff. Android suspends a
     * backgrounded process, so by the time the app returns the socket may be dead
     * and the loop may be sleeping for up to 30 s. A successful REST call or a
     * foreground transition proves the install is reachable and should not wait
     * out a backoff that was earned while the network was asleep.
     */
    private val retryWake = Channel<Unit>(Channel.CONFLATED)

    /** When the current socket opened, used to refresh a long-idle connection on foreground. */
    @Volatile private var openedAtNanos = 0L

    /** When the last connect attempt started, to throttle reachability-triggered wakes. */
    @Volatile private var lastAttemptAtNanos = 0L

    private suspend fun openOnce(): OpenOutcome {
        // A manual reconnect re-enters through a stream that was already live. Do not
        // flash "Connecting" for that; the feed is already anchored, so the badge only
        // changes when the next real attempt fails.
        val wasLive = _status.value == StreamStatus.LIVE
        if (!wasLive) {
            _status.value = StreamStatus.CONNECTING
        }
        lastAttemptAtNanos = System.nanoTime()
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
        val signal = CompletableDeferred<Unit>()
        closeSignal = signal

        val listener = object : LiveStreamClient.Listener {
            override fun onOpen() {
                opened = true
            }

            override fun onEvent(frame: SseFrame) = handleFrame(frame)

            override fun onClosed(retryable: Boolean, reason: String?) {
                retryAllowed = retryable
                if (reason != null) Timber.i("live stream closed: %s", reason)
                latch.countDown()
                signal.complete(Unit)
            }
        }

        stream?.cancel()
        stream = streamClient.open(connection.streamUrl(ticket), listener)
        // The listener runs on OkHttp's threads; block this IO coroutine until the
        // connection resolves, with a bound so a silent failure cannot hang forever.
        latch.await(CONNECT_LATCH_SECONDS, TimeUnit.SECONDS)
        if (opened) {
            retryAllowed = true
            openedAtNanos = System.nanoTime()
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
        // onClosed, reconnect() and teardown() complete this signal, so the loop
        // sleeps until something actually happens instead of polling every 500 ms.
        closeSignal.await()
        if (reconnectRequested) {
            reconnectRequested = false
            Timber.i("live stream reconnect requested; reopening")
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
        val seeded = events.asReversed().take(BUFFER_LIMIT)
        synchronized(bufferLock) {
            bufferGeneration++
            flushJob?.cancel()
            flushJob = null
            bufferedQueries.clear()
            seeded.forEach { bufferedQueries.addLast(it) }
            bufferDirty = false
        }
        _queries.value = seeded
    }

    private fun publish(event: QueryEvent) {
        synchronized(bufferLock) {
            bufferedQueries.addFirst(event)
            while (bufferedQueries.size > BUFFER_LIMIT) bufferedQueries.removeLast()
            bufferDirty = true
        }
        queryListeners.forEach { runCatching { it(event) } }
        scheduleFlush()
    }

    /**
     * Coalesce UI updates. The first event starts a short window; further events join
     * the same snapshot. The generation check keeps a publish that was already in
     * flight from overwriting a clear or a fresh history seed.
     */
    private fun scheduleFlush() {
        synchronized(bufferLock) {
            if (flushJob?.isActive == true) return
            flushJob = scope.launch {
                delay(UI_PUBLISH_INTERVAL_MS)
                val snapshot: List<QueryEvent>
                val generation: Long
                synchronized(bufferLock) {
                    bufferDirty = false
                    generation = bufferGeneration
                    snapshot = bufferedQueries.toList()
                }
                synchronized(bufferLock) {
                    if (generation == bufferGeneration) _queries.value = snapshot
                    flushJob = null
                    if (!bufferDirty) return@launch
                }
                scheduleFlush()
            }
        }
    }

    private fun teardown() {
        runJob = null
        boundConnectionId = null
        stream?.cancel()
        stream = null
        closeSignal.complete(Unit)
        _status.value = StreamStatus.STOPPED
    }

    private companion object {
        /** The ring the log renders. Matches the panel's recent-log depth. */
        const val BUFFER_LIMIT = 200

        const val CONNECT_LATCH_SECONDS = 20L

        /**
         * A reachability hit may cut a backoff short, but only after this long.
         * The dashboard polls every second; without the throttle a reachable
         * install whose SSE route is broken would be retried every second.
         */
        const val REACHABILITY_RETRY_AFTER_NANOS = 5_000_000_000L

        /**
         * A socket older than this is refreshed on foreground even if its status
         * still says LIVE. An idle SSE stream sends no data frames, so Doze can
         * leave a dead socket looking healthy until the next query.
         */
        const val FOREGROUND_RECONNECT_AFTER_NANOS = 120_000_000_000L

        /** UI coalescing window: enough to batch a burst, short enough to feel live. */
        const val UI_PUBLISH_INTERVAL_MS = 250L
    }
}
