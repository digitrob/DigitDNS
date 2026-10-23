package com.digitrob.digitdns.ui.clients

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.ClientUpdateRequest
import com.digitrob.digitdns.data.AccessRepository
import com.digitrob.digitdns.data.ClientsRepository
import com.digitrob.digitdns.data.ConfigRepository
import com.digitrob.digitdns.data.ConnectionStore
import com.digitrob.digitdns.domain.model.Client
import com.digitrob.digitdns.domain.model.ClientAction
import com.digitrob.digitdns.domain.model.ClientExpiry
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch
import java.io.IOException
import javax.inject.Inject

data class ClientsUiState(
    val clients: List<Client> = emptyList(),
    val query: String = "",
    /** The first load, when there is nothing to show yet. */
    val isLoading: Boolean = false,
    /** A user pull, which alone may show the pull-to-refresh indicator. */
    val isRefreshing: Boolean = false,
    val error: String? = null,
    val allowAll: Boolean = false,
    /** Origin subscriber links are built on (config's subscription_origin, or the install). */
    val origin: String = "",
    /** The address the bot card prints as the subscriber's primary DNS. */
    val dnsAddress: String = "",
    /** The client a quick action is currently running for, so its row can show a spinner. */
    val busyId: String? = null,
    /** A transient result of a quick action, shown as a toast over the list. */
    val message: String? = null,
) {
    /** The clients after the search box is applied. */
    val filtered: List<Client>
        get() = if (query.isBlank()) {
            clients
        } else {
            val needle = query.trim().lowercase()
            clients.filter { client ->
                client.name.lowercase().contains(needle) ||
                    client.id.lowercase().contains(needle) ||
                    client.allowedIps.any { it.lowercase().contains(needle) }
            }
        }
}

/** Lists and searches clients. Mutations are delegated to the detail screen's VM. */
@HiltViewModel
class ClientsViewModel @Inject constructor(
    private val clientsRepository: ClientsRepository,
    private val configRepository: ConfigRepository,
    private val connectionStore: ConnectionStore,
    private val accessRepository: AccessRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(ClientsUiState())
    val state: StateFlow<ClientsUiState> = _state.asStateFlow()

    /** The in-flight refresh, replaced when a newer one starts. */
    private var refreshJob: Job? = null

    init {
        refresh()
    }

    /**
     * Reload the list and the link origin.
     *
     * Only [userInitiated] refreshes raise [ClientsUiState.isRefreshing]. Background
     * reloads (returning to the tab, closing a popup, a mutation elsewhere) replace the
     * data silently: a spinner appearing while the operator is reading makes the
     * screen look like it reloaded for no reason.
     */
    fun refresh(userInitiated: Boolean = false) {
        refreshJob?.cancel()
        refreshJob = viewModelScope.launch {
            _state.update {
                it.copy(
                    isLoading = it.clients.isEmpty(),
                    isRefreshing = userInitiated && it.clients.isNotEmpty(),
                )
            }
            try {
                val snapshot = clientsRepository.listAll()
                // The links are a convenience, not the list: a config call that fails
                // (an older daemon, a transient 502) must not hide every client. The
                // install's own origin/address are the documented fallback.
                val config = runCatching { configRepository.config() }.getOrNull()
                val connection = runCatching { connectionStore.active() }.getOrNull()
                val origin = config?.subscriptionOrigin?.takeIf { it.isNotBlank() }
                    ?: connection?.publicOrigin.orEmpty()
                val dnsAddress = config?.server?.publicIp
                    ?.takeIf { it.isNotBlank() && it != "127.0.0.1" && it != "0.0.0.0" }
                    ?: connection?.host.orEmpty()
                _state.update {
                    it.copy(
                        clients = snapshot.clients,
                        allowAll = snapshot.allowAll,
                        origin = origin,
                        dnsAddress = dnsAddress,
                        error = null,
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not load clients.") }
            } finally {
                // Cancellation and both failure paths land here, so neither indicator
                // can be left spinning after a refresh that did not finish.
                _state.update { it.copy(isLoading = false, isRefreshing = false) }
            }
        }
    }

    /** Extend one client from the list, the panel's "+30 Days" quick action. */
    fun renew(client: Client, days: Int) {
        if (_state.value.busyId != null) return
        _state.update { it.copy(busyId = client.id, error = null) }
        viewModelScope.launch {
            try {
                clientsRepository.runAction(client.id, ClientAction.Renew(days))
                val snapshot = clientsRepository.listAll()
                _state.update {
                    it.copy(
                        clients = snapshot.clients,
                        allowAll = snapshot.allowAll,
                        busyId = null,
                        message = "Extended ${client.name.ifBlank { client.id }} by $days days.",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(busyId = null, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(busyId = null, error = e.message ?: "Could not renew the client.") }
            }
        }
    }

    /**
     * Clear the expiry on one client, the list's infinity quick action.
     *
     * The API has no "make lifetime" action: the zero timestamp on `expires_at` is the
     * contract, shared with the detail form's blank-expiry path.
     */
    fun makeLifetime(client: Client) {
        if (_state.value.busyId != null || client.expiresAt == null) return
        _state.update { it.copy(busyId = client.id, error = null) }
        viewModelScope.launch {
            try {
                clientsRepository.update(
                    client.id,
                    ClientUpdateRequest(expiresAt = ClientExpiry.NEVER),
                )
                val snapshot = clientsRepository.listAll()
                _state.update {
                    it.copy(
                        clients = snapshot.clients,
                        allowAll = snapshot.allowAll,
                        busyId = null,
                        message = "${client.name.ifBlank { client.id }} now expires never.",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(busyId = null, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(busyId = null, error = e.message ?: "Could not clear the expiry.") }
            }
        }
    }

    /** Delete one client from the list, reached via the row's swipe-to-delete. */
    fun delete(client: Client) {
        if (_state.value.busyId != null) return
        _state.update { it.copy(busyId = client.id, error = null) }
        viewModelScope.launch {
            try {
                clientsRepository.delete(client.id)
                val snapshot = clientsRepository.listAll()
                _state.update {
                    it.copy(
                        clients = snapshot.clients,
                        allowAll = snapshot.allowAll,
                        busyId = null,
                        message = "Deleted ${client.name.ifBlank { client.id }}.",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(busyId = null, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(busyId = null, error = e.message ?: "Could not delete the client.") }
            }
        }
    }

    fun onAccessModeChange(allowAll: Boolean) {
        val previous = _state.value.allowAll
        _state.update { it.copy(allowAll = allowAll) }
        viewModelScope.launch {
            try {
                accessRepository.setMode(allowAll)
            } catch (e: ApiException) {
                _state.update { it.copy(allowAll = previous, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(allowAll = previous, error = e.message ?: "Could not change access mode.") }
            }
        }
    }

    fun dismissMessage() = _state.update { it.copy(message = null) }

    fun onQueryChange(value: String) = _state.update { it.copy(query = value) }
}
