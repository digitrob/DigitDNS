package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.GeoipStatusDto
import com.digitrob.digitdns.data.GeoipRepository
import com.digitrob.digitdns.data.SystemRepository
import com.digitrob.digitdns.data.SystemSummary
import com.digitrob.digitdns.domain.model.UpdateProgressView
import com.digitrob.digitdns.domain.model.UpdateStatus
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import javax.inject.Inject

data class MaintenanceUiState(
    val summary: SystemSummary? = null,
    val update: UpdateStatus? = null,
    val progress: UpdateProgressView? = null,
    val checking: Boolean = false,
    /**
     * The pull-to-refresh indicator's own flag.
     *
     * It used to be bound to [checking], which belongs to an update check and is
     * never set by a summary load — so the pull indicator had nothing to animate on.
     */
    val isRefreshing: Boolean = false,
    val applying: Boolean = false,
    val flushing: Boolean = false,
    /** The server-location database, or null while the status is being read. */
    val geoip: GeoipStatusDto? = null,
    val geoipUpdating: Boolean = false,
    val geoipError: String? = null,
    val error: String? = null,
    val message: String? = null,
)

/**
 * The install's maintenance surface: version/health, update check and apply, cache
 * flush. Applying restarts the process, so the poll below tolerates the connection
 * dropping mid-run — a failed status call during a restart is expected, not an error.
 */
@HiltViewModel
class MaintenanceViewModel @Inject constructor(
    private val repository: SystemRepository,
    private val geoipRepository: GeoipRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(MaintenanceUiState())
    val state: StateFlow<MaintenanceUiState> = _state.asStateFlow()

    private var pollJob: Job? = null

    init {
        loadSummary()
    }

    fun loadSummary() {
        viewModelScope.launch {
            _state.update { it.copy(isRefreshing = true) }
            try {
                val summary = repository.summary()
                _state.update { it.copy(summary = summary, error = null) }
                loadGeoip()
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not load system status.") }
            } finally {
                // Cleared whatever happened, so a failed load cannot leave the pull
                // indicator spinning.
                _state.update { it.copy(isRefreshing = false) }
            }
        }
    }

    fun consumeMessage() = _state.update { it.copy(message = null) }

    /**
     * Reads the GeoIP database's date and the country it resolves for this
     * install. The About card shows it next to the update button; a failure
     * leaves the card in its "checking" state rather than showing an error over
     * the whole page.
     */
    private fun loadGeoip() {
        viewModelScope.launch {
            runCatching { geoipRepository.status() }
                .onSuccess { status -> _state.update { it.copy(geoip = status) } }
                .onFailure { error ->
                    _state.update {
                        it.copy(geoipError = error.message?.takeIf { m -> m.isNotBlank() })
                    }
                }
        }
    }

    /**
     * Downloads a fresh database. The button is disabled while this runs; on
     * success the card redraws with the new file's date, and on failure the
     * server's message is shown in place so the operator knows why.
     */
    fun updateGeoip() {
        if (_state.value.geoipUpdating) return
        viewModelScope.launch {
            _state.update { it.copy(geoipUpdating = true, geoipError = null) }
            try {
                val status = geoipRepository.update()
                _state.update {
                    it.copy(
                        geoip = status,
                        geoipUpdating = false,
                        message = "GeoIP database updated.",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(geoipUpdating = false, geoipError = e.userMessage) }
            } catch (e: IOException) {
                _state.update {
                    it.copy(
                        geoipUpdating = false,
                        geoipError = e.message ?: "Could not update the GeoIP database.",
                    )
                }
            }
        }
    }

    fun checkForUpdate() {
        viewModelScope.launch {
            _state.update { it.copy(checking = true, error = null) }
            try {
                val update = repository.checkUpdate()
                _state.update { it.copy(update = update, checking = false) }
            } catch (e: ApiException) {
                _state.update { it.copy(checking = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(checking = false, error = e.message ?: "Update check failed.") }
            }
        }
    }

    fun applyUpdate() {
        viewModelScope.launch {
            _state.update { it.copy(applying = true, error = null) }
            try {
                repository.applyUpdate()
                _state.update { it.copy(applying = false, message = "Update started. The service will restart.") }
                awaitUpdate()
            } catch (e: ApiException) {
                _state.update { it.copy(applying = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(applying = false, error = e.message ?: "Could not start the update.") }
            }
        }
    }

    fun flushCache() {
        viewModelScope.launch {
            _state.update { it.copy(flushing = true, error = null) }
            try {
                val flushed = repository.flushCache()
                _state.update {
                    it.copy(
                        flushing = false,
                        // The panel's own toast is "DNS cache flushed successfully!"; keep
                        // the phone saying the same thing so support scripts match.
                        message = if (flushed) "DNS cache flushed successfully!" else "Nothing to flush.",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(flushing = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(flushing = false, error = e.message ?: "Could not flush the cache.") }
            }
        }
    }

    /**
     * Poll the update run. A status failure is ignored: during the restart phase the
     * API is unreachable for a moment by design, so a gap in the stream is not fatal.
     * The loop ends on the first terminal phase or when the run reports done.
     */
    private fun awaitUpdate() {
        pollJob?.cancel()
        pollJob = viewModelScope.launch {
            repeat(MAX_POLLS) {
                delay(POLL_INTERVAL_MS)
                val progress = runCatching { repository.updateProgress() }.getOrNull() ?: return@repeat
                _state.update { it.copy(progress = progress) }
                if (!progress.running || progress.phase == "done" || progress.phase == "error") {
                    return@launch
                }
            }
        }
    }

    override fun onCleared() {
        pollJob?.cancel()
        super.onCleared()
    }

    private companion object {
        const val POLL_INTERVAL_MS = 1_500L
        const val MAX_POLLS = 120
    }
}
