package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.AuthState
import com.digitrob.digitdns.data.LiveAlertsPreferences
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class LiveAlertsUiState(
    val enabled: Boolean = false,
    val canEnable: Boolean = true,
    val isLoading: Boolean = true,
)

/**
 * The live-alerts opt-in.
 *
 * The service itself is started and stopped by the screen (it needs a `Context`),
 * but the *decision* lives here and is persisted, so the app knows on next launch
 * whether the operator had it on. [canEnable] is false when there is no credential to
 * authenticate the stream with — starting a stream that cannot authenticate would
 * just reconnect-loop against a 401.
 */
@HiltViewModel
class LiveAlertsViewModel @Inject constructor(
    private val preferences: LiveAlertsPreferences,
    authState: AuthState,
) : ViewModel() {

    private val _state = MutableStateFlow(LiveAlertsUiState())
    val state: StateFlow<LiveAlertsUiState> = _state.asStateFlow()

    init {
        viewModelScope.launch {
            preferences.enabled.collect { enabled ->
                _state.update {
                    it.copy(enabled = enabled, canEnable = authState.hasCredential, isLoading = false)
                }
            }
        }
    }

    /** Persist the new intent. The caller starts or stops the service to match. */
    fun setEnabled(enabled: Boolean) {
        viewModelScope.launch { preferences.setEnabled(enabled) }
    }
}
