package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.data.AuthRepository
import com.digitrob.digitdns.domain.model.CredentialMode
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

/** One saved install as the list renders it. */
data class ServerRow(
    val id: String,
    val label: String,
    val authority: String,
    val adminPath: String,
    val credentialMode: CredentialMode,
    val isActive: Boolean,
    val credentialsSaved: Boolean,
)

/** What the Servers view shows, and whether a switch or removal is in flight. */
data class ServersUiState(
    val servers: List<ServerRow> = emptyList(),
    val isBusy: Boolean = false,
    val error: String? = null,
)

/**
 * The Servers view: the saved installs, which one is active, and the operations that
 * move between them.
 *
 * Switching and removal both change what "the active credential" means, so both route
 * through [AuthRepository] — which reloads the runtime's credential — rather than
 * touching the store directly. A removal that dropped an install while leaving its
 * credential in memory would leave the app presenting a session for a server it no
 * longer lists.
 */
@HiltViewModel
class ServersViewModel @Inject constructor(
    private val authRepository: AuthRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(ServersUiState())
    val state: StateFlow<ServersUiState> = _state.asStateFlow()

    init {
        refresh()
    }

    /** Rebuild the list from storage; cheap enough to call after every mutation. */
    fun refresh() {
        viewModelScope.launch {
            val active = authRepository.currentConnection()
            val servers = authRepository.listServers()
            _state.update {
                it.copy(
                    servers = servers.map { c ->
                        ServerRow(
                            id = c.id,
                            label = c.label.ifBlank { c.host },
                            authority = c.authority,
                            adminPath = c.adminPath,
                            credentialMode = c.credentialMode,
                            isActive = c.id == active?.id,
                            credentialsSaved = authRepository.hasSavedCredentials(c.id),
                        )
                    },
                )
            }
        }
    }

    /** Make [id] the active install, unless it already is or a switch is running. */
    fun switchTo(id: String) {
        if (_state.value.isBusy) return
        _state.update { it.copy(isBusy = true, error = null) }
        viewModelScope.launch {
            authRepository.setActiveServer(id)
            _state.update { it.copy(isBusy = false) }
            refresh()
        }
    }

    /** Forget [id]: drop its credential and remove it from the list. */
    fun remove(id: String) {
        if (_state.value.isBusy) return
        _state.update { it.copy(isBusy = true, error = null) }
        viewModelScope.launch {
            authRepository.removeServer(id)
            _state.update { it.copy(isBusy = false) }
            refresh()
        }
    }
}
