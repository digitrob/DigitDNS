package com.digitrob.digitdns.ui.dashboard

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.LatencySnapshot
import com.digitrob.digitdns.core.network.LiveStats
import com.digitrob.digitdns.core.network.UpstreamStat
import com.digitrob.digitdns.data.AppPreferences
import com.digitrob.digitdns.data.DashboardCard
import com.digitrob.digitdns.data.SettingsRepository
import com.digitrob.digitdns.data.StatsRepository
import com.digitrob.digitdns.data.UpstreamsRepository
import com.digitrob.digitdns.data.stream.LiveQueryStream
import com.digitrob.digitdns.data.stream.StreamStatus
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import timber.log.Timber
import javax.inject.Inject

/** The live window the dashboard shows, derived from the shared stream. */
data class LiveWindow(
    /** Queries in the shared buffer — the recent window, not the all-time counter. */
    val recentQueries: Int = 0,
    /** How many of those were blocked. */
    val blocked: Int = 0,
    val status: StreamStatus = StreamStatus.IDLE,
)

/** The operator's chosen dashboard layout, read from [AppPreferences]. */
data class DashboardLayout(
    val order: List<DashboardCard> = DashboardCard.defaultOrder,
    val disabled: Set<DashboardCard> = DashboardCard.defaultDisabled,
) {
    /** Cards to render, in the saved order, minus the ones switched off. */
    val visible: List<DashboardCard> get() = order.filterNot { it in disabled }
}

data class DashboardUiState(
    val stats: LiveStats? = null,
    val upstreams: List<UpstreamStat> = emptyList(),
    /** A rolling window of `qps` readings, oldest first, for the sparkline. */
    val qpsHistory: List<Double> = emptyList(),
    val live: LiveWindow = LiveWindow(),
    val layout: DashboardLayout = DashboardLayout(),
    /** The install's public address, shown in the dashboard's top box. */
    val serverIp: String = "",
    val isLoading: Boolean = false,
    val isRefreshing: Boolean = false,
    val isBenchmarking: Boolean = false,
    val error: String? = null,
) {
    /** The cache-miss latency distribution, the one that moves when an upstream slows. */
    val latency: LatencySnapshot get() = stats?.latencyUncached ?: LatencySnapshot()
}

/**
 * Feeds the dashboard from three sources: the shared live stream for the connection
 * badge, a `/stats` poll for the server's own counters, and a lighter
 * `/upstreams/stats` poll for the resolver racer.
 *
 * The live stream ([LiveQueryStream]) is held while the dashboard is on screen so the
 * status badge reflects the same socket the log and notifications use. The server's
 * aggregate metrics (CPU, RAM, uptime, totals) are not on the stream, so those still
 * poll `/stats`, at the panel's own three-second cadence. Both stop with the ViewModel.
 */
@HiltViewModel
class DashboardViewModel @Inject constructor(
    private val statsRepository: StatsRepository,
    private val upstreamsRepository: UpstreamsRepository,
    private val settingsRepository: SettingsRepository,
    private val liveStream: LiveQueryStream,
    private val appPreferences: AppPreferences,
) : ViewModel() {

    private val _state = MutableStateFlow(DashboardUiState())
    val state: StateFlow<DashboardUiState> = _state.asStateFlow()

    private var pollJob: Job? = null

    init {
        observeLayout()
        loadServerIp()
        startPolling()
        observeLiveStream()
    }

    /** Mirror the operator's card order and toggles from preferences. */
    private fun observeLayout() {
        viewModelScope.launch {
            combine(appPreferences.dashboardOrder, appPreferences.dashboardDisabled) { order, disabled ->
                DashboardLayout(order = order, disabled = disabled)
            }.collect { layout -> _state.update { it.copy(layout = layout) } }
        }
    }

    /**
     * Hold a stream reference while the dashboard is on screen and derive the live
     * window from its buffer. The window is recomputed on every buffer change; the
     * stream itself self-throttles, so this is the 1 Hz cadence the panel shows.
     */
    private fun observeLiveStream() {
        liveStream.start()
        viewModelScope.launch {
            liveStream.queries.collect { queries ->
                _state.update {
                    it.copy(
                        live = it.live.copy(
                            recentQueries = queries.size,
                            blocked = queries.count { q -> q.action.equals("BLOCK", ignoreCase = true) },
                        ),
                    )
                }
            }
        }
        viewModelScope.launch {
            liveStream.status.collect { status ->
                _state.update { it.copy(live = it.live.copy(status = status)) }
            }
        }
    }

    private fun startPolling() {
        pollJob?.cancel()
        pollJob = viewModelScope.launch {
            while (isActive) {
                refreshOnce(background = true)
                delay(POLL_INTERVAL_MS)
            }
        }
    }

    /** Manual refresh, for pull-to-refresh. */
    fun refresh() {
        viewModelScope.launch {
            loadServerIp()
            refreshOnce(background = false)
        }
    }

    /**
     * Read the install's public address for the dashboard's top box. It comes from the
     * server config, which the 3 s stats tick does not carry, so it is fetched on entry
     * and on a manual refresh only — the address does not change under a poll. A failed
     * read is not worth an error banner; the box simply stays hidden until it succeeds.
     */
    private fun loadServerIp() {
        viewModelScope.launch {
            val ip = runCatching { settingsRepository.load().server?.publicIp }
                .getOrElse {
                    Timber.d(it, "server IP load failed; keeping last value")
                    _state.value.serverIp
                }
            if (!ip.isNullOrBlank()) _state.update { it.copy(serverIp = ip) }
        }
    }

    /**
     * One poll: `/stats` (aggregate counters) and `/upstreams/stats` (racer rows).
     *
     * `background` distinguishes the 3 s tick from a pull-to-refresh: the tick never
     * shows the pull spinner, and a dropped tick is not worth an error banner over
     * numbers already on screen. A manual refresh does surface failure.
     */
    private suspend fun refreshOnce(background: Boolean) {
        if (_state.value.stats == null) {
            _state.update { it.copy(isLoading = true) }
        } else if (!background) {
            _state.update { it.copy(isRefreshing = true) }
        }
        try {
            val stats = statsRepository.live()
            val upstreams = runCatching { upstreamsRepository.stats() }
                .getOrElse {
                    Timber.d(it, "upstream stats poll failed; keeping last rows")
                    _state.value.upstreams
                }
            _state.update {
                it.copy(
                    stats = stats,
                    upstreams = upstreams,
                    qpsHistory = pushQps(it.qpsHistory, stats.qps),
                    isLoading = false,
                    isRefreshing = false,
                    error = null,
                )
            }
        } catch (e: ApiException) {
            _state.update { it.copy(isLoading = false, isRefreshing = false, error = e.userMessage) }
        } catch (e: java.io.IOException) {
            // A dropped poll is not worth an error banner over good numbers already on
            // screen; keep the last reading and try again on the next tick. It is still
            // logged, so a persistently unreachable install leaves a trail.
            Timber.d(e, "stats poll failed; keeping last reading")
            _state.update { it.copy(isLoading = false, isRefreshing = false) }
        }
    }

    /** Kick off a fresh latency benchmark; the racer refreshes as probes land. */
    fun benchmarkUpstreams() {
        if (_state.value.isBenchmarking) return
        viewModelScope.launch {
            _state.update { it.copy(isBenchmarking = true) }
            try {
                upstreamsRepository.benchmark()
                // The server returns probes as they land, so refresh immediately and then
                // poll at a tighter cadence than the dashboard's 3 s tick, stopping early
                // once the racer's latency signature stops changing.
                var lastSignature: String? = null
                repeat(BENCHMARK_FOLLOW_UPS) {
                    delay(BENCHMARK_STEP_MS)
                    val upstreams = runCatching { upstreamsRepository.stats() }.getOrNull()
                    if (upstreams != null) {
                        _state.update { it.copy(upstreams = upstreams) }
                        val signature = upstreams.joinToString("|") { "%.1f".format(it.latencyMs) }
                        if (signature == lastSignature) return@launch
                        lastSignature = signature
                    }
                }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: java.io.IOException) {
                Timber.d(e, "benchmark failed")
            } finally {
                _state.update { it.copy(isBenchmarking = false) }
            }
        }
    }

    /** Add an upstream by address, then refresh the racer. */
    suspend fun addUpstream(address: String): String? = try {
        upstreamsRepository.add(address.trim())
        val upstreams = runCatching { upstreamsRepository.stats() }
            .getOrElse {
                Timber.d(it, "upstream stats poll failed after add")
                _state.value.upstreams
            }
        _state.update { it.copy(upstreams = upstreams) }
        null
    } catch (e: ApiException) {
        e.userMessage
    } catch (e: java.io.IOException) {
        Timber.d(e, "add upstream failed")
        "Could not reach the server."
    }

    /** Remove an upstream by address, then refresh the racer. */
    suspend fun removeUpstream(address: String): String? = try {
        upstreamsRepository.remove(address)
        val upstreams = runCatching { upstreamsRepository.stats() }
            .getOrElse {
                Timber.d(it, "upstream stats poll failed after remove")
                _state.value.upstreams.filterNot { it.address == address }
            }
        _state.update { it.copy(upstreams = upstreams) }
        null
    } catch (e: ApiException) {
        e.userMessage
    } catch (e: java.io.IOException) {
        Timber.d(e, "remove upstream failed")
        "Could not reach the server."
    }

    override fun onCleared() {
        pollJob?.cancel()
        liveStream.stop()
        super.onCleared()
    }

    private companion object {
        /** Three seconds: the panel's own dashboard poll cadence. */
        const val POLL_INTERVAL_MS = 3_000L
        const val QPS_HISTORY_POINTS = 60
        const val BENCHMARK_FOLLOW_UPS = 8
        const val BENCHMARK_STEP_MS = 800L

        /** Append a reading, keeping only the trailing window. */
        fun pushQps(history: List<Double>, value: Double): List<Double> =
            (history + value).takeLast(QPS_HISTORY_POINTS)
    }
}
