package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.AuthApi
import com.digitrob.digitdns.core.network.AuthState
import com.digitrob.digitdns.core.network.LoginRequest
import com.digitrob.digitdns.core.network.ServerUrlProvider
import com.digitrob.digitdns.core.network.SessionRefresher
import com.digitrob.digitdns.core.security.SecretStore
import com.digitrob.digitdns.core.security.TlsTrustFailure
import com.digitrob.digitdns.domain.model.Connection
import com.digitrob.digitdns.domain.model.CredentialMode
import kotlinx.coroutines.flow.firstOrNull
import java.io.IOException
import javax.inject.Inject
import javax.inject.Singleton
import timber.log.Timber

/**
 * The outcome of a login attempt.
 *
 * A login is not simply "worked" or "failed": when two-factor is on and the code is
 * missing or wrong, the server answers 401 with `twofactor_required`, and the right
 * response is to ask for the code — not to re-prompt for the password. Modelling
 * that as its own case keeps the screen's branching honest.
 */
sealed interface LoginResult {
    data class Success(val username: String, val passwordWeak: Boolean) : LoginResult

    /** The password was right but a TOTP code is required (or the one given was wrong). */
    data object TwoFactorRequired : LoginResult

    /**
     * The install's certificate is not trusted yet, so the handshake never completed
     * and no credential was checked. The operator confirms [fingerprint] (and the app
     * pins it), then retries — see [AuthRepository.trustCurrentHost] and
     * [AuthRepository.verifyCredential]'s `TrustRequired`.
     */
    data class TrustRequired(val fingerprint: String, val changedFrom: String?) : LoginResult

    data class Failure(val message: String) : LoginResult
}

/**
 * The outcome of probing the current credential against `/auth/me`.
 *
 * Three answers, not a boolean, because "no" has two very different causes: a
 * genuinely rejected key (fix the key) and a connection that never got far enough to
 * check it (fix the address, the path, or trust the certificate). Collapsing them
 * into `false` is what made an untrusted cert and a wrong admin path both read as
 * "the API key was rejected".
 */
/** A stored login pair, present only when the operator opted into "save credentials". */
data class SavedCredentials(val username: String, val password: String)

sealed interface CredentialCheck {
    /** The credential was accepted. */
    data object Valid : CredentialCheck

    /** The server refused the credential (401). The key/session is wrong. */
    data class Rejected(val message: String) : CredentialCheck

    /** The certificate is untrusted; nothing was checked. */
    data class TrustRequired(val fingerprint: String, val changedFrom: String?) : CredentialCheck

    /** The request never reached a verdict — transport, DNS, TLS, or an unexpected status. */
    data class Unreachable(val message: String) : CredentialCheck
}

/**
 * All authentication for the app: the saved connection, the credential it presents,
 * and the login flows that establish one.
 *
 * The two credential modes converge here. A session login trades username/password
 * (+TOTP) for a `digitdns_session_` bearer; the master-key path just stores the key.
 * Both then behave identically on the wire because [AuthState] decides the header.
 *
 * **Idle-window handling.** The v3 session expires after an idle period
 * (`expires_in`, `session_idle_minutes` on the server). Rather than let every screen
 * surface a 401, the app attempts one silent re-login with the stored credentials
 * when a call is refused; [retryOnUnauthorized] is that hook, and the login
 * credentials are kept (encrypted) only when the operator opts into "stay signed
 * in". Without them a lapsed session is a real logout and the login screen returns,
 * which is the honest behaviour for a device that is not meant to hold a password.
 *
 * The function count is high because this is the single seam for an install's whole
 * lifecycle — connect, log in, switch, forget, refresh — and splitting those apart
 * would only spread the same [AuthState] invariants across several classes.
 */
@Suppress("TooManyFunctions")
@Singleton
class AuthRepository @Inject constructor(
    private val authApi: AuthApi,
    private val authState: AuthState,
    private val connectionStore: ConnectionStore,
    private val urlProvider: ServerUrlProvider,
    private val secretStore: SecretStore,
    private val sessionRefresher: SessionRefresher,
) {
    /**
     * Persist a connection and point the runtime at it.
     *
     * When [connection] carries an id that is already saved, its fields are updated in
     * place; when it is new (blank id, or an id not in the list), it is added and made
     * active. Either way the active install becomes this one.
     */
    suspend fun saveConnection(connection: Connection) {
        val known = connectionStore.servers.firstOrNull().orEmpty().any { it.id == connection.id }
        if (connection.id.isBlank() || !known) {
            connectionStore.add(connection)
        } else {
            connectionStore.setActive(connection.id)
            connectionStore.saveActive(connection)
        }
        urlProvider.invalidate()
        loadActiveCredential()
    }

    /**
     * What a provisional save has to undo if the login it enabled fails: the install
     * that was active before, and whether the target was already a saved profile.
     */
    private data class StagedConnection(
        val previous: Connection?,
        val target: Connection,
        val wasNew: Boolean,
    )

    private var staged: StagedConnection? = null

    /**
     * Persist [connection] provisionally so the login call it enables can rebase onto
     * it, remembering what to restore if the attempt fails.
     *
     * A login cannot be sent before the connection is known — the host, port and admin
     * path are the request's origin — so the form's values must reach [ServerUrlProvider]
     * first. Saving them permanently at that point is the bug: a mistyped host would
     * stay in the saved-server list even though it never authenticated. Staging splits
     * "usable for this attempt" from "kept", and [commitStagedConnection] is the only
     * path that keeps it.
     */
    suspend fun stageConnection(connection: Connection) {
        val known = connectionStore.servers.firstOrNull().orEmpty().any { it.id == connection.id }
        staged = StagedConnection(
            previous = connectionStore.connection.firstOrNull(),
            target = connection,
            wasNew = connection.id.isBlank() || !known,
        )
        saveConnection(connection)
    }

    /** Keep the staged connection: it authenticated, so it stays saved. */
    fun commitStagedConnection() {
        staged = null
    }

    /**
     * Undo a staged save whose login did not succeed. A newly-added profile is removed
     * and the install that was active before is restored; a still-live session for the
     * previous server is left untouched. A no-op when nothing was staged.
     */
    suspend fun rollbackStagedConnection() {
        val pending = staged ?: return
        staged = null
        if (pending.wasNew && pending.target.id.isNotBlank()) {
            connectionStore.remove(pending.target.id)
        }
        pending.previous?.let { connectionStore.setActive(it.id) }
        urlProvider.invalidate()
        loadActiveCredential()
    }

    /**
     * Adopt the active install's stored credential into memory. Called once at startup
     * and after every switch; a no-op (clearing the in-memory token) when nothing is
     * active.
     */
    suspend fun loadActiveCredential() {
        val active = connectionStore.connection.firstOrNull()
        if (active == null) {
            // Memory only: a transient empty read at startup must not erase a session
            // that is still persisted. Explicit logout/forget is what removes secrets.
            authState.clearMemory()
        } else {
            authState.loadFor(active.id, active.credentialMode)
        }
    }

    /**
     * One-time adoption of a pre-multi-server install: copy the legacy connection into
     * the list and move its credentials under the new id. Safe to call on every start —
     * it does nothing once migration has run.
     */
    suspend fun migrateLegacyIfNeeded() {
        val id = connectionStore.migrateLegacyIfNeeded() ?: return
        secretStore.adoptLegacyCredential(id)
        urlProvider.invalidate()
        loadActiveCredential()
    }

    /** Every saved install, in add order. */
    suspend fun listServers(): List<Connection> = connectionStore.servers.firstOrNull().orEmpty()

    /** Switch to another saved install and adopt its credential. */
    suspend fun setActiveServer(id: String) {
        connectionStore.setActive(id)
        urlProvider.invalidate()
        loadActiveCredential()
    }

    /**
     * Forget an install: drop its saved credential and remove it from the list. When it
     * was active, the store promotes another install and the runtime follows.
     */
    suspend fun removeServer(id: String) {
        secretStore.clearCredentials(id)
        connectionStore.remove(id)
        urlProvider.invalidate()
        loadActiveCredential()
    }

    /** The saved connection, read straight from storage (not the interceptor cache). */
    suspend fun currentConnection(): Connection? = connectionStore.connection.firstOrNull()

    /**
     * Log in with a username/password, plus a TOTP [code] when one is supplied.
     * On success the session token is stored and the app is authenticated.
     */
    suspend fun loginWithPassword(
        username: String,
        password: String,
        code: String? = null,
    ): LoginResult {
        authState.updateMode(CredentialMode.SESSION)
        return try {
            val response = authApi.login(
                LoginRequest(username = username, password = password, code = code?.ifBlank { null }),
            )
            authState.updateSessionToken(response.token)
            LoginResult.Success(
                username = response.username.ifBlank { username },
                passwordWeak = response.passwordWeak,
            )
        } catch (e: ApiException) {
            if (e.twoFactorRequired) {
                LoginResult.TwoFactorRequired
            } else {
                LoginResult.Failure(httpFailureMessage(e))
            }
        } catch (e: IOException) {
            // Transport failures (unknown host, refused, TLS, timeout) all arrive as
            // IOException; anything else is a programming error and should propagate.
            Timber.w(e, "login transport failure")
            // A self-signed cert on first contact is not a transport failure to report
            // as one — it is a decision the operator has to make, so it is its own case.
            TlsTrustFailure.find(e)
                ?.let { return LoginResult.TrustRequired(it.fingerprint, it.changedFrom) }
            LoginResult.Failure(connectionFailureMessage(e))
        }
    }

    /**
     * Adopt the master REST API key as the credential. There is no server round-trip
     * to validate it at login time — the key is checked on the first real request — so
     * this stores it and lets the caller probe (`me()`), turning a 401 into a clear
     * "this key was rejected" rather than a silent no-op.
     */
    suspend fun useApiKey(key: String) {
        authState.updateMode(CredentialMode.API_KEY)
        authState.updateApiKey(key)
    }

    /**
     * Verify the current credential against `/auth/me`. Used right after adopting an
     * API key, and as a cheap liveness check when returning to the app.
     *
     * Returns *why* it failed, not just that it did: a rejected key, an untrusted
     * certificate, and a wrong admin path are three different remedies, and a single
     * boolean forced them all through "the API key was rejected".
     */
    suspend fun verifyCredential(): CredentialCheck = try {
        authApi.me()
        CredentialCheck.Valid
    } catch (e: ApiException) {
        Timber.i("credential check refused with HTTP %d", e.httpStatus)
        // A 401 is the server judging the credential. Anything else (a 404 from a
        // wrong admin path, a 400 from the exposure gate, a 5xx) is not a verdict on
        // the key at all and must not be reported as one.
        if (e.isUnauthorized) {
            CredentialCheck.Rejected(httpFailureMessage(e))
        } else {
            CredentialCheck.Unreachable(httpFailureMessage(e))
        }
    } catch (e: IOException) {
        Timber.w(e, "credential check failed")
        TlsTrustFailure.find(e)
            ?.let { return CredentialCheck.TrustRequired(it.fingerprint, it.changedFrom) }
        CredentialCheck.Unreachable(connectionFailureMessage(e))
    }

    /**
     * Remember [host]'s certificate fingerprint, so the next handshake to it is
     * accepted by [com.digitrob.digitdns.core.security.TofuTrustManager] without a
     * prompt. Called when the operator confirms the fingerprint the app showed them.
     */
    fun trustHost(host: String, fingerprint: String) {
        secretStore.pinFingerprint(host, fingerprint)
    }

    /** The host of the saved connection, or null when none is configured. */
    suspend fun currentHost(): String? = connectionStore.connection.firstOrNull()?.host

    /**
     * Remember the login pair so the app can silently re-mint a lapsed session. Called
     * only when the operator ticks "save credentials"; the untick path clears them.
     */
    fun saveCredentials(username: String, password: String) {
        val id = authState.activeId ?: return
        secretStore.setSavedUsername(id, username)
        secretStore.setSavedPassword(id, password)
    }

    /** Forget the saved login pair, leaving the connection and any live session alone. */
    fun clearSavedCredentials() {
        val id = authState.activeId ?: return
        secretStore.setSavedUsername(id, null)
        secretStore.setSavedPassword(id, null)
    }

    /** The saved login pair for the active install, or null when the operator did not opt in. */
    fun savedCredentials(): SavedCredentials? {
        val id = authState.activeId ?: return null
        val user = secretStore.savedUsername(id)
        val pass = secretStore.savedPassword(id)
        return if (!user.isNullOrBlank() && pass != null) SavedCredentials(user, pass) else null
    }

    /** True when the install [id] holds a saved login pair, for per-install status display. */
    fun hasSavedCredentials(id: String): Boolean = secretStore.hasSavedCredentials(id)

    /** True when a credential is already held, so the app can skip the login screen. */
    fun hasCredential(): Boolean = authState.hasCredential

    /**
     * Re-establish a session from the saved pair after an idle-window lapse. Returns
     * true when a fresh token was obtained. Delegates to [loginWithPassword] so the
     * same TOTP/mfa branching applies — a saved pair on an account with 2FA will land
     * on the code screen rather than authenticate, which is why the caller must be
     * ready to fall back to the login screen.
     */
    suspend fun refreshSavedSession(): Boolean {
        val saved = savedCredentials() ?: return false
        return loginWithPassword(saved.username, saved.password) is LoginResult.Success
    }

    /**
     * Bring the active credential to a usable state on startup, without forcing a
     * logout. A persisted session token can outlive the server's in-memory session
     * (daemon restart, idle window), so the first real request would 401 even though
     * the app still holds a token. When that is the case and a saved login pair
     * exists, re-mint the session silently; when no silent path exists, leave the
     * (possibly stale) credential in place — routing and per-screen 401 handling will
     * prompt for sign-in, which is honest for a device that never opted into saving a
     * password. Never tears down a recoverable session.
     *
     * Returns true when a credential is usable and the app may start signed in.
     */
    suspend fun ensureSession(): Boolean {
        if (!authState.hasCredential) return false
        if (authState.mode == CredentialMode.SESSION && sessionRefresher.canRefresh()) {
            // Optimistically re-mint before the first screen fires its calls, so the
            // app does not flash "Unauthorized" on a session that only just lapsed.
            sessionRefresher.refresh()
        }
        return authState.hasCredential
    }

    /** Revoke the session server-side and forget it locally. Best-effort. */
    suspend fun logout() {
        if (authState.mode == CredentialMode.SESSION && authState.sessionToken != null) {
            runCatching { authApi.logout() }.onFailure { Timber.i(it, "logout call failed; clearing locally anyway") }
        }
        authState.clear()
    }

    /**
     * One silent re-login attempt used when a request is refused with 401. Only
     * meaningful for the session mode, and only when credentials are available to
     * replay — which, in this app, means the caller supplied them this session.
     * Returns true if a fresh token was obtained.
     */
    suspend fun silentlyRefreshSession(username: String, password: String): Boolean {
        if (authState.mode != CredentialMode.SESSION) return false
        return loginWithPassword(username, password) is LoginResult.Success
    }

    private fun connectionFailureMessage(e: IOException): String = when (e) {
        is java.net.UnknownHostException -> "Host not found. Check the address."
        is java.net.SocketTimeoutException -> "The server did not respond in time."
        is javax.net.ssl.SSLException -> "TLS failed. The certificate may not be trusted yet."
        is java.net.ConnectException -> "Connection refused. Check the port and that the server is running."
        else -> e.message ?: "Could not reach the server."
    }

    /**
     * A message for a failure the *server* answered with. The status decides the
     * remedy, and they are not interchangeable:
     *  - **404** — no handler matched. On this API that almost always means the admin
     *    path is wrong (or the request lost its `/api/v3` segment): point at the path.
     *  - **400** — the exposure gate refused this address, not the credential. The
     *    server's own `detail` ("REST API is not exposed…") is the right thing to show.
     *  - **401** — the credential is genuinely wrong.
     *  - **403** — the address is admitted but the route still refuses; show the detail.
     * Both the gate (400) and the fallthrough use the problem `detail` when present,
     * because on those paths the server's wording is more specific than any we could
     * invent.
     */
    private fun httpFailureMessage(e: ApiException): String = when {
        e.isUnauthorized -> e.userMessage
        e.httpStatus == HTTP_NOT_FOUND ->
            "No API here — check the admin path (that part of the URL is the install's secret)."
        else -> e.userMessage
    }

    private companion object {
        /** HTTP 404: no route matched, which on this API means a wrong admin path. */
        const val HTTP_NOT_FOUND = 404
    }
}
