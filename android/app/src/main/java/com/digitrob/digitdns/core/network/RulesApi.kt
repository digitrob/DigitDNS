package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST

/**
 * `GET`/`POST /api/v3/rules` — the four custom domain lists plus pinned records.
 *
 * The write is a **POST**, not a PUT: the v3 rules handler is mounted for POST only
 * (the same route the panel writes to), so a `PUT` is a 405 the app would surface as a
 * generic failure. The body is a **partial merge**: the server starts from the persisted
 * state and only replaces the keys the body actually carries, so a screen can save the
 * one list the operator edited without round-tripping the other four. That is why
 * [RulesUpdateRequest] has a nullable member per key rather than required ones — an
 * omitted member means "leave this list alone", and the converter drops nulls.
 *
 * Every list on the read side is guaranteed non-null (`[]` at worst), and
 * `custom_records` is always an object, not a list of "domain=ip" strings — the server
 * splits those back out before answering.
 */
interface RulesApi {

    @GET("rules")
    suspend fun get(): RulesResponse

    @POST("rules")
    suspend fun update(@Body body: RulesUpdateRequest): SimpleSuccess
}

@Serializable
data class RulesResponse(
    @SerialName("custom_proxied") val customProxied: List<String> = emptyList(),
    @SerialName("custom_proxied_disabled") val customProxiedDisabled: List<String> = emptyList(),
    @SerialName("custom_blocked") val customBlocked: List<String> = emptyList(),
    @SerialName("custom_direct") val customDirect: List<String> = emptyList(),
    @SerialName("custom_records") val customRecords: Map<String, String> = emptyMap(),
)

/**
 * A rules save. Present keys replace their list wholesale; absent keys are untouched
 * by the server. `explicitNulls = false` on the Json instance keeps the absent ones
 * out of the wire body — a present-but-null would be an error for this API.
 */
@Serializable
data class RulesUpdateRequest(
    @SerialName("custom_proxied") val customProxied: List<String>? = null,
    @SerialName("custom_proxied_disabled") val customProxiedDisabled: List<String>? = null,
    @SerialName("custom_blocked") val customBlocked: List<String>? = null,
    @SerialName("custom_direct") val customDirect: List<String>? = null,
    @SerialName("custom_records") val customRecords: Map<String, String>? = null,
)

/** The shared `{"success": true}` acknowledgement many write routes return. */
@Serializable
data class SimpleSuccess(val success: Boolean = false)
