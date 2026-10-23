package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.data.AuthRepository
import com.digitrob.digitdns.domain.model.CredentialMode
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

/** What the Session view shows and whether a sign-out is in flight. */
data class SessionUiState(
    /** `host:port` of the install, or the app's placeholder before a connection exists. */
    val authority: String = "",
    val adminPath: String = "",
    val credentialMode: CredentialMode = CredentialMode.SESSION,
    /** True when a username/password pair is stored for silent re-login. */
    val credentialsSaved: Boolean = false,
    val isSigningOut: Boolean = false,
) {
    val credentialLabel: String
        get() = when (credentialMode) {
            CredentialMode.SESSION -> "Account session"
            CredentialMode.API_KEY -> "Master API key"
        }
}

/**
 * The Session view: which install the app is signed into, which credential it presents,
 * and the way out.
 *
 * Logout is deliberately explicit — revoke the session server-side, drop the stored
 * credential, then go back to the login screen — because "sign out" that only forgot a
 * token locally would leave a live session server-side, and one that only forgot it
 * server-side would leave a saved password on the device.
 */
@HiltViewModel
class SessionViewModel @Inject constructor(
    private val authRepository: AuthRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(SessionUiState())
    val state: StateFlow<SessionUiState> = _state.asStateFlow()

    init {
        // Follow both the active profile and the credential revision: switching emits
        // on the connection flow, while saving/clearing a password only changes the
        // secret store (the connection itself is unchanged). Collecting just
        // `currentConnection()` once left the card stale after either event.
        viewModelScope.launch {
            combine(
                authRepository.activeConnection,
                authRepository.credentialRevision,
            ) { connection, _ -> connection }.collect { connection ->
                _state.update {
                    it.copy(
                        authority = connection?.authority ?: "",
                        adminPath = connection?.adminPath ?: "",
                        credentialMode = connection?.credentialMode ?: CredentialMode.SESSION,
                        credentialsSaved = connection?.let { authRepository.hasSavedCredentials(it.id) } == true,
                    )
                }
            }
        }
    }

    /** Revoke and forget; [onDone] runs once the credential is cleared. */
    fun signOut(onDone: () -> Unit) {
        if (_state.value.isSigningOut) return
        _state.update { it.copy(isSigningOut = true) }
        viewModelScope.launch {
            authRepository.logout()
            _state.update { it.copy(isSigningOut = false) }
            onDone()
        }
    }
}
