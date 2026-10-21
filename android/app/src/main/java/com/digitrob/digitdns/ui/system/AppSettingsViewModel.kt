package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
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
)

/**
 * App-level preferences: the theme choice and the dashboard card layout. Kept separate
 * from the server settings because these are client-side UI choices that exist even
 * before a server is added.
 */
@HiltViewModel
class AppSettingsViewModel @Inject constructor(
    private val preferences: AppPreferences,
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
    }

    fun setThemeMode(mode: ThemeMode) {
        viewModelScope.launch { preferences.setThemeMode(mode) }
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
}
