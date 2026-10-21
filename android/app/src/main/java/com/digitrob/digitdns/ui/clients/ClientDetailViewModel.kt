package com.digitrob.digitdns.ui.clients

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.ClientCreateRequest
import com.digitrob.digitdns.core.network.ClientUpdateRequest
import com.digitrob.digitdns.data.ClientsRepository
import com.digitrob.digitdns.domain.model.Client
import com.digitrob.digitdns.domain.model.ClientAction
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import javax.inject.Inject

/**
 * Whether the detail screen is editing an existing client or creating a new one.
 * A new client has no id and starts from empty defaults; an existing one loads and
 * can run actions/deletion, which a not-yet-created client cannot.
 */
sealed interface ClientDetailMode {
    data object Create : ClientDetailMode
    data class Edit(val id: String) : ClientDetailMode
}

data class ClientDetailUiState(
    val mode: ClientDetailMode = ClientDetailMode.Create,
    val client: Client? = null,
    val name: String = "",
    val allowedIps: String = "",
    val trafficLimitGb: String = "",
    val resetCycle: String = "",
    val note: String = "",
    val maxDevices: String = "",
    val validityDays: String = "",
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    val error: String? = null,
    val saved: Boolean = false,
    val deleted: Boolean = false,
    /** A transient message for an action result (a new UUID, a secret, etc.). */
    val message: String? = null,
) {
    val isCreate: Boolean get() = mode is ClientDetailMode.Create

    val canSave: Boolean
        get() = !isSaving && name.isNotBlank() && (isCreate || client != null)
}

@HiltViewModel
class ClientDetailViewModel @Inject constructor(
    private val repository: ClientsRepository,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val _state = MutableStateFlow(ClientDetailUiState())
    val state: StateFlow<ClientDetailUiState> = _state.asStateFlow()

    init {
        // The route passes the id as "clientId"; "new" means create. The route
        // convention is shared with the nav graph so both agree on the sentinel.
        val id = savedStateHandle.get<String>(ARG_CLIENT_ID).orEmpty()
        if (id.isBlank() || id == NEW_CLIENT_SENTINEL) {
            _state.update { it.copy(mode = ClientDetailMode.Create) }
        } else {
            _state.update { it.copy(mode = ClientDetailMode.Edit(id)) }
            load(id)
        }
    }

    private fun load(id: String) {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = true, error = null) }
            try {
                val client = repository.get(id)
                _state.update {
                    it.copy(
                        client = client,
                        name = client.name,
                        allowedIps = client.allowedIps.joinToString(", "),
                        trafficLimitGb = client.trafficLimitGb.takeIf { g -> g > 0 }?.toString().orEmpty(),
                        resetCycle = client.trafficResetCycle,
                        note = client.note,
                        maxDevices = client.maxDevices.takeIf { d -> d > 0 }?.toString().orEmpty(),
                        isLoading = false,
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(isLoading = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isLoading = false, error = e.message ?: "Could not load client.") }
            }
        }
    }

    fun onNameChange(value: String) = _state.update { it.copy(name = value, error = null) }
    fun onAllowedIpsChange(value: String) = _state.update { it.copy(allowedIps = value, error = null) }
    fun onTrafficLimitChange(value: String) = _state.update {
        it.copy(trafficLimitGb = value.filter { c -> c.isDigit() || c == '.' }, error = null)
    }
    fun onResetCycleChange(value: String) = _state.update { it.copy(resetCycle = value, error = null) }
    fun onNoteChange(value: String) = _state.update { it.copy(note = value, error = null) }
    fun onMaxDevicesChange(value: String) = _state.update {
        it.copy(maxDevices = value.filter { c -> c.isDigit() }, error = null)
    }
    fun onValidityDaysChange(value: String) = _state.update {
        it.copy(validityDays = value.filter { c -> c.isDigit() }, error = null)
    }

    fun dismissMessage() = _state.update { it.copy(message = null) }

    fun save() {
        val current = _state.value
        if (!current.canSave) return
        _state.update { it.copy(isSaving = true, error = null) }

        viewModelScope.launch {
            try {
                when (val mode = current.mode) {
                    is ClientDetailMode.Create -> create(current)
                    is ClientDetailMode.Edit -> update(mode.id, current)
                }
                _state.update { it.copy(isSaving = false, saved = true) }
            } catch (e: ApiException) {
                _state.update { it.copy(isSaving = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isSaving = false, error = e.message ?: "Could not save.") }
            }
        }
    }

    private suspend fun create(state: ClientDetailUiState) {
        repository.create(
            ClientCreateRequest(
                name = state.name.trim(),
                validityDays = state.validityDays.toIntOrNull() ?: 0,
                allowedIps = parseIpList(state.allowedIps),
                trafficLimitGb = state.trafficLimitGb.toDoubleOrNull() ?: 0.0,
                trafficResetCycle = state.resetCycle,
                note = state.note.trim(),
                maxDevices = state.maxDevices.toIntOrNull() ?: 0,
            ),
        )
    }

    private suspend fun update(id: String, state: ClientDetailUiState) {
        // Only send fields that changed: the PATCH body treats null as "leave alone",
        // so an untouched field must not be sent as an empty value.
        val original = state.client
        repository.update(
            id,
            ClientUpdateRequest(
                name = state.name.trim().takeIf { it != original?.name },
                allowedIps = parseIpList(state.allowedIps).takeIf { it != original?.allowedIps },
                trafficLimitGb = (state.trafficLimitGb.toDoubleOrNull() ?: 0.0)
                    .takeIf { it != original?.trafficLimitGb },
                trafficResetCycle = state.resetCycle.takeIf { it != original?.trafficResetCycle },
                note = state.note.trim().takeIf { it != original?.note },
                maxDevices = (state.maxDevices.toIntOrNull() ?: 0)
                    .takeIf { it != original?.maxDevices },
            ),
        )
    }

    fun runAction(action: ClientAction) {
        val id = (_state.value.mode as? ClientDetailMode.Edit)?.id ?: return
        viewModelScope.launch {
            try {
                val result = repository.runAction(id, action)
                // Re-read so the screen reflects the action's effect (a toggle flips
                // the badge, a renew moves the date) instead of guessing locally.
                val refreshed = repository.get(id)
                _state.update {
                    it.copy(
                        client = refreshed,
                        message = actionMessage(action, result.uuid, result.registerSecret),
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Action failed.") }
            }
        }
    }

    fun delete() {
        val id = (_state.value.mode as? ClientDetailMode.Edit)?.id ?: return
        viewModelScope.launch {
            try {
                repository.delete(id)
                _state.update { it.copy(deleted = true) }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not delete.") }
            }
        }
    }

    private fun actionMessage(action: ClientAction, uuid: String, secret: String): String = when (action) {
        ClientAction.RegenerateUuid -> "New UUID: $uuid"
        ClientAction.RegenerateRegisterSecret -> "New secret: $secret"
        ClientAction.ResetTraffic -> "Traffic counter reset."
        is ClientAction.Renew -> "Extended by ${action.extendDays} days."
        is ClientAction.Toggle -> if (action.enabled) "Client enabled." else "Client disabled."
        is ClientAction.AddIp -> "Added ${action.ip}."
        is ClientAction.RemoveIp -> "Removed ${action.ip}."
    }

    /** Split a comma/space-separated field into a clean list, the panel's own rule. */
    private fun parseIpList(value: String): List<String> =
        value.split(Regex("[\\s,]+")).map { it.trim() }.filter { it.isNotEmpty() }

    companion object {
        const val ARG_CLIENT_ID = "clientId"
        const val NEW_CLIENT_SENTINEL = "new"
    }
}
