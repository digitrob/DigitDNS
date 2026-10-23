package com.digitrob.digitdns.ui.auth

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.data.AuthRepository
import com.digitrob.digitdns.data.CredentialCheck
import com.digitrob.digitdns.data.LoginResult
import com.digitrob.digitdns.domain.model.Connection
import com.digitrob.digitdns.domain.model.CredentialMode
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

/** RFC 6238 codes are six digits. */
private const val TOTP_CODE_LENGTH = 6

/** Everything the login screen renders. */
data class LoginUiState(
    val host: String = "",
    val port: String = "443",
    val adminPath: String = "",
    val isHttps: Boolean = true,
    /**
     * The install this form edits. Blank means a brand-new server, so a successful
     * login adds a profile; a saved id means an existing one, so the fields are
     * updated in place rather than duplicated.
     */
    val connectionId: String = "",
    val username: String = "",
    val password: String = "",
    val code: String = "",
    val apiKey: String = "",
    val credentialMode: CredentialMode = CredentialMode.SESSION,
    /**
     * The "save credentials" tick. On by default (opt-out): persisting the pair lets
     * the app silently re-mint a session when the server's idle window lapses, which is
     * the behaviour that keeps an operator signed in across restarts. Only meaningful in
     * [CredentialMode.SESSION] — an API key is already stored as the credential itself.
     */
    val saveCredentials: Boolean = true,
    /** True once the server has demanded a TOTP code for these credentials. */
    val needsTwoFactor: Boolean = false,
    val isLoading: Boolean = false,
    val error: String? = null,
    val authenticated: Boolean = false,
    /**
     * Set when the install's certificate is untrusted: the handshake never completed,
     * so no credential was checked. The screen shows the fingerprint and a "trust"
     * action; on confirm the fingerprint is pinned and the attempt is retried.
     */
    val trustPrompt: TrustPrompt? = null,
) {
    val canSubmit: Boolean
        get() = !isLoading && host.isNotBlank() && port.toIntOrNull() in 1..65535 && when (credentialMode) {
            CredentialMode.SESSION -> username.isNotBlank() && password.isNotBlank()
            CredentialMode.API_KEY -> apiKey.isNotBlank()
        }
}

/**
 * A pending trust decision: the certificate [fingerprint] the app saw, and whether it
 * replaced a previously remembered one ([changedFrom]) — a change is a much louder
 * signal than a first contact and the screen says so.
 */
data class TrustPrompt(val fingerprint: String, val changedFrom: String?)

/**
 * Drives the connection + login flow.
 *
 * The screen collects host/port/admin-path/credentials in one form because every one
 * of them is required before a single API call can succeed — there is no "connect
 * first, credentials later" state to expose, since nothing is reachable until the
 * whole connection is known.
 */
@HiltViewModel
class LoginViewModel @Inject constructor(
    private val authRepository: AuthRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(LoginUiState())
    val state: StateFlow<LoginUiState> = _state.asStateFlow()

    /**
     * Prepare the form. Called once by the screen.
     *
     * [addNewServer] is the "Add server" entry point: it keeps the form empty (no id,
     * no host) so a successful login adds a *second* install rather than editing the
     * active one. The default path prefills from the active install so logging back in
     * after a sign-out comes back the way the operator left it — and reuses its id, so
     * it updates that profile instead of duplicating it.
     */
    fun start(addNewServer: Boolean) {
        if (addNewServer) return
        viewModelScope.launch {
            authRepository.currentConnection()?.let { conn ->
                _state.update {
                    it.copy(
                        connectionId = conn.id,
                        host = conn.host,
                        port = conn.port.toString(),
                        adminPath = conn.adminPath,
                        isHttps = conn.isHttps,
                        credentialMode = conn.credentialMode,
                    )
                }
            }
            // If a pair is already stored, reflect that in the tick and prefill the
            // username so the form comes back the way the operator left it.
            authRepository.savedCredentials()?.let { saved ->
                _state.update {
                    it.copy(
                        saveCredentials = true,
                        username = it.username.ifBlank { saved.username },
                    )
                }
            }
        }
    }

    fun onHostChange(value: String) = _state.update { it.copy(host = value.trim(), error = null) }
    fun onPortChange(value: String) = _state.update { it.copy(port = value.filter { c -> c.isDigit() }, error = null) }

    /**
     * Keep the typed value as-is while editing — including leading/trailing slashes a
     * paste carries in. Stripping them on every keystroke deleted a `/` the instant it
     * was typed, which read as the field fighting the operator. Normalisation happens
     * in [normaliseAdminPath] on submit, and [Connection] trims again at use, so the
     * stored path is clean regardless.
     */
    fun onAdminPathChange(value: String) = _state.update { it.copy(adminPath = value, error = null) }

    /** Trim the admin path to its bare segment — no surrounding slashes. */
    private fun normaliseAdminPath(value: String): String = value.trim().trim('/')
    fun onHttpsChange(value: Boolean) = _state.update { it.copy(isHttps = value) }
    fun onUsernameChange(value: String) = _state.update { it.copy(username = value, error = null) }
    fun onPasswordChange(value: String) = _state.update { it.copy(password = value, error = null) }
    fun onCodeChange(value: String) = _state.update {
        it.copy(code = value.filter { c -> c.isDigit() }.take(TOTP_CODE_LENGTH))
    }
    fun onApiKeyChange(value: String) = _state.update { it.copy(apiKey = value.trim(), error = null) }
    fun onSaveCredentialsChange(value: Boolean) = _state.update { it.copy(saveCredentials = value) }

    fun onModeChange(mode: CredentialMode) = _state.update {
        it.copy(credentialMode = mode, error = null, needsTwoFactor = false)
    }

    fun submit() {
        val current = _state.value
        if (!current.canSubmit) return
        _state.update { it.copy(isLoading = true, error = null, trustPrompt = null) }

        viewModelScope.launch {
            val connection = Connection(
                id = current.connectionId,
                host = current.host,
                port = current.port.toIntOrNull() ?: 443,
                // Normalise only now, at use — not on every keystroke.
                adminPath = normaliseAdminPath(current.adminPath),
                isHttps = current.isHttps,
                credentialMode = current.credentialMode,
            )
            // Stage, not save: the login call needs this connection to rebase onto it,
            // but a wrong host must not survive a failed attempt. It is kept only on
            // success (commit) and undone on a real failure (rollback).
            val staged = authRepository.stageConnection(connection)

            when (current.credentialMode) {
                CredentialMode.SESSION -> submitSession(current, staged.id)
                CredentialMode.API_KEY -> submitApiKey(current)
            }
        }
    }

    private suspend fun submitSession(current: LoginUiState, connectionId: String) {
        when (val result = authRepository.loginWithPassword(
            username = current.username,
            password = current.password,
            code = current.code.ifBlank { null },
        )) {
            is LoginResult.Success -> {
                // Persist (or clear) the saved pair only now that login has actually
                // succeeded — a tick alone must not write a password that has never
                // been proven to work.
                if (current.saveCredentials) {
                    authRepository.saveCredentialsFor(connectionId, current.username, current.password)
                } else {
                    authRepository.clearSavedCredentialsFor(connectionId)
                }
                authRepository.commitStagedConnection()
                _state.update { it.copy(isLoading = false, authenticated = true) }
            }

            LoginResult.TwoFactorRequired -> _state.update {
                it.copy(isLoading = false, needsTwoFactor = true, code = "", error = null)
            }

            is LoginResult.TrustRequired -> promptTrust(result.fingerprint, result.changedFrom)

            is LoginResult.Failure -> {
                // The connection never authenticated; do not leave it saved.
                authRepository.rollbackStagedConnection()
                _state.update { it.copy(isLoading = false, error = result.message) }
            }
        }
    }

    private suspend fun submitApiKey(current: LoginUiState) {
        authRepository.useApiKey(current.apiKey)
        when (val check = authRepository.verifyCredential()) {
            CredentialCheck.Valid -> {
                authRepository.commitStagedConnection()
                _state.update {
                    it.copy(isLoading = false, authenticated = true)
                }
            }

            is CredentialCheck.TrustRequired -> {
                // The cert is untrusted, so the key was never checked — do not drop it.
                promptTrust(check.fingerprint, check.changedFrom)
            }

            is CredentialCheck.Rejected -> {
                // Drop the rejected key so a retry starts clean, and unsave the host.
                authRepository.logout()
                authRepository.rollbackStagedConnection()
                _state.update { it.copy(isLoading = false, error = check.message) }
            }

            is CredentialCheck.Unreachable -> {
                // Not a verdict on the key: the request never reached the server, so
                // the host is not proven either — unsave it.
                authRepository.rollbackStagedConnection()
                _state.update { it.copy(isLoading = false, error = check.message) }
            }
        }
    }

    /** Show the fingerprint and wait for the operator's decision. */
    private fun promptTrust(fingerprint: String, changedFrom: String?) {
        _state.update {
            it.copy(
                isLoading = false,
                trustPrompt = TrustPrompt(fingerprint, changedFrom),
                error = null,
            )
        }
    }

    /**
     * The operator confirmed the fingerprint: remember it, then retry the whole attempt
     * so the handshake succeeds and the credential is actually checked this time.
     */
    fun onTrustCertificate() {
        val prompt = _state.value.trustPrompt ?: return
        val host = _state.value.host
        viewModelScope.launch {
            authRepository.trustHost(host, prompt.fingerprint)
            _state.update { it.copy(trustPrompt = null) }
            submit()
        }
    }

    /** The operator declined: forget the prompt and stop. */
    fun onDismissTrust() = _state.update { it.copy(trustPrompt = null) }
}
