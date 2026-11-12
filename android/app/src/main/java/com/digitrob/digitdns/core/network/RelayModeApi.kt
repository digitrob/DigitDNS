package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Path

/** Full-instance relay mode (`/api/v3/relay-mode`). */
interface RelayModeApi {

    @GET("relay-mode")
    suspend fun get(): RelayModeResponse

    @POST("relay-mode")
    suspend fun save(@Body body: RelayModeRequest): RelayModeResponse

    @POST("relay-mode/global")
    suspend fun setGlobal(@Body body: RelayModeGlobalRequest): RelayModeResponse

    @POST("relay-mode/test")
    suspend fun test(): RelayModeTestResponse

    @DELETE("relay-mode/{id}")
    suspend fun delete(@Path("id") id: String): RelayModeResponse

    @POST("relay-mode/{id}/test")
    suspend fun testHub(@Path("id") id: String): RelayModeTestResponse
}

@Serializable
data class RelayModeDto(
    val id: String = "",
    val name: String = "",
    val enabled: Boolean = false,
    @SerialName("hub_url") val hubUrl: String = "",
    @SerialName("agent_id") val agentId: String = "",
    @SerialName("agent_key_set") val agentKeySet: Boolean = false,
    val country: String = "",
    @SerialName("ipv6_enabled") val ipv6Enabled: Boolean = false,
    val hosts: Int = 0,
    val clients: Int = 0,
    val online: Boolean = false,
    @SerialName("last_sync") val lastSync: String = "",
    @SerialName("last_heartbeat") val lastHeartbeat: String = "",
    @SerialName("last_error") val lastError: String = "",
    @SerialName("relay_bytes_in") val relayBytesIn: Long = 0,
    @SerialName("relay_bytes_out") val relayBytesOut: Long = 0,
    @SerialName("relay_port") val relayPort: Int = 443,
)

@Serializable
data class RelayModeResponse(
    @SerialName("relay_mode") val relayMode: RelayModeDto = RelayModeDto(),
    @SerialName("relay_hubs") val relayHubs: List<RelayModeDto> = emptyList(),
    @SerialName("global_enabled") val globalEnabled: Boolean = true,
)

@Serializable
data class RelayModeGlobalRequest(val enabled: Boolean)

@Serializable
data class RelayModeTestResponse(
    val ok: Boolean = false,
    @SerialName("latency_ms") val latencyMs: Double = 0.0,
    val error: String = "",
)

@Serializable
data class RelayModeRequest(
    val id: String? = null,
    val name: String? = null,
    val enabled: Boolean,
    @SerialName("hub_url") val hubUrl: String,
    @SerialName("agent_key") val agentKey: String? = null,
    val token: String? = null,
)
