package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.PATCH
import retrofit2.http.POST
import retrofit2.http.Path
import retrofit2.http.Query

/**
 * The v3 clients surface, from `internal/api/openapi/v3.json` plus the handler's
 * actual body shapes (`router_v3_handlers.go`).
 *
 * Paths are relative to the API root; [AuthInterceptor] prepends the admin path and
 * rewrites the origin.
 *
 * **Cursor pagination.** `list` takes `limit`/`cursor` and returns `next_cursor`; the
 * cursor is the server's own next-offset string, opaque to the client. The app is
 * expected to page through, not to guess at offsets — the cursor is the contract.
 */
interface ClientsApi {

    @GET("clients")
    suspend fun list(
        @Query("limit") limit: Int? = null,
        @Query("cursor") cursor: String? = null,
    ): ClientListResponse

    @GET("clients/{id}")
    suspend fun get(@Path("id") id: String): ClientDto

    @POST("clients")
    suspend fun create(@Body body: ClientCreateRequest): ClientDto

    @PATCH("clients/{id}")
    suspend fun patch(@Path("id") id: String, @Body body: ClientUpdateRequest): ClientDto

    @DELETE("clients/{id}")
    suspend fun delete(@Path("id") id: String)

    /**
     * `POST /clients/{id}/actions/{action}`. Each action carries its own body; the
     * caller passes the right one. Actions: renew, toggle, add_ip, remove_ip,
     * reset-traffic, regenerate-uuid, regenerate-register-secret.
     */
    @POST("clients/{id}/actions/{action}")
    suspend fun action(
        @Path("id") id: String,
        @Path("action") action: String,
        @Body body: ClientActionBody = ClientActionBody(),
    ): ClientActionResponse
}

@Serializable
data class ClientDto(
    val id: String = "",
    val uuid: String = "",
    val name: String = "",
    val token: String = "",
    @SerialName("allowed_ips") val allowedIps: List<String> = emptyList(),
    @SerialName("traffic_limit_gb") val trafficLimitGb: Double = 0.0,
    @SerialName("traffic_used_bytes") val trafficUsedBytes: Long = 0,
    @SerialName("traffic_reset_cycle") val trafficResetCycle: String = "",
    @SerialName("quota_exceeded") val quotaExceeded: Boolean = false,
    @SerialName("expires_at") val expiresAt: String = "",
    val enabled: Boolean = false,
    val note: String = "",
    @SerialName("custom_policies") val customPolicies: List<String> = emptyList(),
    @SerialName("max_devices") val maxDevices: Int = 0,
    @SerialName("register_secret") val registerSecret: String = "",
    @SerialName("total_queries") val totalQueries: Long = 0,
    @SerialName("last_seen") val lastSeen: String = "",
    @SerialName("next_traffic_reset") val nextTrafficReset: String = "",
)

@Serializable
data class ClientListResponse(
    val items: List<ClientDto> = emptyList(),
    @SerialName("next_cursor") val nextCursor: String = "",
    @SerialName("allow_all") val allowAll: Boolean = false,
)

@Serializable
data class ClientCreateRequest(
    val name: String,
    @SerialName("validity_days") val validityDays: Int = 0,
    /** Absolute expiry (RFC 3339). Wins over [validityDays] when set. */
    @SerialName("expires_at") val expiresAt: String? = null,
    @SerialName("allowed_ips") val allowedIps: List<String> = emptyList(),
    @SerialName("traffic_limit_gb") val trafficLimitGb: Double = 0.0,
    @SerialName("traffic_reset_cycle") val trafficResetCycle: String = "",
    @SerialName("custom_policies") val customPolicies: List<String> = emptyList(),
    val note: String = "",
    @SerialName("max_devices") val maxDevices: Int = 0,
)

/**
 * A PATCH body where "absent" and "empty" are different things: the server treats a
 * null field as "leave unchanged", so every member defaults to null and only the
 * fields the operator actually edited are sent. kotlinx.serialization omits nulls
 * because the converter is configured with `explicitNulls = false`.
 */
@Serializable
data class ClientUpdateRequest(
    val name: String? = null,
    /** Replacement UUID; null leaves the stored one alone. */
    val uuid: String? = null,
    /** RFC 3339 expiry; [ClientExpiry.NEVER] clears it (lifetime). */
    @SerialName("expires_at") val expiresAt: String? = null,
    @SerialName("allowed_ips") val allowedIps: List<String>? = null,
    @SerialName("traffic_limit_gb") val trafficLimitGb: Double? = null,
    @SerialName("traffic_reset_cycle") val trafficResetCycle: String? = null,
    val note: String? = null,
    val enabled: Boolean? = null,
    @SerialName("validity_days_add") val validityDaysAdd: Int? = null,
    @SerialName("max_devices") val maxDevices: Int? = null,
)

/** Per-action body; only the fields the action reads are ever set. */
@Serializable
data class ClientActionBody(
    @SerialName("extend_days") val extendDays: Int? = null,
    val enabled: Boolean? = null,
    val ip: String? = null,
)

@Serializable
data class ClientActionResponse(
    val success: Boolean = false,
    val reset: Boolean = false,
    val uuid: String = "",
    @SerialName("register_secret") val registerSecret: String = "",
)
