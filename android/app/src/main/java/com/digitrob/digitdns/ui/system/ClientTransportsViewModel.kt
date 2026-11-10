package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.DoqSettingsSave
import com.digitrob.digitdns.core.network.TransportSettingsDto
import com.digitrob.digitdns.data.SettingsRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import javax.inject.Inject

private val TOKEN_LABEL = Regex("^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$")
private const val MAX_DOQ_PORT = 65535
private const val NO_PORT = 0
private const val MAX_PORT_DIGITS = 5

data class ClientTransportsUiState(
    val doh3Enabled: Boolean = false,
    val appliedDoh3Enabled: Boolean = false,
    val doh3Port: Int = 8443,
    val doqEnabled: Boolean = false,
    val appliedDoqEnabled: Boolean = false,
    val doqPort: String = "853",
    val doqAuth: String = "ip",
    val tokenDomain: String = "",
    val tokens: List<String> = emptyList(),
    val newToken: String = "",
    val doqDirty: Boolean = false,
    val doh3Listening: Boolean = false,
    val doqListening: Boolean = false,
    val allowAll: Boolean = false,
    val allowlistClientAccounts: Int = 0,
    val allowlistClientEntries: Int = 0,
    val allowlistResolverEntries: Int = 0,
    val loaded: Boolean = false,
    val isLoading: Boolean = false,
    val isRefreshing: Boolean = false,
    val doh3Saving: Boolean = false,
    val doqSaving: Boolean = false,
    val error: String? = null,
)

/**
 * Android parity for the web's Client Encrypted Transports card. DoH3 applies
 * as soon as its switch moves. DoQ is staged because its port, auth mode and
 * SNI-token list must be valid together, so it keeps a Save DoQ action.
 */
@HiltViewModel
class ClientTransportsViewModel @Inject constructor(
    private val repository: SettingsRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(ClientTransportsUiState())
    val state: StateFlow<ClientTransportsUiState> = _state.asStateFlow()

    init {
        load()
    }

    fun load() {
        val refresh = _state.value.loaded
        viewModelScope.launch {
            _state.update {
                if (refresh) it.copy(isRefreshing = true, error = null)
                else it.copy(isLoading = true, isRefreshing = false, error = null)
            }
            try {
                val settings = repository.loadTransports()
                _state.update {
                    it.apply(settings).copy(
                        isLoading = false,
                        isRefreshing = false,
                        loaded = true,
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(isLoading = false, isRefreshing = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update {
                    it.copy(
                        isLoading = false,
                        isRefreshing = false,
                        error = e.message ?: "Could not load the transport settings.",
                    )
                }
            }
        }
    }

    /** DoH3 is a single switch, so it applies immediately; failure reverts it. */
    fun setDoh3Enabled(value: Boolean) {
        val current = _state.value
        if (current.doh3Saving || current.doqSaving || !current.loaded) return
        _state.update { it.copy(doh3Enabled = value, doh3Saving = true, error = null) }
        viewModelScope.launch {
            try {
                val saved = repository.saveDoh3(value)
                _state.update {
                    it.copy(
                        appliedDoh3Enabled = saved.doh3Enabled,
                        doh3Enabled = saved.doh3Enabled,
                        doh3Port = saved.doh3Port,
                        doh3Listening = saved.doh3Listening,
                        doh3Saving = false,
                        error = saved.lastError.ifBlank { null },
                    )
                }
            } catch (e: ApiException) {
                _state.update {
                    it.copy(
                        doh3Enabled = it.appliedDoh3Enabled,
                        doh3Saving = false,
                        error = e.userMessage,
                    )
                }
            } catch (e: IOException) {
                _state.update {
                    it.copy(
                        doh3Enabled = it.appliedDoh3Enabled,
                        doh3Saving = false,
                        error = e.message ?: "Could not apply the DoH3 setting.",
                    )
                }
            }
        }
    }

    fun setDoqEnabled(value: Boolean) = _state.update {
        it.copy(doqEnabled = value, doqDirty = true, error = null)
    }

    fun setDoqPort(value: String) = _state.update {
        it.copy(
            doqPort = value.filter(Char::isDigit).take(MAX_PORT_DIGITS),
            doqDirty = true,
            error = null,
        )
    }

    fun setDoqAuth(value: String) = _state.update {
        it.copy(
            doqAuth = if (value == "sni-token") "sni-token" else "ip",
            doqDirty = true,
            error = null,
        )
    }

    fun setTokenDomain(value: String) = _state.update {
        it.copy(tokenDomain = value.trim().lowercase(), doqDirty = true, error = null)
    }

    fun setNewToken(value: String) = _state.update {
        it.copy(newToken = value, error = null)
    }

    fun addToken() {
        _state.update { current ->
            val token = current.newToken.trim().lowercase()
            when {
                token.isEmpty() -> current
                !TOKEN_LABEL.matches(token) -> current.copy(error = "A token must be a single DNS label.")
                current.tokens.contains(token) -> current.copy(error = "That token is already listed.")
                else -> current.copy(
                    tokens = current.tokens + token,
                    newToken = "",
                    doqDirty = true,
                    error = null,
                )
            }
        }
    }

    fun removeToken(token: String) = _state.update {
        it.copy(tokens = it.tokens - token, doqDirty = true, error = null)
    }

    /** DoQ is multi-field, so it is committed only by this explicit action. */
    fun saveDoq() {
        val current = _state.value
        if (current.doqSaving || current.doh3Saving || !current.loaded) return
        val port = current.doqPort.toIntOrNull() ?: NO_PORT
        val problem = doqProblem(current, port)
        if (problem != null) {
            _state.update { it.copy(error = problem) }
            return
        }
        _state.update { it.copy(doqSaving = true, error = null) }
        viewModelScope.launch {
            try {
                val saved = repository.saveDoq(
                    DoqSettingsSave(
                        doqEnabled = current.doqEnabled,
                        doqPort = port,
                        doqAuth = current.doqAuth,
                        doqTokenDomain = current.tokenDomain,
                        doqTokens = current.tokens,
                    ),
                )
                _state.update { it.applyDoq(saved).copy(doqSaving = false, doqDirty = false) }
            } catch (e: ApiException) {
                _state.update {
                    it.copy(
                        doqEnabled = it.appliedDoqEnabled,
                        doqSaving = false,
                        doqDirty = true,
                        error = e.userMessage,
                    )
                }
            } catch (e: IOException) {
                _state.update {
                    it.copy(
                        doqEnabled = it.appliedDoqEnabled,
                        doqSaving = false,
                        doqDirty = true,
                        error = e.message ?: "Could not save the DoQ settings.",
                    )
                }
            }
        }
    }

    private fun doqProblem(current: ClientTransportsUiState, port: Int): String? = when {
        port !in 1..MAX_DOQ_PORT -> "DoQ port must be between 1 and $MAX_DOQ_PORT."
        current.doqAuth == "sni-token" && current.tokenDomain.isBlank() ->
            "Token domain is required for SNI-token mode."
        current.doqAuth == "sni-token" && current.tokens.isEmpty() ->
            "Add at least one token for SNI-token mode."
        else -> null
    }
}

private fun ClientTransportsUiState.apply(settings: TransportSettingsDto): ClientTransportsUiState = copy(
    doh3Enabled = settings.doh3Enabled,
    appliedDoh3Enabled = settings.doh3Enabled,
    doh3Port = settings.doh3Port,
    doqEnabled = settings.doqEnabled,
    appliedDoqEnabled = settings.doqEnabled,
    doqPort = settings.doqPort.takeIf { it > 0 }?.toString() ?: "853",
    doqAuth = if (settings.doqAuth == "sni-token") "sni-token" else "ip",
    tokenDomain = settings.doqTokenDomain,
    tokens = settings.doqTokens,
    doqDirty = false,
    doh3Listening = settings.doh3Listening,
    doqListening = settings.doqListening,
    allowAll = settings.ipAllowlist.allowAll,
    allowlistClientAccounts = settings.ipAllowlist.clientAccounts,
    allowlistClientEntries = settings.ipAllowlist.clientEntries,
    allowlistResolverEntries = settings.ipAllowlist.resolverEntries,
    error = settings.lastError.ifBlank { null },
)

private fun ClientTransportsUiState.applyDoq(settings: TransportSettingsDto): ClientTransportsUiState = copy(
    doqEnabled = settings.doqEnabled,
    appliedDoqEnabled = settings.doqEnabled,
    doqPort = settings.doqPort.takeIf { it > 0 }?.toString() ?: "853",
    doqAuth = if (settings.doqAuth == "sni-token") "sni-token" else "ip",
    tokenDomain = settings.doqTokenDomain,
    tokens = settings.doqTokens,
    doqListening = settings.doqListening,
    allowAll = settings.ipAllowlist.allowAll,
    allowlistClientAccounts = settings.ipAllowlist.clientAccounts,
    allowlistClientEntries = settings.ipAllowlist.clientEntries,
    allowlistResolverEntries = settings.ipAllowlist.resolverEntries,
    error = settings.lastError.ifBlank { null },
)
