package com.digitrob.digitdns.ui.clients

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.ClientCreateRequest
import com.digitrob.digitdns.core.network.ClientUpdateRequest
import com.digitrob.digitdns.data.ClientsRepository
import com.digitrob.digitdns.domain.model.Client
import com.digitrob.digitdns.domain.model.ClientAction
import com.digitrob.digitdns.domain.model.ClientExpiry
import com.digitrob.digitdns.domain.model.ClientLinks
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import java.time.Instant
import java.time.temporal.ChronoUnit
import javax.inject.Inject

/**
 * Whether the detail form is editing an existing client or creating a new one.
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
    val uuid: String = "",
    val allowedIps: String = "",
    val trafficLimitGb: String = "",
    val resetCycle: String = "",
    val note: String = "",
    val maxDevices: String = "",
    /** `yyyy-MM-dd HH:mm`; blank means lifetime. */
    val expiryField: String = "",
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    val error: String? = null,
    val saved: Boolean = false,
    val deleted: Boolean = false,
    /** Set by a successful create, so the popup can show the new credentials. */
    val created: Client? = null,
    /** True while a tray action is in flight, so its buttons can disable. */
    val busyAction: Boolean = false,
    /** A transient message for an action result (a new UUID, a secret, etc.). */
    val message: String? = null,
) {
    val isCreate: Boolean get() = mode is ClientDetailMode.Create

    /** True when the expiry text is blank (lifetime) or a parseable date-time. */
    val expiryValid: Boolean
        get() = expiryField.isBlank() || parseExpiryField(expiryField) != null

    val canSave: Boolean
        get() = !isSaving && name.isNotBlank() && expiryValid && (isCreate || client != null)
}

@HiltViewModel
class ClientDetailViewModel @Inject constructor(
    private val repository: ClientsRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(ClientDetailUiState())
    val state: StateFlow<ClientDetailUiState> = _state.asStateFlow()

    /**
     * The client the form is currently pointed at, if it is editing one.
     *
     * One view model now serves every client the popup opens, so a response can
     * arrive after the form has moved on. Comparing the id it was asked for with
     * this drops that stale response instead of writing it over the new client.
     */
    private val editingId: String? get() = (_state.value.mode as? ClientDetailMode.Edit)?.id

    /**
     * Point the form at a client and load it.
     *
     * The form is a dialog over the client list, so the host names the client
     * instead of a nav route argument. The state is reset first: a client opened
     * after another must never show the previous one's half-typed fields.
     */
    fun open(id: String) {
        _state.update { ClientDetailUiState(mode = ClientDetailMode.Edit(id), isLoading = true) }
        load(id)
    }

    private fun load(id: String) {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = true, error = null) }
            try {
                val client = repository.get(id)
                if (editingId != id) return@launch
                _state.update {
                    it.copy(
                        client = client,
                        name = client.name,
                        uuid = client.uuid,
                        allowedIps = client.allowedIps.joinToString(", "),
                        trafficLimitGb = client.trafficLimitGb.takeIf { g -> g > 0 }?.toString().orEmpty(),
                        resetCycle = client.trafficResetCycle,
                        note = client.note,
                        maxDevices = client.maxDevices.takeIf { d -> d > 0 }?.toString().orEmpty(),
                        expiryField = expiryFieldValue(client.expiresAt),
                        isLoading = false,
                    )
                }
            } catch (e: ApiException) {
                if (editingId != id) return@launch
                _state.update { it.copy(isLoading = false, error = e.userMessage) }
            } catch (e: IOException) {
                if (editingId != id) return@launch
                _state.update { it.copy(isLoading = false, error = e.message ?: "Could not load client.") }
            }
        }
    }

    fun onNameChange(value: String) = _state.update { it.copy(name = value, error = null) }
    fun onUuidChange(value: String) = _state.update { it.copy(uuid = value.trim(), error = null) }
    fun onExpiryChange(value: String) = _state.update { it.copy(expiryField = value, error = null) }
    fun onAllowedIpsChange(value: String) = _state.update { it.copy(allowedIps = value, error = null) }
    fun onTrafficLimitChange(value: String) = _state.update {
        it.copy(trafficLimitGb = value.filter { c -> c.isDigit() || c == '.' }, error = null)
    }
    fun onResetCycleChange(value: String) = _state.update { it.copy(resetCycle = value, error = null) }
    fun onNoteChange(value: String) = _state.update { it.copy(note = value, error = null) }
    fun onMaxDevicesChange(value: String) = _state.update {
        it.copy(maxDevices = value.filter { c -> c.isDigit() }, error = null)
    }
    fun dismissMessage() = _state.update { it.copy(message = null) }

    /** The dialog owns the clipboard; the view model only carries the confirmation. */
    fun noteCopied(label: String) = _state.update { it.copy(message = "$label copied.") }

    /**
     * Return the form to its create defaults.
     *
     * The new-client dialog's view model is scoped to the client list, so it
     * outlives the dialog itself: without this, reopening the dialog would show
     * the previous client's values, a stale error and a "saved" flag that was
     * still set.
     */
    fun resetForNewClient() {
        // This instance only ever backs the new-client popup, so it must always be
        // resettable: a leftover Edit state (or a stale saved/deleted flag) would
        // otherwise flash on the next open. The +30-day prefill matches the web
        // form; clearing it is how the operator asks for a lifetime account.
        _state.value = ClientDetailUiState(
            expiryField = expiryFieldValue(Instant.now().plus(DEFAULT_VALIDITY_DAYS, ChronoUnit.DAYS)),
        )
    }

    fun save() {
        val current = _state.value
        if (!current.canSave) return
        val forId = (current.mode as? ClientDetailMode.Edit)?.id
        _state.update { it.copy(isSaving = true, error = null) }

        viewModelScope.launch {
            try {
                when (val mode = current.mode) {
                    is ClientDetailMode.Create -> {
                        val created = create(current)
                        // The create popup stays open on a success view so the operator
                        // can copy the link, secret and UUID before the list reappears.
                        _state.update { it.copy(isSaving = false, created = created) }
                    }
                    is ClientDetailMode.Edit -> {
                        update(mode.id, current)
                        if (editingId != forId) return@launch
                        _state.update { it.copy(isSaving = false, saved = true) }
                    }
                }
            } catch (e: ApiException) {
                if (editingId != forId) return@launch
                _state.update { it.copy(isSaving = false, error = e.userMessage) }
            } catch (e: IOException) {
                if (editingId != forId) return@launch
                _state.update { it.copy(isSaving = false, error = e.message ?: "Could not save.") }
            }
        }
    }

    private suspend fun create(state: ClientDetailUiState): Client {
        return repository.create(
            ClientCreateRequest(
                name = state.name.trim(),
                // Blank is lifetime; the field is already validated, so a null here
                // means "no expiry" rather than "bad input".
                expiresAt = parseExpiryField(state.expiryField)?.toString(),
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
        val parsedExpiry = parseExpiryField(state.expiryField)
        repository.update(
            id,
            ClientUpdateRequest(
                name = state.name.trim().takeIf { it != original?.name },
                uuid = state.uuid.takeIf { it.isNotBlank() && it != original?.uuid },
                // The zero timestamp clears the date; omitting it would leave the old
                // one in place, which is why "blank = lifetime" has to be sent
                // explicitly rather than as null.
                expiresAt = when {
                    parsedExpiry == original?.expiresAt -> null
                    parsedExpiry == null -> ClientExpiry.NEVER
                    else -> parsedExpiry.toString()
                },
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
        if (_state.value.busyAction) return
        _state.update { it.copy(busyAction = true) }
        viewModelScope.launch {
            try {
                val result = repository.runAction(id, action)
                // Re-read so the screen reflects the action's effect (a toggle flips
                // the badge, a renew moves the date) instead of guessing locally.
                val refreshed = repository.get(id)
                if (editingId != id) return@launch
                _state.update {
                    it.copy(
                        client = refreshed,
                        uuid = refreshed.uuid,
                        expiryField = expiryFieldValue(refreshed.expiresAt),
                        busyAction = false,
                        message = actionMessage(action, result.uuid, result.registerSecret),
                    )
                }
            } catch (e: ApiException) {
                if (editingId != id) return@launch
                _state.update { it.copy(busyAction = false, error = e.userMessage) }
            } catch (e: IOException) {
                if (editingId != id) return@launch
                _state.update { it.copy(busyAction = false, error = e.message ?: "Action failed.") }
            }
        }
    }

    fun delete() {
        val id = (_state.value.mode as? ClientDetailMode.Edit)?.id ?: return
        viewModelScope.launch {
            try {
                repository.delete(id)
                if (editingId != id) return@launch
                _state.update { it.copy(deleted = true) }
            } catch (e: ApiException) {
                if (editingId != id) return@launch
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                if (editingId != id) return@launch
                _state.update { it.copy(error = e.message ?: "Could not delete.") }
            }
        }
    }

    /** Split a comma/space-separated field into a clean list, the panel's own rule. */
    private fun parseIpList(value: String): List<String> =
        value.split(Regex("[\\s,]+")).map { it.trim() }.filter { it.isNotEmpty() }

    companion object {
        /** The key the new-client dialog's view model is stored under. */
        const val CREATE_VIEWMODEL_KEY = "client-create"

        /** The key the edit-client dialog's view model is stored under. */
        const val DETAIL_VIEWMODEL_KEY = "client-detail"

        /** The web panel's default validity for a new client. */
        private const val DEFAULT_VALIDITY_DAYS = 30L
    }
}

/** The toast wording for an action result, kept next to the actions it describes. */
private fun actionMessage(action: ClientAction, uuid: String, secret: String): String = when (action) {
    ClientAction.RegenerateUuid -> "New UUID: $uuid"
    ClientAction.RegenerateRegisterSecret -> "New secret: $secret"
    ClientAction.ResetTraffic -> "Traffic counter reset."
    is ClientAction.Renew -> "Extended by ${action.extendDays} days."
    is ClientAction.Toggle -> if (action.enabled) "Client enabled." else "Client disabled."
    is ClientAction.AddIp -> "Added ${action.ip}."
    is ClientAction.RemoveIp -> "Removed ${action.ip}."
}
