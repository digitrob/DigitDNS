package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.AlertChannelDto
import com.digitrob.digitdns.core.network.AlertDeliveryDto
import com.digitrob.digitdns.core.network.AlertThresholdsDto
import com.digitrob.digitdns.core.network.AlertsDto
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.data.ServerAlertsRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import javax.inject.Inject

data class ServerAlertsUiState(
    val settings: AlertsDto = AlertsDto(),
    val history: List<AlertDeliveryDto> = emptyList(),
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    val isTesting: Boolean = false,
    val error: String? = null,
    val message: String? = null,
)

@HiltViewModel
class ServerAlertsViewModel @Inject constructor(
    private val repository: ServerAlertsRepository,
) : ViewModel() {
    private val _state = MutableStateFlow(ServerAlertsUiState())
    val state: StateFlow<ServerAlertsUiState> = _state.asStateFlow()

    init { load() }

    fun load() {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = true, error = null) }
            try {
                val envelope = repository.loadEnvelope()
                _state.update { it.copy(settings = envelope.alerts, history = envelope.history, isLoading = false) }
            } catch (e: ApiException) {
                _state.update { it.copy(isLoading = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isLoading = false, error = e.message ?: "Could not load alerts.") }
            }
        }
    }

    fun setEnabled(value: Boolean) = _state.update { it.copy(settings = it.settings.copy(enabled = value)) }
    fun setCooldown(value: String) = _state.update {
        it.copy(settings = it.settings.copy(cooldownMinutes = value.filter { c -> c.isDigit() }.toIntOrNull() ?: 15))
    }
    fun setThreshold(transform: (AlertThresholdsDto) -> AlertThresholdsDto) =
        _state.update {
            it.copy(settings = it.settings.copy(thresholds = transform(it.settings.thresholds)))
        }

    fun setEvent(key: String, value: Boolean) = _state.update {
        it.copy(settings = it.settings.copy(events = it.settings.events + (key to value)))
    }
    fun addChannel() = _state.update {
        it.copy(settings = it.settings.copy(channels = it.settings.channels + AlertChannelDto()))
    }
    fun removeChannel(index: Int) = _state.update {
        it.copy(settings = it.settings.copy(channels = it.settings.channels.filterIndexed { i, _ -> i != index }))
    }
    fun updateChannel(index: Int, transform: (AlertChannelDto) -> AlertChannelDto) = _state.update {
        val list = it.settings.channels.toMutableList()
        if (index in list.indices) list[index] = transform(list[index])
        it.copy(settings = it.settings.copy(channels = list))
    }

    fun save() {
        viewModelScope.launch {
            _state.update { it.copy(isSaving = true, error = null) }
            try {
                val saved = repository.save(_state.value.settings)
                _state.update { it.copy(settings = saved, isSaving = false, message = "Alert settings saved.") }
            } catch (e: ApiException) {
                _state.update { it.copy(isSaving = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isSaving = false, error = e.message ?: "Could not save alerts.") }
            }
        }
    }

    fun test() {
        viewModelScope.launch {
            _state.update { it.copy(isTesting = true, error = null) }
            try {
                repository.save(_state.value.settings)
                val result = repository.test()
                val message = if (result.ok) {
                    "Test alert sent."
                } else {
                    result.errors.joinToString(" · ").ifBlank { "No channel accepted the test." }
                }
                _state.update { it.copy(isTesting = false, message = message) }
            } catch (e: ApiException) {
                _state.update { it.copy(isTesting = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isTesting = false, error = e.message ?: "Could not send the test.") }
            }
        }
    }

    fun dismissMessage() = _state.update { it.copy(message = null) }
}
