package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * The subscriber portal's JSON, as `/api/sub/<token>` serves it. The names mirror the
 * server's JSON; the app only reads display data.
 */
@Serializable
data class SubscriberStatusDto(
    val success: Boolean = false,
    val client: SubscriberClientDto = SubscriberClientDto(),
    @SerialName("detected_ip") val detectedIp: String = "",
    @SerialName("server_dns") val serverDns: String = "",
    @SerialName("server_host") val serverHost: String = "",
    @SerialName("has_domain") val hasDomain: Boolean = false,
    @SerialName("doh_port") val dohPort: Int = 0,
    @SerialName("dot_port") val dotPort: Int = 0,
    @SerialName("detected_ip_registered") val detectedIpRegistered: Boolean = false,
    /** active | suspended | expired | quota, worked out by the server. */
    val status: String = "active",
    @SerialName("quota_exceeded") val quotaExceeded: Boolean = false,
    @SerialName("next_traffic_reset") val nextTrafficReset: String? = null,
)

@Serializable
data class SubscriberClientDto(
    val id: String = "",
    val uuid: String = "",
    val name: String = "",
    @SerialName("allowed_ips") val allowedIps: List<String> = emptyList(),
    @SerialName("max_devices") val maxDevices: Int = 0,
    @SerialName("traffic_limit_gb") val trafficLimitGb: Double = 0.0,
    @SerialName("traffic_used_bytes") val trafficUsedBytes: Long = 0,
    @SerialName("expires_at") val expiresAt: String = "",
    val enabled: Boolean = true,
    @SerialName("custom_policies") val customPolicies: List<String> = emptyList(),
    @SerialName("traffic_reset_cycle") val trafficResetCycle: String = "",
)

@Serializable
data class SubscriberRegisterRequest(
    val secret: String,
    val ip: String? = null,
)

@Serializable
data class SubscriberRegisterResponse(
    val success: Boolean = false,
    @SerialName("already_bound") val alreadyBound: Boolean = false,
    @SerialName("bound_ip") val boundIp: String = "",
    @SerialName("detected_ip") val detectedIp: String = "",
    val client: SubscriberClientDto? = null,
    val error: String? = null,
    val reason: String? = null,
)

/** The error shape the portal's write endpoint uses. */
@Serializable
data class SubscriberErrorDto(
    val error: String = "",
    val reason: String = "",
)
