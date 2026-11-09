package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.TransportSettingsDto
import com.digitrob.digitdns.core.network.TransportSettingsSave
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
    val doqEnabled: Boolean = false,
    val doqPort: String = "853",
    val doqAuth: String = "ip",
    val tokenDomain: String = "",
    val tokens: List<String> = emptyList(),
    val newToken: String = "",
    val doh3Listening: Boolean = false,
    val doqListening: Boolean = false,
    val loaded: Boolean = false,
    val isLoading: Boolean = false,
    val isRefreshing: Boolean = false,
    val isSaving: Boolean = false,
    val saved: Boolean = false,
    val error: String? = null,
)

/**
 * The Android parity surface for the web's Client Encrypted Transports card.
 * Both switches are staged and written by Save; DoQ carries its port, auth mode
 * and SNI-token list because a half-configured token mode cannot start.
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
                else it.copy(isLoading = true, isRefreshing = false, error = null, saved = false)
            }
            try {
                val settings = repository.loadTransports()
                _state.update {
                    it.apply(settings).copy(
                        isLoading = false,
                        isRefreshing = false,
                        loaded = true,
                        error = null,
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

    fun setDoh3Enabled(value: Boolean) = _state.update {
        it.copy(doh3Enabled = value, saved = false)
    }

    fun setDoqEnabled(value: Boolean) = _state.update {
        it.copy(doqEnabled = value, saved = false)
    }

    fun setDoqPort(value: String) = _state.update {
        it.copy(doqPort = value.filter(Char::isDigit).take(MAX_PORT_DIGITS), saved = false)
    }

    fun setDoqAuth(value: String) = _state.update {
        it.copy(doqAuth = if (value == "sni-token") "sni-token" else "ip", saved = false)
    }

    fun setTokenDomain(value: String) = _state.update {
        it.copy(tokenDomain = value.trim().lowercase(), saved = false)
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
                    error = null,
                    saved = false,
                )
            }
        }
    }

    fun removeToken(token: String) = _state.update {
        it.copy(tokens = it.tokens - token, saved = false)
    }

    fun save() {
        val current = _state.value
        if (current.isSaving || !current.loaded) return
        val port = current.doqPort.toIntOrNull() ?: NO_PORT
        val problem = when {
            port !in 1..MAX_DOQ_PORT -> "DoQ port must be between 1 and $MAX_DOQ_PORT."
            current.doqAuth == "sni-token" && current.tokenDomain.isBlank() ->
                "Token domain is required for SNI-token mode."
            current.doqAuth == "sni-token" && current.tokens.isEmpty() ->
                "Add at least one token for SNI-token mode."
            else -> null
        }
        if (problem != null) {
            _state.update { it.copy(error = problem) }
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(isSaving = true, saved = false, error = null) }
            try {
                val saved = repository.saveTransports(
                    TransportSettingsSave(
                        doqEnabled = current.doqEnabled,
                        doh3Enabled = current.doh3Enabled,
                        doqPort = port,
                        doqAuth = current.doqAuth,
                        doqTokenDomain = current.tokenDomain,
                        doqTokens = current.tokens,
                    ),
                )
                _state.update {
                    it.apply(saved).copy(isSaving = false, saved = true, error = null)
                }
            } catch (e: ApiException) {
                _state.update { it.copy(isSaving = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update {
                    it.copy(
                        isSaving = false,
                        error = e.message ?: "Could not save the transport settings.",
                    )
                }
            }
        }
    }
}

private fun ClientTransportsUiState.apply(settings: TransportSettingsDto): ClientTransportsUiState = copy(
    doh3Enabled = settings.doh3Enabled,
    doqEnabled = settings.doqEnabled,
    doqPort = settings.doqPort.takeIf { it > 0 }?.toString() ?: "853",
    doqAuth = if (settings.doqAuth == "sni-token") "sni-token" else "ip",
    tokenDomain = settings.doqTokenDomain,
    tokens = settings.doqTokens,
    doh3Listening = settings.doh3Listening,
    doqListening = settings.doqListening,
)
