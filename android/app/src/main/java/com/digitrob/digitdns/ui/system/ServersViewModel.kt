package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.data.AuthRepository
import com.digitrob.digitdns.data.ServerSwitchResult
import com.digitrob.digitdns.domain.model.Connection
import com.digitrob.digitdns.domain.model.CredentialMode
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.Job
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.launch
import javax.inject.Inject

/** One saved install as the list renders it. */
data class ServerEdit(
    val label: String,
    val host: String,
    val port: Int,
    val adminPath: String,
    val isHttps: Boolean,
    val credentialMode: CredentialMode,
    val username: String,
    val password: String,
    val apiKey: String,
    val saveCredentials: Boolean,
)

data class ServerRow(
    val id: String,
    val label: String,
    val authority: String,
    val adminPath: String,
    val isHttps: Boolean,
    val credentialMode: CredentialMode,
    val isActive: Boolean,
    val credentialsSaved: Boolean,
    val isOnline: Boolean? = null,
)

/**
 * The one-shot result of a switch the host has to act on.
 *
 * A transport failure is not here: it leaves the previous server active and becomes
 * [ServersUiState.error], which the list can show inline. A ready switch closes the
 * picker; login-required closes it and sends the operator to the prefilled form.
 */
sealed interface ServerSwitchOutcome {
    data object Ready : ServerSwitchOutcome
    data object LoginRequired : ServerSwitchOutcome
}

/** What the Servers view shows, and whether a switch or removal is in flight. */
data class ServersUiState(
    val servers: List<ServerRow> = emptyList(),
    val isBusy: Boolean = false,
    /**
     * The pull-to-refresh indicator's own flag.
     *
     * It used to be bound to [isBusy], which is set by a switch or a removal and is
     * never set by a refresh at all — so the indicator had nothing to animate on and
     * the pull looked dead. The two now say what they mean: one is a pull in
     * progress, the other is a mutation in progress.
     */
    val isRefreshing: Boolean = false,
    /** The row a switch/removal is running for, so only that row shows a spinner. */
    val busyId: String? = null,
    val error: String? = null,
    val switchOutcome: ServerSwitchOutcome? = null,
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

    private var probeJob: Job? = null

    init {
        refresh()
    }

    /**
     * Rebuild the list from storage; cheap enough to call after every mutation.
     *
     * Only a user pull raises [ServersUiState.isRefreshing]. Opening the manager,
     * returning to Settings, and reloading after a switch are background reads and
     * must not flash the pull indicator over data that is already on screen.
     */
    fun refresh(userInitiated: Boolean = false) {
        probeJob?.cancel()
        viewModelScope.launch {
            _state.update { it.copy(isRefreshing = userInitiated && it.servers.isNotEmpty()) }
            val servers = try {
                val active = authRepository.currentConnection()
                val loaded = authRepository.listServers()
                _state.update {
                    it.copy(
                        servers = loaded.map { c ->
                            ServerRow(
                                id = c.id,
                                label = c.label.ifBlank { c.host },
                                authority = c.authority,
                                adminPath = c.adminPath,
                                isHttps = c.isHttps,
                                credentialMode = c.credentialMode,
                                isActive = c.id == active?.id,
                                credentialsSaved = authRepository.hasStoredCredential(c.id, c.credentialMode),
                                isOnline = null,
                            )
                        },
                    )
                }
                loaded
            } finally {
                _state.update { it.copy(isRefreshing = false) }
            }
            probeJob = viewModelScope.launch {
                val results = servers.map { connection ->
                    async { connection.id to authRepository.probeServer(connection) }
                }.awaitAll().toMap()
                _state.update { state ->
                    state.copy(servers = state.servers.map { row ->
                        row.copy(isOnline = results[row.id])
                    })
                }
            }
        }
    }

    /** Make [id] the active install, unless it already is or a switch is running. */
    fun switchTo(id: String) {
        if (_state.value.isBusy) return
        _state.update { it.copy(isBusy = true, busyId = id, error = null, switchOutcome = null) }
        viewModelScope.launch {
            val result = authRepository.switchToServer(id)
            _state.update {
                it.copy(
                    isBusy = false,
                    busyId = null,
                    error = (result as? ServerSwitchResult.Failed)?.message,
                    switchOutcome = when (result) {
                        ServerSwitchResult.Ready -> ServerSwitchOutcome.Ready
                        ServerSwitchResult.LoginRequired -> ServerSwitchOutcome.LoginRequired
                        is ServerSwitchResult.Failed -> null
                    },
                )
            }
            refresh()
        }
    }

    /** The host has navigated or closed its popup; do not replay the switch. */
    fun consumeSwitchOutcome() = _state.update { it.copy(switchOutcome = null) }

    /** Forget [id]: drop its credential and remove it from the list. */
    fun remove(id: String) {
        if (_state.value.isBusy) return
        _state.update { it.copy(isBusy = true, busyId = id, error = null) }
        viewModelScope.launch {
            try {
                authRepository.removeServer(id)
            } finally {
                _state.update { it.copy(isBusy = false, busyId = null) }
            }
            refresh()
        }
    }

    /**
     * Set a new display label for [id]. Display-only, so it never sets [isBusy]: the
     * rows keep staying interactive behind a rename the way a screen for editing text
     * does not need to suspend switching.
     */
    fun rename(id: String, label: String) {
        viewModelScope.launch {
            authRepository.renameServer(id, label)
            refresh()
        }
    }

    /**
     * Save one profile's connection details and any new credential. The row is a UI
     * edit, not a connection attempt, so it never runs while a switch is in flight and
     * it does not block other rows: it simply rewrites storage and then refreshes.
     */
    fun editServer(id: String, edited: ServerEdit) {
        if (_state.value.isBusy) return
        viewModelScope.launch {
            authRepository.updateServer(
                id = id,
                updated = Connection(
                    id = id,
                    host = edited.host,
                    port = edited.port,
                    adminPath = edited.adminPath,
                    isHttps = edited.isHttps,
                    credentialMode = edited.credentialMode,
                    label = edited.label,
                ),
            )
            when {
                !edited.saveCredentials -> authRepository.clearStoredCredential(id, edited.credentialMode)
                edited.credentialMode == CredentialMode.SESSION && edited.username.isNotBlank() && edited.password.isNotBlank() ->
                    authRepository.saveCredentialsFor(id, edited.username, edited.password)
                edited.credentialMode == CredentialMode.API_KEY && edited.apiKey.isNotBlank() ->
                    authRepository.saveApiKeyFor(id, edited.apiKey)
            }
            refresh()
        }
    }
}
