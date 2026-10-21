package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
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
    val applying: Boolean = false,
    val flushing: Boolean = false,
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
) : ViewModel() {

    private val _state = MutableStateFlow(MaintenanceUiState())
    val state: StateFlow<MaintenanceUiState> = _state.asStateFlow()

    private var pollJob: Job? = null

    init {
        loadSummary()
    }

    fun loadSummary() {
        viewModelScope.launch {
            try {
                val summary = repository.summary()
                _state.update { it.copy(summary = summary, error = null) }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not load system status.") }
            }
        }
    }

    fun consumeMessage() = _state.update { it.copy(message = null) }

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
