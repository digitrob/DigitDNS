package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.EngineApi
import com.digitrob.digitdns.core.network.EngineState
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The engine switch is global, but the Android bottom bar lives in every tab.
 * One singleton holds the state so toggling it anywhere updates the bar
 * everywhere without each tab keeping a stale copy.
 */
@Singleton
class EngineStore @Inject constructor(
    private val api: EngineApi,
) {
    private val _enabled = MutableStateFlow<Boolean?>(null)
    val enabled: StateFlow<Boolean?> = _enabled.asStateFlow()

    suspend fun refresh() {
        _enabled.value = api.status().enabled
    }

    suspend fun setEnabled(enabled: Boolean) {
        _enabled.value = api.set(EngineState(enabled)).enabled
    }
}
