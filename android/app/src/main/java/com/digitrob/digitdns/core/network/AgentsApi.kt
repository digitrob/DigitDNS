package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.PUT
import retrofit2.http.Path

/**
 * The v3 relay-agent surface (`internal/api/router_v3_handlers.go`).
 *
 * The list never carries an agent key. The create and rotate-key calls return the
 * clear key once; the copy action asks `/key` for the encrypted-at-rest copy. An
 * agent that enrolled before encrypted keys were stored answers 409 there and
 * needs one rotation first.
 */
interface AgentsApi {

    @GET("agents")
    suspend fun list(): AgentsResponse

    @POST("agents")
    suspend fun create(@Body body: AgentCreateRequest): AgentKeyResponse

    @PUT("agents/{id}")
    suspend fun update(@Path("id") id: String, @Body body: AgentUpdateRequest): AgentEnvelope

    @DELETE("agents/{id}")
    suspend fun delete(@Path("id") id: String): SimpleSuccess

    @POST("agents/{id}/enable")
    suspend fun enable(@Path("id") id: String): SimpleSuccess

    @POST("agents/{id}/disable")
    suspend fun disable(@Path("id") id: String): SimpleSuccess

    @POST("agents/{id}/rotate-key")
    suspend fun rotateKey(@Path("id") id: String): AgentKey

    @GET("agents/{id}/key")
    suspend fun key(@Path("id") id: String): AgentKey

    @POST("agents/{id}/test")
    suspend fun test(@Path("id") id: String): AgentTestResponse
}

@Serializable
data class AgentDto(
    val id: String = "",
    val name: String = "",
    val country: String = "",
    @SerialName("public_ip") val publicIp: String = "",
    @SerialName("public_ipv6") val publicIpv6: String = "",
    @SerialName("ipv6_enabled") val ipv6Enabled: Boolean = false,
    @SerialName("relay_port") val relayPort: Int = 0,
    val version: String = "",
    val enabled: Boolean = false,
    @SerialName("created_at") val createdAt: String = "",
    @SerialName("last_seen") val lastSeen: String = "",
    /** Server-computed with the same 90 s window the DNS router uses. */
    val online: Boolean? = null,
    @SerialName("ping_ms") val pingMs: Double = 0.0,
    @SerialName("ping_at") val pingAt: String = "",
    @SerialName("bytes_in") val bytesIn: Long = 0,
    @SerialName("bytes_out") val bytesOut: Long = 0,
)

@Serializable
data class AgentsResponse(
    val agents: List<AgentDto> = emptyList(),
)

@Serializable
data class AgentEnvelope(
    val agent: AgentDto? = null,
)

@Serializable
data class AgentKeyResponse(
    val agent: AgentDto? = null,
    val key: String = "",
)

@Serializable
data class AgentKey(
    val key: String = "",
)

@Serializable
data class AgentCreateRequest(
    val name: String = "",
    val country: String,
)

@Serializable
data class AgentUpdateRequest(
    val name: String = "",
    val country: String,
    @SerialName("public_ip") val publicIp: String = "",
    // Null means "leave unchanged": the server treats an explicit empty address
    // as a clear, so an absent field must not be confused with that. The switch
    // must be nullable so an explicit `false` is sent rather than omitted as a
    // default.
    @SerialName("public_ipv6") val publicIpv6: String? = null,
    @SerialName("ipv6_enabled") val ipv6Enabled: Boolean? = null,
)

/** The result of the real TCP test; [ok] is false with [error] on a failed connect. */
@Serializable
data class AgentTestResponse(
    val ok: Boolean = false,
    val error: String = "",
    @SerialName("ping_ms") val pingMs: Double = 0.0,
    val address: String = "",
    val agent: AgentDto? = null,
)
