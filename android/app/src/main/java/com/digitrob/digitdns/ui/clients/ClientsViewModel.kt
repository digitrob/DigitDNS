package com.digitrob.digitdns.ui.clients

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.data.ClientsRepository
import com.digitrob.digitdns.domain.model.Client
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import javax.inject.Inject

data class ClientsUiState(
    val clients: List<Client> = emptyList(),
    val query: String = "",
    val isLoading: Boolean = false,
    val error: String? = null,
    val allowAll: Boolean = false,
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
) : ViewModel() {

    private val _state = MutableStateFlow(ClientsUiState())
    val state: StateFlow<ClientsUiState> = _state.asStateFlow()

    init {
        refresh()
    }

    fun refresh() {
        viewModelScope.launch {
            // Always flag loading: the list screen keys its pull-to-refresh spinner on
            // it, and the old guard (`isLoading` only when empty) made a refresh of a
            // non-empty list never show the spinner — the pull looked dead.
            _state.update { it.copy(isLoading = true) }
            try {
                val clients = clientsRepository.listAll()
                _state.update { it.copy(clients = clients, isLoading = false, error = null) }
            } catch (e: ApiException) {
                _state.update { it.copy(isLoading = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isLoading = false, error = e.message ?: "Could not load clients.") }
            }
        }
    }

    fun onQueryChange(value: String) = _state.update { it.copy(query = value) }
}
