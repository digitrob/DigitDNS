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

    private fun matchesAction(event: QueryEvent): Boolean = when (actionFilter) {
        ActionFilter.All -> true
        ActionFilter.Proxy -> event.action.equals("PROXY", ignoreCase = true)
        ActionFilter.Direct -> event.action.equals("DIRECT", ignoreCase = true)
        ActionFilter.Block -> event.action.equals("BLOCK", ignoreCase = true)
    }
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
     * The same re-anchor, but reached by pulling. The indicator shows briefly so the
     * gesture reads, and a reconnect is forced so the feed is refreshed against the
     * server's own history snapshot while the stream stays live.
     */
    fun refresh() {
        liveStream.start()
        _state.update {
            it.copy(
                isRefreshing = true,
                buffer = liveStream.queries.value,
                paused = false,
                pausedBuffer = null,
                refreshTick = it.refreshTick + 1,
            )
        }
        liveStream.reconnect()
        viewModelScope.launch {
            delay(REFRESH_INDICATOR_MILLIS)
            _state.update { it.copy(isRefreshing = false) }
        }
    }

    /** Apply a quick action to the already-resolved [target] (e.g. `*.target.com`). */
    fun applyQuickAction(action: RouteAction, target: String) {
        if (target.isBlank()) return
        viewModelScope.launch {
            val result = runCatching {
                when (action) {
                    RouteAction.Proxy -> rulesRepository.addDomain(RuleList.Proxied, target)
                        .let { it != null } to "Proxying $target"
                    RouteAction.Block -> rulesRepository.addDomain(RuleList.Blocked, target)
                        .let { it != null } to "Blocking $target"
                    RouteAction.Direct -> rulesRepository.addDomain(RuleList.Direct, target)
                        .let { it != null } to "Routing $target direct"
                    RouteAction.Remove -> rulesRepository.removeDomain(RuleList.Proxied, target)
                        .let { it != null } to "Removed $target from proxied"
                }
            }
            val message = result.fold(
                onSuccess = { (ok, text) -> if (ok) text else "Could not update $target" },
                onFailure = { e ->
                    Timber.w(e, "quick action failed")
                    (e as? ApiException)?.userMessage ?: "Could not update $target"
                },
            )
            _state.update { it.copy(message = message) }
        }
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
