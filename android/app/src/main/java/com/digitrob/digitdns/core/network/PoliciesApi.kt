package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST

/**
 * `GET`/`POST /api/v3/policies` — the raw stored rule entries.
 *
 * This is the lower-level view of the same records the rules screen edits: the five
 * built-in keys (`custom_proxied`, `custom_proxied_disabled`, `custom_blocked`,
 * `custom_direct`, `custom_records`) are themselves stored as Policy rows and appear
 * in `items` alongside any custom policy the operator stored here.
 *
 * POST is a strict upsert keyed by `key`: unknown fields are rejected, and the whole
 * record is written, so this is not a partial patch. `catalog` is a legacy field that
 * is always an empty array in this build; it is decoded so an install that starts
 * populating it does not break the parse.
 */
interface PoliciesApi {

    @GET("policies")
    suspend fun list(): PoliciesResponse

    @POST("policies")
    suspend fun create(@Body body: PolicyDto): PolicyDto
}

@Serializable
data class PoliciesResponse(
    val items: List<PolicyDto> = emptyList(),
    val catalog: List<String> = emptyList(),
)

@Serializable
data class PolicyDto(
    val key: String = "",
    val name: String = "",
    val category: String = "",
    val enabled: Boolean = false,
    @SerialName("custom_domains") val customDomains: List<String> = emptyList(),
)
