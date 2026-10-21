package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST

/**
 * The resolver access gate: who may query this install at all.
 *
 * Two write routes with a split responsibility that the UI must respect:
 *  - [update] (`POST /access`) carries the DoH token list and the allowlist CIDRs. The
 *    `allow_all` in its body is **ignored** by the server, which reads the live value
 *    instead, so saving tokens can never flip the mode as a side effect.
 *  - [setMode] (`POST /access/mode`) is the only route that changes `allow_all`, and
 *    it takes nothing else.
 *
 * The write is a **POST**, not a PUT: the delegated v3 handler only accepts POST, so a
 * PUT is a 405 surfaced as a generic failure.
 *
 * `allowed_ips` on the read side are the canonical CIDR form the resolver actually
 * uses — a legacy or hand-edited entry is shown as the resolver sees it, not as it was
 * typed.
 */
interface AccessApi {

    @GET("access")
    suspend fun get(): AccessResponse

    @POST("access")
    suspend fun update(@Body body: AccessUpdateRequest): SimpleSuccess

    @POST("access/mode")
    suspend fun setMode(@Body body: AccessModeRequest): AccessModeResponse
}

@Serializable
data class AccessResponse(
    @SerialName("allow_all") val allowAll: Boolean = false,
    @SerialName("doh_tokens") val dohTokens: List<String> = emptyList(),
    @SerialName("allowed_ips") val allowedIps: List<String> = emptyList(),
)

/**
 * The access write body. `allow_all` is sent for completeness but the server ignores
 * it; [AccessApi.setMode] is the route that changes the mode.
 */
@Serializable
data class AccessUpdateRequest(
    @SerialName("allow_all") val allowAll: Boolean = false,
    @SerialName("doh_tokens") val dohTokens: List<String> = emptyList(),
    @SerialName("allowed_ips") val allowedIps: List<String> = emptyList(),
)

@Serializable
data class AccessModeRequest(
    @SerialName("allow_all") val allowAll: Boolean,
)

@Serializable
data class AccessModeResponse(
    @SerialName("allow_all") val allowAll: Boolean = false,
)
