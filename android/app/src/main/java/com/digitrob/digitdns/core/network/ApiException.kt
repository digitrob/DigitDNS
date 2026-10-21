package com.digitrob.digitdns.core.network

import java.io.IOException

/**
 * A failed API call, carrying whatever the server said about why.
 *
 * The app could let Retrofit throw and read the status at each call site, but the
 * three failure modes that matter here are cross-cutting:
 *  - **401** — the session's idle window lapsed. The remedy is one silent re-login
 *    and one retry, handled centrally, not in every ViewModel.
 *  - **403** — the install is not exposed to this network, or docs/OpenAPI are gated.
 *    The v3 server answers *this* with a problem+json body naming `api_bind`, which
 *    the operator needs to see verbatim.
 *  - **twofactor_required** — a login that is otherwise correct but is missing the
 *    code; the login screen advances to the code field on it.
 *
 * [isUnauthorized], [isForbidden] and [problem] expose those; everything else falls
 * back to a readable message.
 */
class ApiException(
    val httpStatus: Int,
    val problem: Problem?,
    /** The raw response body, kept for an unexpected shape the parser did not expect. */
    val rawBody: String? = null,
    cause: Throwable? = null,
) : IOException(buildMessage(httpStatus, problem), cause) {

    val isUnauthorized: Boolean get() = httpStatus == 401
    val isForbidden: Boolean get() = httpStatus == 403
    val isConflict: Boolean get() = httpStatus == 409
    val isTooManyRequests: Boolean get() = httpStatus == 429

    /** True when the server demands a TOTP code to finish the login. */
    val twoFactorRequired: Boolean get() = problem?.twoFactorRequired == true

    /** The message to show a person. */
    val userMessage: String
        get() = problem?.message ?: rawBody?.takeIf { it.isNotBlank() }?.take(300)
            ?: "Request failed (HTTP $httpStatus)"

    companion object {
        private fun buildMessage(status: Int, problem: Problem?): String =
            problem?.message ?: "HTTP $status"

        /** Build from a decoded problem document. */
        fun from(status: Int, problem: Problem?, rawBody: String? = null): ApiException =
            ApiException(status, problem, rawBody)
    }
}
