package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.DnssecSettings
import com.digitrob.digitdns.data.SettingsRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import javax.inject.Inject

private const val HTTP_NOT_FOUND = 404
private const val HTTP_UNAVAILABLE = 503

data class DnssecSettingsUiState(
    val enabled: Boolean = false,
    val anchors: String = "",
    val defaultAnchors: List<String> = emptyList(),
    val running: Boolean = false,
    val restartRequired: Boolean = false,
    /** False until the saved policy has been read once; the card is not drawn before this. */
    val loaded: Boolean = false,
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    val unsupported: Boolean = false,
    val saved: Boolean = false,
    val error: String? = null,
)

/**
 * The server-wide DNSSEC policy. It is not an app preference: the validator is
 * installed by the daemon at boot, so this screen reads and writes the saved
 * record through the v3 API and reports when a restart is still required.
 */
@HiltViewModel
class DnssecSettingsViewModel @Inject constructor(
    private val repository: SettingsRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(DnssecSettingsUiState())
    val state: StateFlow<DnssecSettingsUiState> = _state.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = true, error = null, saved = false) }
            try {
                val settings = repository.loadDnssec()
                _state.update {
                    it.apply(settings).copy(isLoading = false, unsupported = false, loaded = true)
                }
            } catch (e: ApiException) {
                _state.update {
                    if (e.httpStatus == HTTP_NOT_FOUND || e.httpStatus == HTTP_UNAVAILABLE) {
                        it.copy(isLoading = false, unsupported = true, loaded = true, error = null)
                    } else {
                        it.copy(isLoading = false, error = e.userMessage)
                    }
                }
            } catch (e: IOException) {
                _state.update {
                    it.copy(isLoading = false, error = e.message ?: "Could not load DNSSEC settings.")
                }
            }
        }
    }

    fun onEnabledChange(value: Boolean) = _state.update {
        it.copy(enabled = value, saved = false, error = null)
    }

    fun onAnchorsChange(value: String) = _state.update {
        it.copy(anchors = value, saved = false, error = null)
    }

    fun save() {
        val s = _state.value
        val anchors = s.anchors
            .split('\n')
            .map { it.trim() }
            .filter { it.isNotEmpty() }
        viewModelScope.launch {
            _state.update { it.copy(isSaving = true, error = null, saved = false) }
            try {
                val settings = repository.saveDnssec(enabled = s.enabled, trustAnchors = anchors)
                _state.update {
                    it.apply(settings).copy(
                        isSaving = false,
                        unsupported = false,
                        loaded = true,
                        error = null,
                        saved = true,
                    )
                }
            } catch (e: ApiException) {
                _state.update {
                    if (e.httpStatus == HTTP_NOT_FOUND || e.httpStatus == HTTP_UNAVAILABLE) {
                        it.copy(isSaving = false, unsupported = true)
                    } else {
                        it.copy(isSaving = false, error = e.userMessage)
                    }
                }
            } catch (e: IOException) {
                _state.update {
                    it.copy(isSaving = false, error = e.message ?: "Could not save DNSSEC settings.")
                }
            }
        }
    }
}

private fun DnssecSettingsUiState.apply(settings: DnssecSettings): DnssecSettingsUiState = copy(
    enabled = settings.enabled,
    anchors = settings.trustAnchors.joinToString("\n"),
    defaultAnchors = settings.defaultAnchors,
    running = settings.running,
    restartRequired = settings.restartRequired,
)
