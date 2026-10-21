package com.digitrob.digitdns.core.network

import com.digitrob.digitdns.domain.model.CredentialMode
import okhttp3.Interceptor
import okhttp3.Response
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Attaches the credential and rebases the URL, on every request.
 *
 * Order matters and is fixed by the two jobs this does:
 *  1. **Rebase first.** The typed Retrofit methods declare a path relative to the
 *     API root; the origin (scheme/host/port/admin-path) is only known from the
 *     stored connection, so the URL is rewritten before the request leaves.
 *  2. **Credential.** A session token rides as `Authorization: Bearer`; the master
 *     key rides as `X-API-Key`. Exactly one is sent — never both — because the
 *     server's `securityV3` accepts either and presenting the master key alongside a
 *     narrower session would widen what a leaked request log exposes.
 *
 * Meta calls that are keyless (`/version`, `/auth/login`) simply carry no
 * credential; adding one is harmless but unnecessary, so the check is "is a
 * credential present", not "is this route public".
 */
@Singleton
class AuthInterceptor @Inject constructor(
    private val authState: AuthState,
    private val urlProvider: ServerUrlProvider,
) : Interceptor {

    override fun intercept(chain: Interceptor.Chain): Response {
        // An already-absolute URL (the SSE stream, which builds its own) has an origin
        // and needs no stored connection; anything else still on the placeholder base
        // with no connection is a call that would contact 127.0.0.1 if let through.
        if (urlProvider.current() == null && !urlProvider.isAbsoluteApiUrl(chain.request().url)) {
            throw NoConnectionException()
        }
        val rebased = urlProvider.rebase(chain.request().url)
        val builder = chain.request().newBuilder().url(rebased)

        when (authState.mode) {
            CredentialMode.SESSION -> authState.sessionToken
                ?.takeIf { it.isNotBlank() }
                ?.let { builder.header("Authorization", "Bearer $it") }

            CredentialMode.API_KEY -> authState.apiKey
                ?.takeIf { it.isNotBlank() }
                ?.let { builder.header("X-API-Key", it) }
        }

        return chain.proceed(builder.build())
    }
}
