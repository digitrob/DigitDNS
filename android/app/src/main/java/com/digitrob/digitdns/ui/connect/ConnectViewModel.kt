package com.digitrob.digitdns.ui.connect

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.data.ClientsRepository
import com.digitrob.digitdns.data.ConfigRepository
import com.digitrob.digitdns.domain.model.Client
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import javax.inject.Inject

data class ConnectUiState(
    val clients: List<Client> = emptyList(),
    val selectedClientId: String? = null,
    val subscriptionOrigin: String = "",
    val dohUrl: String = "",
    val dnsAddress: String = "",
    /** The raw public IP the Windows/console guide tells the user to enter. */
    val serverIp: String = "",
    val dotHostname: String = "",
    val isLoading: Boolean = true,
    val error: String? = null,
) {
    val selectedClient: Client? get() = clients.firstOrNull { it.id == selectedClientId }

    /**
     * The subscriber's portal link, or empty when it cannot be built. The panel builds
     * it from the config's `subscription_origin` (not the panel's own origin) because
     * the portal may be served from a different host or port — reconstructing it from
     * the panel address would hand out a link nothing answers.
     */
    val subscriptionUrl: String
        get() {
            val token = selectedClient?.token ?: return ""
            val origin = subscriptionOrigin.trimEnd('/')
            return if (origin.isBlank() || token.isBlank()) "" else "$origin/sub/$token"
        }

    val canShowQr: Boolean get() = subscriptionUrl.isNotBlank()
}

/**
 * Backs the Connect tab: pick a client, see the link to hand them and a QR to scan.
 *
 * The QR encodes the subscription URL — the thing an operator actually shares — not
 * a raw DNS address, matching the panel's Connect Guide.
 */
@HiltViewModel
class ConnectViewModel @Inject constructor(
    private val clientsRepository: ClientsRepository,
    private val configRepository: ConfigRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(ConnectUiState())
    val state: StateFlow<ConnectUiState> = _state.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = true, error = null) }
            try {
                val clients = clientsRepository.listAll()
                val config = runCatching { configRepository.config() }.getOrNull()
                _state.update { current ->
                    current.copy(
                        clients = clients,
                        selectedClientId = current.selectedClientId ?: clients.firstOrNull()?.id,
                        subscriptionOrigin = config?.subscriptionOrigin.orEmpty(),
                        dohUrl = config?.dohUrl.orEmpty(),
                        dnsAddress = config?.server?.let { server ->
                            server.domain.ifBlank { server.publicIp }
                        }.orEmpty(),
                        serverIp = config?.server?.publicIp.orEmpty(),
                        dotHostname = config?.tls?.dotHostname.orEmpty(),
                        isLoading = false,
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(isLoading = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isLoading = false, error = e.message ?: "Could not load.") }
            }
        }
    }

    fun selectClient(id: String) = _state.update { it.copy(selectedClientId = id) }
}
