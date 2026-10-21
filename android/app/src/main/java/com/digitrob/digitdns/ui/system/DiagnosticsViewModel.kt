package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.data.DiagnosticsRepository
import com.digitrob.digitdns.domain.model.DiagnosticReport
import com.digitrob.digitdns.domain.model.DiagnosticTarget
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import javax.inject.Inject

data class DiagnosticsUiState(
    val targets: List<DiagnosticTarget> = emptyList(),
    val report: DiagnosticReport? = null,
    val isLoading: Boolean = false,
    val isRunning: Boolean = false,
    val error: String? = null,
    val newName: String = "",
    val newHost: String = "",
    val adding: Boolean = false,
)

/**
 * Reachability diagnostics. Add/delete both return the refreshed list, so the screen
 * never re-fetches after a mutation — the repository hands back the new state.
 */
@HiltViewModel
class DiagnosticsViewModel @Inject constructor(
    private val repository: DiagnosticsRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(DiagnosticsUiState())
    val state: StateFlow<DiagnosticsUiState> = _state.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = true, error = null) }
            try {
                val targets = repository.targets()
                _state.update { it.copy(targets = targets, isLoading = false) }
            } catch (e: ApiException) {
                _state.update { it.copy(isLoading = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isLoading = false, error = e.message ?: "Could not load targets.") }
            }
        }
    }

    fun onNewNameChange(v: String) = _state.update { it.copy(newName = v) }
    fun onNewHostChange(v: String) = _state.update { it.copy(newHost = v) }

    fun addTarget() {
        val s = _state.value
        if (s.newName.isBlank() || s.newHost.isBlank()) return
        viewModelScope.launch {
            _state.update { it.copy(adding = true, error = null) }
            try {
                val targets = repository.addTarget(s.newName.trim(), s.newHost.trim())
                _state.update { it.copy(targets = targets, adding = false, newName = "", newHost = "") }
            } catch (e: ApiException) {
                _state.update { it.copy(adding = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(adding = false, error = e.message ?: "Could not add target.") }
            }
        }
    }

    fun deleteTarget(host: String) {
        viewModelScope.launch {
            try {
                val targets = repository.deleteTarget(host)
                _state.update { it.copy(targets = targets) }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not delete target.") }
            }
        }
    }

    fun run() {
        viewModelScope.launch {
            _state.update { it.copy(isRunning = true, error = null) }
            try {
                val report = repository.run()
                _state.update { it.copy(report = report, isRunning = false) }
            } catch (e: ApiException) {
                _state.update { it.copy(isRunning = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isRunning = false, error = e.message ?: "Diagnostics run failed.") }
            }
        }
    }
}
