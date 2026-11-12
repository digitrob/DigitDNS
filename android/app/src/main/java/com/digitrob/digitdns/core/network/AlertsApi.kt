package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST

/** Server-side alert channels and event settings. */
interface AlertsApi {
    @GET("alerts")
    suspend fun get(): AlertsEnvelope

    @POST("alerts")
    suspend fun save(@Body body: AlertsDto): AlertsEnvelope

    @POST("alerts/test")
    suspend fun test(): AlertTestResponse
}

@Serializable
data class AlertsEnvelope(
    val alerts: AlertsDto = AlertsDto(),
    val history: List<AlertDeliveryDto> = emptyList(),
)

@Serializable
data class AlertsDto(
    val enabled: Boolean = false,
    @SerialName("cooldown_minutes") val cooldownMinutes: Int = 15,
    val events: Map<String, Boolean> = emptyMap(),
    val thresholds: AlertThresholdsDto = AlertThresholdsDto(),
    val channels: List<AlertChannelDto> = emptyList(),
)

@Serializable
data class AlertThresholdsDto(
    @SerialName("agent_offline_minutes") val agentOfflineMinutes: Int = 2,
    @SerialName("egress_offline_minutes") val egressOfflineMinutes: Int = 2,
    @SerialName("cert_expiry_days") val certExpiryDays: Int = 30,
    @SerialName("upstream_error_percent") val upstreamErrorPercent: Int = 50,
)

@Serializable
data class AlertDeliveryDto(
    val time: String = "",
    val channel: String = "",
    val event: String = "",
    val title: String = "",
    val ok: Boolean = false,
    val attempts: Int = 0,
    val error: String = "",
)

@Serializable
data class AlertChannelDto(
    val id: String = "",
    val kind: String = "ntfy",
    val name: String = "",
    val url: String = "",
    val token: String = "",
    @SerialName("chat_id") val chatId: String = "",
    val enabled: Boolean = true,
)

@Serializable
data class AlertTestResponse(
    val ok: Boolean = false,
    val errors: List<String> = emptyList(),
)
