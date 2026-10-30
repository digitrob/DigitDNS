package com.digitrob.digitdns.ui.stream

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.notifications.QueryEvent
import com.digitrob.digitdns.data.RulesRepository
import com.digitrob.digitdns.data.stream.LiveQueryStream
import com.digitrob.digitdns.data.stream.StreamStatus
import com.digitrob.digitdns.domain.model.RuleList
import com.digitrob.digitdns.ui.components.RouteAction
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.isActive
import kotlinx.coroutines.delay
import timber.log.Timber
import javax.inject.Inject

/** The action filter the log chip/dropdown selects. */
enum class ActionFilter { All, Proxy, Direct, Block }

/** How the log orders its rows. */
enum class LogSort { Newest, Oldest }

data class LiveQueryLogUiState(
    /** Every buffered query, newest first, unfiltered — the source the view derives from. */
    val buffer: List<QueryEvent> = emptyList(),
    /** The buffer frozen at the moment pause was pressed; shown while paused. */
    val pausedBuffer: List<QueryEvent>? = null,
    val actionFilter: ActionFilter = ActionFilter.All,
    val search: String = "",
    val sort: LogSort = LogSort.Newest,
    val paused: Boolean = false,
    val status: StreamStatus = StreamStatus.IDLE,
    /** Drives the pull-to-refresh indicator while a manual refresh re-anchors the feed. */
    val isRefreshing: Boolean = false,
    /** Increments on reload/refresh so the screen can scroll back to the top. */
    val refreshTick: Int = 0,
    /** A one-shot message for the last quick action, shown as a snackbar. */
    val message: String? = null,
    /** Every configured routing pattern; the quick-action menu preselects from this. */
    val patterns: Set<String> = emptySet(),
) {
    /** The source list respecting pause: frozen while paused, live otherwise. */
    val source: List<QueryEvent> get() = pausedBuffer ?: buffer

    /** The rows after search + action filter + sort, ready to render. */
    val visible: List<QueryEvent>
        get() {
            val needle = search.trim().lowercase()
            val filtered = source.asSequence()
                .filter { matchesAction(it) }
                .filter { needle.isEmpty() || it.domain.lowercase().contains(needle) }
                .toList()
            return if (sort == LogSort.Newest) filtered else filtered.asReversed()
        }

    private fun matchesAction(event: QueryEvent): Boolean =
        actionMatchesFilter(event.action, actionFilter)
}

/**
 * The Live Query Log.
 *
 * It does not open its own socket: it holds a reference on the shared
 * [LiveQueryStream] while it is on screen — the same stream the notification service
 * uses — and derives its rows from the stream's capped buffer. Pause is a *view*
 * concern: the stream keeps buffering underneath, and pause freezes the snapshot the
 * operator is reading so rows do not shift under their finger; resuming drops the
 * freeze and the current buffer shows.
 *
 * Quick actions (Proxy/Block/Direct/Remove) edit the server's rule lists, so they go
 * through [RulesRepository]; the log itself is read-only.
 */
@HiltViewModel
@Suppress("TooGenericExceptionCaught")
class LiveQueryLogViewModel @Inject constructor(
    private val liveStream: LiveQueryStream,
    private val rulesRepository: RulesRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(LiveQueryLogUiState())
    val state: StateFlow<LiveQueryLogUiState> = _state.asStateFlow()

    init {
        liveStream.start()
        viewModelScope.launch {
            while (isActive) {
                try {
                    liveStream.queries.collect { queries ->
                        _state.update { it.copy(buffer = queries) }
                    }
                } catch (e: Exception) {
                    Timber.w(e, "live query collect failed; retrying")
                    delay(COLLECT_RETRY_DELAY_MS)
                }
            }
        }
        viewModelScope.launch {
            while (isActive) {
                try {
                    liveStream.status.collect { status ->
                        _state.update { it.copy(status = status) }
                    }
                } catch (e: Exception) {
                    Timber.w(e, "live status collect failed; retrying")
                    delay(COLLECT_RETRY_DELAY_MS)
                }
            }
        }
        // The quick-action menu preselects the scope a name is already routed with.
        // Best-effort: a failure leaves the menu on Exact, and the action still works.
        viewModelScope.launch {
            runCatching { rulesRepository.load() }
                .onSuccess { lists -> _state.update { it.copy(patterns = lists.managedPatterns) } }
        }
    }

    fun onSearchChange(value: String) = _state.update { it.copy(search = value) }
    fun onActionFilterChange(filter: ActionFilter) = _state.update { it.copy(actionFilter = filter) }

    fun toggleSort() = _state.update {
        it.copy(sort = if (it.sort == LogSort.Newest) LogSort.Oldest else LogSort.Newest)
    }

    /** Freeze or unfreeze the view. Resuming adopts the current live buffer. */
    fun setPaused(paused: Boolean) = _state.update {
        if (paused) it.copy(paused = true, pausedBuffer = it.buffer) else it.copy(paused = false, pausedBuffer = null)
    }

    /** Empty the shared buffer (and the frozen snapshot, if paused). */
    fun clear() {
        liveStream.clearBuffer()
        _state.update { it.copy(buffer = emptyList(), pausedBuffer = if (it.paused) emptyList() else null) }
    }

    fun consumeMessage() = _state.update { it.copy(message = null) }

    /**
     * Re-anchor the feed to the live stream: adopt the current buffer and drop any
     * pause/frozen snapshot. The stream keeps running underneath, so a refresh is a
     * view reset, not a reconnect.
     */
    fun reload() {
        liveStream.start()
        _state.update {
            it.copy(
                buffer = liveStream.queries.value,
                paused = false,
                pausedBuffer = null,
                refreshTick = it.refreshTick + 1,
            )
        }
        liveStream.reconnect()
    }

    /**
     * The same re-anchor as [reload], but reached by pulling: the indicator shows
     * briefly so the gesture reads. Delegating to [reload] guarantees pull-to-refresh
     * does exactly what the Reload button does (re-anchor + buffer + scroll tick).
     */
    fun refresh() {
        _state.update { it.copy(isRefreshing = true) }
        reload()
        viewModelScope.launch {
            delay(REFRESH_INDICATOR_MILLIS)
            _state.update { it.copy(isRefreshing = false) }
        }
    }

    /** Apply a quick action to the already-resolved [targets] (e.g. host + `*.host`). */
    fun applyQuickAction(domain: String, action: RouteAction, targets: List<String>) {
        val unique = targets.distinct()
        if (unique.isEmpty()) return
        val joined = unique.joinToString(" + ")
        viewModelScope.launch {
            val result = runCatching { performQuickAction(domain, action, unique) }
            val message = result.fold(
                onSuccess = { (ok, text) -> if (ok) text else "Could not update $joined" },
                onFailure = { e ->
                    Timber.w(e, "quick action failed")
                    (e as? ApiException)?.userMessage ?: "Could not update $joined"
                },
            )
            _state.update {
                val patterns = if (action == RouteAction.Remove) {
                    it.patterns - unique.toSet()
                } else {
                    it.patterns + unique
                }
                it.copy(message = message, patterns = patterns)
            }
        }
    }

    /** One read-modify-write for the target list, plus the message for the snackbar. */
    private suspend fun performQuickAction(
        domain: String,
        action: RouteAction,
        domains: List<String>,
    ): Pair<Boolean, String> {
        val key = when (action) {
            RouteAction.Proxy -> RuleList.Proxied
            RouteAction.Block -> RuleList.Blocked
            RouteAction.Direct -> RuleList.Direct
            RouteAction.Remove -> null
        }
        val ok = rulesRepository.route(key, rowPatterns(domain), domains)
        val joined = domains.joinToString(" + ")
        val message = when (action) {
            RouteAction.Proxy -> "Proxying $joined"
            RouteAction.Block -> "Blocking $joined"
            RouteAction.Direct -> "Routing $joined direct"
            RouteAction.Remove -> "Removed $joined from proxied"
        }
        return ok to message
    }

    /** What "this row's rule" means: a wildcard row owns itself, a plain host its pair. */
    private fun rowPatterns(domain: String): Set<String> {
        val wildcard = domain.startsWith("*.")
        val base = if (wildcard) domain.removePrefix("*.") else domain
        return if (wildcard) setOf(domain) else setOf(base, "*.$base")
    }

    override fun onCleared() {
        liveStream.stop()
        super.onCleared()
    }

    private companion object {
        /** How long the pull-to-refresh spinner lingers, so the gesture reads visibly. */
        const val REFRESH_INDICATOR_MILLIS = 450L

        /** How long to wait before re-subscribing after a failed collect. */
        const val COLLECT_RETRY_DELAY_MS = 1_000L
    }
}
