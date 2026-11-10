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
private const val MAX_PORT = 65535
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
    val dnscryptEnabled: Boolean = false,
    val dnscryptPort: String = "5443",
    val dnscryptProviderName: String = "",
    val dnscryptStamp: String = "",
    val dnscryptCertNotAfter: Long = 0,
    val dnscryptListening: Boolean = false,
    val dotEnabled: Boolean = true,
    val dotAuth: String = "ip",
    val dotPort: Int = 853,
    val dotTokenDomain: String = "",
    val dotTokens: List<String> = emptyList(),
    val newDotToken: String = "",
    val mtlsEnabled: Boolean = false,
    val mtlsCaPem: String = "",
    val stagedDirty: Boolean = false,
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
    val transportSaving: Boolean = false,
    val error: String? = null,
)

/**
 * Android parity for the web's Client Encrypted Transports card. DoH3 applies
 * as soon as its switch moves. DoQ, DNSCrypt, DoT authentication and mTLS are
 * staged because their fields must be valid together, so they share one save.
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
        if (current.doh3Saving || current.transportSaving || !current.loaded) return
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

    fun setDoqEnabled(value: Boolean) {
        _state.update { it.copy(doqEnabled = value, stagedDirty = true, error = null) }
        autoSaveIfReady()
    }

    fun setDoqPort(value: String) = _state.update {
        it.copy(doqPort = value.digits(), stagedDirty = true, error = null)
    }

    fun setDoqAuth(value: String) {
        _state.update {
            it.copy(
                doqAuth = if (value == "sni-token") "sni-token" else "ip",
                stagedDirty = true,
                error = null,
            )
        }
        autoSaveIfReady()
    }

    fun setTokenDomain(value: String) = _state.update {
        it.copy(tokenDomain = value.trim().lowercase(), stagedDirty = true, error = null)
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
                    stagedDirty = true,
                    error = null,
                )
            }
        }
        autoSaveIfReady()
    }

    fun removeToken(token: String) {
        _state.update { it.copy(tokens = it.tokens - token, stagedDirty = true, error = null) }
        autoSaveIfReady()
    }

    fun setDnscryptEnabled(value: Boolean) {
        _state.update { it.copy(dnscryptEnabled = value, stagedDirty = true, error = null) }
        autoSaveIfReady()
    }

    fun setDnscryptPort(value: String) = _state.update {
        it.copy(dnscryptPort = value.digits(), stagedDirty = true, error = null)
    }

    fun setDnscryptProviderName(value: String) = _state.update {
        it.copy(dnscryptProviderName = value.trim().lowercase(), stagedDirty = true, error = null)
    }

    fun setDotEnabled(value: Boolean) {
        _state.update { it.copy(dotEnabled = value, stagedDirty = true, error = null) }
        autoSaveIfReady()
    }

    fun setDotAuth(value: String) {
        _state.update {
            it.copy(
                dotAuth = if (value == "sni-token") "sni-token" else "ip",
                stagedDirty = true,
                error = null,
            )
        }
        autoSaveIfReady()
    }

    fun setDotTokenDomain(value: String) = _state.update {
        it.copy(dotTokenDomain = value.trim().lowercase(), stagedDirty = true, error = null)
    }

    fun setNewDotToken(value: String) = _state.update {
        it.copy(newDotToken = value, error = null)
    }

    fun addDotToken() {
        _state.update { current ->
            val token = current.newDotToken.trim().lowercase()
            when {
                token.isEmpty() -> current
                !TOKEN_LABEL.matches(token) -> current.copy(error = "A DoT token must be a single DNS label.")
                current.dotTokens.contains(token) -> current.copy(error = "That DoT token is already listed.")
                else -> current.copy(
                    dotTokens = current.dotTokens + token,
                    newDotToken = "",
                    stagedDirty = true,
                    error = null,
                )
            }
        }
        autoSaveIfReady()
    }

    fun removeDotToken(token: String) {
        _state.update { it.copy(dotTokens = it.dotTokens - token, stagedDirty = true, error = null) }
        autoSaveIfReady()
    }

    fun setMtlsEnabled(value: Boolean) {
        _state.update { it.copy(mtlsEnabled = value, stagedDirty = true, error = null) }
        autoSaveIfReady()
    }

    fun setMtlsCaPem(value: String) = _state.update {
        it.copy(mtlsCaPem = value, stagedDirty = true, error = null)
    }

    /** Applies a staged edit when it is complete; incomplete SNI-token or mTLS
     *  state stays local until its required inputs are present. */
    fun autoSaveIfReady() {
        val current = _state.value
        if (!current.loaded || current.transportSaving || current.doh3Saving) return
        if (!current.stagedDirty) return
        val doqPort = current.doqPort.toIntOrNull() ?: return
        val dnscryptPort = current.dnscryptPort.toIntOrNull() ?: return
        if (transportProblem(current, doqPort, dnscryptPort) != null) return
        saveTransports()
    }

    /** The staged transports are multi-field, so they are committed together. */
    fun saveTransports() {
        val current = _state.value
        if (current.transportSaving || current.doh3Saving || !current.loaded) return
        val doqPort = current.doqPort.toIntOrNull() ?: NO_PORT
        val dnscryptPort = current.dnscryptPort.toIntOrNull() ?: NO_PORT
        val problem = transportProblem(current, doqPort, dnscryptPort)
        if (problem != null) {
            _state.update { it.copy(error = problem) }
            return
        }
        _state.update { it.copy(transportSaving = true, error = null) }
        viewModelScope.launch {
            try {
                val saved = repository.saveTransports(
                    TransportSettingsSave(
                        doqEnabled = current.doqEnabled,
                        doqPort = doqPort,
                        doqAuth = current.doqAuth,
                        doqTokenDomain = current.tokenDomain,
                        doqTokens = current.tokens,
                        dnscryptEnabled = current.dnscryptEnabled,
                        dnscryptPort = dnscryptPort,
                        dnscryptProviderName = dnscryptSuffix(current.dnscryptProviderName),
                        dotEnabled = current.dotEnabled,
                        dotAuth = current.dotAuth,
                        dotTokenDomain = current.dotTokenDomain,
                        dotTokens = current.dotTokens,
                        mtlsEnabled = current.mtlsEnabled,
                        mtlsCaPem = current.mtlsCaPem,
                    ),
                )
                _state.update { it.apply(saved).copy(transportSaving = false) }
            } catch (e: ApiException) {
                revertAfterFailure(e.userMessage)
            } catch (e: IOException) {
                revertAfterFailure(e.message ?: "Could not save the transport settings.")
            }
        }
    }

    private fun revertAfterFailure(message: String) {
        // The server rolled back to the previous listeners. Keep the staged
        // values so the operator can correct the failing field; the next edit
        // retries automatically.
        _state.update { it.copy(transportSaving = false, stagedDirty = true, error = message) }
    }

    private fun transportProblem(
        current: ClientTransportsUiState,
        doqPort: Int,
        dnscryptPort: Int,
    ): String? = when {
        doqPort !in 1..MAX_PORT -> "DoQ port must be between 1 and $MAX_PORT."
        dnscryptPort !in 1..MAX_PORT -> "DNSCrypt port must be between 1 and $MAX_PORT."
        current.doqEnabled && current.doqAuth == "sni-token" && current.tokenDomain.isBlank() ->
            "Token domain is required for DoQ SNI-token mode."
        current.doqEnabled && current.doqAuth == "sni-token" && current.tokens.isEmpty() ->
            "Add at least one DoQ token for SNI-token mode."
        current.dotEnabled && current.dotAuth == "sni-token" && current.dotTokenDomain.isBlank() ->
            "Token domain is required for DoT SNI-token mode."
        current.dotEnabled && current.dotAuth == "sni-token" && current.dotTokens.isEmpty() ->
            "Add at least one DoT token for SNI-token mode."
        current.mtlsEnabled && current.mtlsCaPem.isBlank() ->
            "Paste the client CA bundle to enable mTLS."
        else -> null
    }
}

private fun String.digits(): String = filter(Char::isDigit).take(MAX_PORT_DIGITS)

/** The server always prefixes the DNSCrypt provider identity; the field shows
 *  only the part the operator chooses, so it fits on a phone. */
private fun dnscryptSuffix(name: String): String {
    val trimmed = name.trim()
    val prefix = "2.dnscrypt-cert."
    return if (trimmed.startsWith(prefix, ignoreCase = true)) trimmed.substring(prefix.length) else trimmed
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
    newToken = "",
    dnscryptEnabled = settings.dnscryptEnabled,
    dnscryptPort = settings.dnscryptPort.takeIf { it > 0 }?.toString() ?: "5443",
    dnscryptProviderName = dnscryptSuffix(settings.dnscryptProviderName),
    dnscryptStamp = settings.dnscryptStamp,
    dnscryptCertNotAfter = settings.dnscryptCertNotAfter,
    dnscryptListening = settings.dnscryptListening,
    dotEnabled = settings.dotEnabled,
    dotAuth = if (settings.dotAuth == "sni-token") "sni-token" else "ip",
    dotPort = settings.dotPort.takeIf { it > 0 } ?: 853,
    dotTokenDomain = settings.dotTokenDomain,
    dotTokens = settings.dotTokens,
    newDotToken = "",
    mtlsEnabled = settings.mtlsEnabled,
    mtlsCaPem = settings.mtlsCaPem,
    stagedDirty = false,
    doh3Listening = settings.doh3Listening,
    doqListening = settings.doqListening,
    allowAll = settings.ipAllowlist.allowAll,
    allowlistClientAccounts = settings.ipAllowlist.clientAccounts,
    allowlistClientEntries = settings.ipAllowlist.clientEntries,
    allowlistResolverEntries = settings.ipAllowlist.resolverEntries,
    error = settings.lastError.ifBlank { null },
)
