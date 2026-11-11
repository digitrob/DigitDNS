package com.digitrob.digitdns.ui.system

import androidx.fragment.app.FragmentActivity
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.R
import com.digitrob.digitdns.core.security.BiometricAuthenticator
import com.digitrob.digitdns.core.security.BiometricAvailability
import com.digitrob.digitdns.data.AppPreferences
import com.digitrob.digitdns.data.DashboardCard
import com.digitrob.digitdns.data.ThemeMode
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class AppSettingsUiState(
    val themeMode: ThemeMode = ThemeMode.SYSTEM,
    val dashboardOrder: List<DashboardCard> = DashboardCard.defaultOrder,
    val dashboardDisabled: Set<DashboardCard> = emptySet(),
    val biometricEnabled: Boolean = false,
    val biometricAvailability: BiometricAvailability = BiometricAvailability.AVAILABLE,
    val biometricMessage: String? = null,
)

/**
 * App-level preferences: the theme choice and the dashboard card layout. Kept separate
 * from the server settings because these are client-side UI choices that exist even
 * before a server is added.
 */
@HiltViewModel
class AppSettingsViewModel @Inject constructor(
    private val preferences: AppPreferences,
    private val biometric: BiometricAuthenticator,
) : ViewModel() {

    private val _state = MutableStateFlow(AppSettingsUiState())
    val state: StateFlow<AppSettingsUiState> = _state.asStateFlow()

    init {
        viewModelScope.launch {
            preferences.themeMode.collect { mode ->
                _state.update { it.copy(themeMode = mode) }
            }
        }
        viewModelScope.launch {
            preferences.dashboardOrder.collect { order ->
                _state.update { it.copy(dashboardOrder = order) }
            }
        }
        viewModelScope.launch {
            preferences.dashboardDisabled.collect { disabled ->
                _state.update { it.copy(dashboardDisabled = disabled) }
            }
        }
        viewModelScope.launch {
            preferences.biometricEnabled.collect { enabled ->
                _state.update { it.copy(biometricEnabled = enabled) }
            }
        }
        _state.update { it.copy(biometricAvailability = biometric.availability()) }
    }

    fun setThemeMode(mode: ThemeMode) {
        viewModelScope.launch { preferences.setThemeMode(mode) }
    }

    /**
     * Turning the lock on requires a successful prompt first: an operator who
     * cannot authenticate would otherwise enable a lock they cannot open. Turning
     * it off is immediate — they already have the app open.
     */
    fun setBiometricEnabled(activity: FragmentActivity?, enabled: Boolean) {
        if (!enabled) {
            viewModelScope.launch {
                preferences.setBiometricEnabled(false)
                _state.update { it.copy(biometricMessage = null) }
            }
            return
        }
        val availability = biometric.availability()
        if (availability != BiometricAvailability.AVAILABLE || activity == null) {
            _state.update {
                it.copy(
                    biometricAvailability = availability,
                    biometricMessage = if (activity == null) {
                        "The settings screen is not attached to an activity."
                    } else {
                        activity.getString(R.string.settings_biometric_unavailable)
                    },
                )
            }
            return
        }
        viewModelScope.launch {
            val ok = biometric.authenticate(
                activity,
                activity.getString(R.string.biometric_enable_title),
                activity.getString(R.string.biometric_enable_subtitle),
            )
            if (ok) {
                preferences.setBiometricEnabled(true)
                _state.update { it.copy(biometricMessage = null) }
            } else {
                _state.update { it.copy(biometricMessage = activity.getString(R.string.biometric_enable_failed)) }
            }
        }
    }

    /** Show or hide a dashboard card. */
    fun setCardEnabled(card: DashboardCard, enabled: Boolean) {
        viewModelScope.launch { preferences.setDashboardCardEnabled(card, enabled) }
    }

    /** Move the card at [from] to slot [to], as a drag-reorder lands it. */
    fun moveCardTo(from: Int, to: Int) {
        val current = _state.value.dashboardOrder
        if (from == to || from !in current.indices || to !in current.indices) return
        val reordered = current.toMutableList().apply { add(to, removeAt(from)) }
        viewModelScope.launch { preferences.setDashboardOrder(reordered) }
    }

    /** Restore the built-in dashboard card order and visibility. */
    fun resetDashboardLayout() {
        viewModelScope.launch { preferences.resetDashboardLayout() }
        _state.update { it.copy(
            dashboardOrder = DashboardCard.defaultOrder,
            dashboardDisabled = DashboardCard.defaultDisabled,
        ) }
    }}
