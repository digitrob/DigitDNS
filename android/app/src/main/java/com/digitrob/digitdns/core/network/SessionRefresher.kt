package com.digitrob.digitdns.core.network

import com.digitrob.digitdns.core.security.SecretStore
import com.digitrob.digitdns.domain.model.CredentialMode
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import javax.inject.Inject
import javax.inject.Provider
import javax.inject.Singleton
import timber.log.Timber

/**
 * Re-mints a lapsed dashboard session from the saved login pair, in place.
 *
 * The v3 session is short-lived by design — it has an idle window and lives only in
 * the daemon's memory, so a restart or a quiet spell invalidates it. Without a
 * recovery path, every screen that was open onto that session starts answering 401
 * (an empty upstreams list, an "Unauthorized credentials" banner on the TLS screen),
 * which reads as random breakage rather than a lapsed session. This is the one place
 * that turns "the server said 401" into "log back in silently and carry on".
 *
 * It deliberately does **not** depend on `AuthRepository`: that type owns `AuthApi`,
 * which owns the `OkHttpClient`, which owns the interceptor that calls this — a
 * cycle. The pieces needed (the token slot, the saved pair, the login endpoint) are
 * reachable directly, and `AuthApi` is resolved through a [Provider] so it is only
 * touched at call time, after the client is fully built.
 *
 * The refresh is guarded by a [Mutex] and a short cooldown: several in-flight
 * requests can all see the same 401 at once, and without the guard each would mint a
 * fresh session, invalidating the others a moment later.
 */
@Singleton
class SessionRefresher @Inject constructor(
    private val authState: AuthState,
    private val secretStore: SecretStore,
    private val authApi: Provider<AuthApi>,
) {
    private val mutex = Mutex()

    @Volatile
    private var lastAttemptAt = 0L

    @Volatile
    private var lastResult = false

    /**
     * Whether a silent re-login is even possible: a session credential is in play and
     * a saved login pair exists to replay. A key-auth install or one that never opted
     * into "save credentials" has no silent path and must re-prompt.
     */
    fun canRefresh(): Boolean {
        val id = authState.activeId ?: return false
        return authState.mode == CredentialMode.SESSION && secretStore.hasSavedCredentials(id)
    }

    /**
     * Re-mint the session from the saved pair. Returns true when a fresh token was
     * obtained (and installed into [AuthState]). Serialised and cooldown-guarded so a
     * burst of concurrent 401s mints at most one session per window.
     */
    suspend fun refresh(): Boolean = mutex.withLock {
        val id = authState.activeId
        if (id == null || authState.mode != CredentialMode.SESSION) return@withLock false
        val username = secretStore.savedUsername(id)
        val password = secretStore.savedPassword(id)
        if (username.isNullOrBlank() || password == null) return@withLock false

        // A refresh that just succeeded a moment ago will have already fixed the
        // token the other waiters are holding; do not spend a second session on it.
        val now = System.currentTimeMillis()
        if (lastResult && now - lastAttemptAt < SUCCESS_COOLDOWN_MILLIS) return@withLock true
        if (!lastResult && now - lastAttemptAt < FAILURE_COOLDOWN_MILLIS) return@withLock false

        lastAttemptAt = now
        lastResult = runCatching {
            val response = authApi.get().login(
                LoginRequest(username = username, password = password),
            )
            val token = response.token
            if (token.isBlank()) {
                false
            } else {
                authState.updateSessionToken(token)
                true
            }
        }.onFailure { Timber.i(it, "silent session refresh failed") }
            .getOrDefault(false)
        lastResult
    }

    /** [refresh] for OkHttp's synchronous interceptor thread. */
    fun refreshBlocking(): Boolean = runBlocking { refresh() }

    private companion object {
        const val SUCCESS_COOLDOWN_MILLIS = 5_000L
        const val FAILURE_COOLDOWN_MILLIS = 15_000L
    }
}
