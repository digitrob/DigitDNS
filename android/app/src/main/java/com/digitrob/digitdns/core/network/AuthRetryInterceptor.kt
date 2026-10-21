package com.digitrob.digitdns.core.network

import okhttp3.Interceptor
import okhttp3.Response
import com.digitrob.digitdns.domain.model.CredentialMode
import javax.inject.Inject
import javax.inject.Singleton
import timber.log.Timber

/**
 * Retries a request once after a 401, when the session can be silently re-minted.
 *
 * This is the "one silent re-login and one retry, handled centrally" the app
 * promises: rather than teach every ViewModel about a lapsed session, a single 401
 * from any call is caught here, [SessionRefresher] replays the saved password, and
 * the original request is sent again with the fresh token. The caller never sees the
 * intermediate failure.
 *
 * It sits **before** [AuthInterceptor] in the chain so the retry re-runs the whole
 * downstream stack — the new token is attached by [AuthInterceptor] on the second
 * pass exactly as on the first. The failure it reacts to is the [ApiException] raised
 * by [ApiExceptionInterceptor] a layer below, so it catches that rather than peeking
 * at a raw response.
 *
 * It refrains from retrying where a retry is meaningless or harmful: the login and
 * ticket routes (a 401 there is the answer, not a lapse), and any non-session
 * credential (a rejected master key cannot be refreshed from a password). A single
 * retry is enforced so a persistently-wrong credential cannot loop.
 */
@Singleton
class AuthRetryInterceptor @Inject constructor(
    private val refresher: SessionRefresher,
    private val authState: AuthState,
) : Interceptor {

    override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()
        return try {
            chain.proceed(request)
        } catch (e: ApiException) {
            if (!e.isUnauthorized || !isRefreshable(request)) {
                throw e
            }
            if (!refresher.canRefresh()) {
                // A session 401 with no saved pair to replay is a real expiry: tell the
                // UI so it can route to login with an explanation. A rejected API key is
                // not a session lapse and is left for its own screen to report.
                if (authState.mode == CredentialMode.SESSION) authState.markSessionExpired()
                throw e
            }
            Timber.i("request refused with 401; attempting silent session refresh")
            if (!refresher.refreshBlocking()) {
                authState.markSessionExpired()
                throw e
            }
            // Retry once, on the same request object, so AuthInterceptor re-attaches
            // the credential it now finds in AuthState.
            chain.proceed(request)
        }
    }

    /** Login, logout, the SSE ticket and the stream itself: a 401 there is a verdict. */
    private fun isRefreshable(request: okhttp3.Request): Boolean {
        val path = request.url.encodedPath
        return AUTH_PATHS.none { path.endsWith(it) }
    }

    private companion object {
        val AUTH_PATHS = listOf(
            "auth/login",
            "auth/logout",
            "auth/sse-ticket",
            // A rejected stream ticket means "mint a fresh one", not "refresh the
            // session"; retrying with the same spent ticket only wastes a round trip.
            "events/stream",
        )
    }
}
