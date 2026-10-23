package com.digitrob.digitdns.core.network

import com.digitrob.digitdns.core.security.SecretStore
import com.digitrob.digitdns.domain.model.CredentialMode
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The credential the app presents on every request, and where it comes from.
 *
 * Interceptors run on OkHttp's threads and cannot suspend, so the live token is held
 * here in memory (backed by [SecretStore] for restarts) and read without blocking.
 * Mutations are `@Volatile` writes of a single reference, which is the cheapest
 * correct choice at this scale — one operator, a handful of in-flight requests.
 *
 * With several saved installs, the credential in memory belongs to exactly one of them
 * — the active one, identified by [activeId]. [loadFor] swaps the in-memory token when
 * the operator switches installs, and every write goes to that install's slot in
 * [SecretStore], so a switch never leaks one install's session to another.
 *
 * The mode decides the header: a session token is `Authorization: Bearer`, the
 * master key is `X-API-Key`. The server accepts either, but sending both would be
 * wrong — it would present the master key on every request even when a session is
 * the intended, narrower credential.
 */
@Singleton
class AuthState @Inject constructor(
    private val secretStore: SecretStore,
) {
    @Volatile
    var activeId: String? = null
        private set

    @Volatile
    var sessionToken: String? = null
        private set

    @Volatile
    var apiKey: String? = null
        private set

    @Volatile
    var mode: CredentialMode = CredentialMode.SESSION
        private set

    private val _sessionExpired = MutableStateFlow(false)

    // Every mutation of the credential above bumps this, so a screen that wants to
    // know "is the active install actually signed in" can observe it. The token
    // itself is a @Volatile field read from OkHttp threads, not a flow.
    private val _credentialRevision = MutableStateFlow(0)

    /** Increments whenever [hasCredential] may have changed. */
    val credentialRevision: StateFlow<Int> = _credentialRevision.asStateFlow()

    private fun bumpCredentialRevision() {
        _credentialRevision.value += 1
    }

    /**
     * Set true when a request was refused with a 401 that could not be silently
     * recovered, so the UI can route to the login screen and explain why. Cleared on
     * any successful re-authentication ([loadFor]/[updateSessionToken]).
     */
    val sessionExpired: StateFlow<Boolean> = _sessionExpired.asStateFlow()

    fun markSessionExpired() {
        _sessionExpired.value = true
    }

    fun clearSessionExpired() {
        _sessionExpired.value = false
    }

    /** True when a credential is available for [mode] and requests can be sent. */
    val hasCredential: Boolean
        get() = when (mode) {
            CredentialMode.SESSION -> !sessionToken.isNullOrBlank()
            CredentialMode.API_KEY -> !apiKey.isNullOrBlank()
        }

    /**
     * Point the in-memory credential at [id]'s stored secrets. Called when the active
     * connection is loaded or switched; replaces whatever was in memory.
     */
    fun loadFor(id: String, mode: CredentialMode) {
        activeId = id
        this.mode = mode
        sessionToken = secretStore.sessionToken(id)
        apiKey = secretStore.apiKey(id)
        clearSessionExpired()
        bumpCredentialRevision()
    }

    fun updateMode(mode: CredentialMode) {
        this.mode = mode
        bumpCredentialRevision()
    }

    fun updateSessionToken(token: String?) {
        sessionToken = token
        activeId?.let { secretStore.setSessionToken(it, token) }
        if (!token.isNullOrBlank()) clearSessionExpired()
        bumpCredentialRevision()
    }

    fun updateApiKey(key: String?) {
        apiKey = key
        activeId?.let { secretStore.setApiKey(it, key) }
        bumpCredentialRevision()
    }

    /** Drop the in-memory credential for the active install, leaving other installs'. */
    fun clear() {
        sessionToken = null
        apiKey = null
        activeId?.let { secretStore.clearCredentials(it) }
        clearSessionExpired()
        bumpCredentialRevision()
    }

    /**
     * Forget the in-memory credential only, leaving [SecretStore] untouched. Used when
     * the active connection is momentarily unknown (a startup race on the DataStore
     * read) so a transient empty read cannot destroy a session that is still on disk.
     */
    fun clearMemory() {
        sessionToken = null
        apiKey = null
        bumpCredentialRevision()
    }
}
