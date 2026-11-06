package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.ProxyLeakSettings
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

data class ProxyLeakSettingsUiState(
    /** The saved, live policy. Defaults on to match the server's recommended policy. */
    val enabled: Boolean = true,
    val recommended: Boolean = true,
    /** False until the server has been read once; the card is not drawn before this. */
    val loaded: Boolean = false,
    /** True only for the first load, before the card exists. */
    val isLoading: Boolean = false,
    /** True for a pull-to-refresh after the card is already on screen. */
    val isRefreshing: Boolean = false,
    val isSaving: Boolean = false,
    val unsupported: Boolean = false,
    val saved: Boolean = false,
    val error: String? = null,
)

/**
 * The server-wide proxy leak guard. Unlike DNSSEC, it is live: the resolver reads
 * an atomic flag, so a successful save applies without a restart. The switch
 * writes immediately instead of waiting for a separate Save button.
 */
@HiltViewModel
class ProxyLeakSettingsViewModel @Inject constructor(
    private val repository: SettingsRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(ProxyLeakSettingsUiState())
    val state: StateFlow<ProxyLeakSettingsUiState> = _state.asStateFlow()

    /** Invalidates an in-flight load when a live toggle supersedes it. */
    private var loadGeneration = 0

    init {
        load()
    }

    fun load() {
        val refresh = _state.value.loaded
        val generation = ++loadGeneration
        viewModelScope.launch {
            _state.update {
                if (refresh) {
                    it.copy(isRefreshing = true, error = null)
                } else {
                    it.copy(isLoading = true, isRefreshing = false, error = null, saved = false)
                }
            }
            try {
                val settings = repository.loadProxyLeak()
                if (generation != loadGeneration) return@launch
                _state.update {
                    it.apply(settings).copy(
                        isLoading = false,
                        isRefreshing = false,
                        unsupported = false,
                        loaded = true,
                    )
                }
            } catch (e: ApiException) {
                if (generation != loadGeneration) return@launch
                _state.update {
                    if (e.httpStatus == HTTP_NOT_FOUND || e.httpStatus == HTTP_UNAVAILABLE) {
                        it.copy(
                            isLoading = false,
                            isRefreshing = false,
                            unsupported = true,
                            loaded = true,
                            error = null,
                        )
                    } else {
                        it.copy(isLoading = false, isRefreshing = false, error = e.userMessage)
                    }
                }
            } catch (e: IOException) {
                if (generation != loadGeneration) return@launch
                _state.update {
                    it.copy(
                        isLoading = false,
                        isRefreshing = false,
                        error = e.message ?: "Could not load proxy leak protection.",
                    )
                }
            }
        }
    }

    /**
     * Save a new value immediately. The switch has already moved in the UI, so a
     * failure restores the previous value instead of leaving a policy that the
     * server never accepted on screen.
     */
    fun setEnabled(value: Boolean) {
        val previous = _state.value.enabled
        if (value == previous) return
        // The toggle is the authoritative action; a refresh that was already in
        // flight must not repaint the old value when it lands.
        loadGeneration++
        _state.update {
            it.copy(
                enabled = value,
                isSaving = true,
                isRefreshing = false,
                error = null,
                saved = false,
            )
        }
        viewModelScope.launch {
            try {
                val settings = repository.saveProxyLeak(value)
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
                        it.copy(isSaving = false, unsupported = true, enabled = previous)
                    } else {
                        it.copy(isSaving = false, enabled = previous, error = e.userMessage)
                    }
                }
            } catch (e: IOException) {
                _state.update {
                    it.copy(
                        isSaving = false,
                        enabled = previous,
                        error = e.message ?: "Could not save proxy leak protection.",
                    )
                }
            }
        }
    }
}

private fun ProxyLeakSettingsUiState.apply(settings: ProxyLeakSettings): ProxyLeakSettingsUiState = copy(
    enabled = settings.enabled,
    recommended = settings.recommended,
)
