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
 * The v3 hub-side egress surface (`internal/api/router_v3_handlers.go`).
 *
 * A SOCKS5 password and a WireGuard configuration are write-only. The list
 * reports `password_set` / `config_set` and never carries the secret itself.
 * Assignment values use `egress:<id>`; existing agent ids are unchanged.
 */
interface EgressesApi {

    @GET("egresses")
    suspend fun list(): EgressesResponse

    @POST("egresses")
    suspend fun create(@Body body: EgressSaveRequest): EgressEnvelope

    @PUT("egresses/{id}")
    suspend fun update(@Path("id") id: String, @Body body: EgressSaveRequest): EgressEnvelope

    @DELETE("egresses/{id}")
    suspend fun delete(@Path("id") id: String): SimpleSuccess

    @POST("egresses/{id}/enable")
    suspend fun enable(@Path("id") id: String): SimpleSuccess

    @POST("egresses/{id}/disable")
    suspend fun disable(@Path("id") id: String): SimpleSuccess

    @POST("egresses/{id}/test")
    suspend fun test(@Path("id") id: String): EgressTestResponse
}

@Serializable
data class EgressDto(
    val id: String = "",
    val kind: String = "socks5",
    val name: String = "",
    val country: String = "",
    val enabled: Boolean = false,
    @SerialName("fail_closed") val failClosed: Boolean = true,
    val address: String = "",
    val username: String = "",
    @SerialName("password_set") val passwordSet: Boolean = false,
    @SerialName("config_set") val configSet: Boolean = false,
    val resolve: String = "remote",
    val health: String = "unknown",
    @SerialName("latency_ms") val latencyMs: Double = 0.0,
    @SerialName("last_error") val lastError: String = "",
    @SerialName("last_seen") val lastSeen: String = "",
    @SerialName("bytes_in") val bytesIn: Long = 0,
    @SerialName("bytes_out") val bytesOut: Long = 0,
)

@Serializable
data class EgressesResponse(
    val egresses: List<EgressDto> = emptyList(),
)

@Serializable
data class EgressEnvelope(
    val egress: EgressDto? = null,
)

/**
 * The create/update body. Null secrets are omitted by the shared Json config, so
 * an edit that leaves the password or config blank keeps the stored value.
 */
@Serializable
data class EgressSaveRequest(
    val kind: String,
    val name: String,
    val country: String = "",
    val address: String? = null,
    val username: String? = null,
    val password: String? = null,
    val config: String? = null,
    val resolve: String? = null,
    @SerialName("fail_closed") val failClosed: Boolean = true,
    val enabled: Boolean = true,
)

@Serializable
data class EgressTestResponse(
    val ok: Boolean = false,
    val error: String = "",
    @SerialName("latency_ms") val latencyMs: Double = 0.0,
)
