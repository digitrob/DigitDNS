package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Query

/** Optional persistent query history. */
interface QueryLogApi {
    @GET("query-log")
    suspend fun search(
        @Query("domain") domain: String? = null,
        @Query("client") client: String? = null,
        @Query("action") action: String? = null,
        @Query("proto") proto: String? = null,
        @Query("limit") limit: Int = 200,
    ): QueryLogResponse

    @GET("query-log/settings")
    suspend fun settings(): QueryLogSettingsEnvelope

    @POST("query-log/settings")
    suspend fun save(@Body body: QueryLogSettingsDto): QueryLogSettingsEnvelope

    @POST("query-log/clear")
    suspend fun clear(): QueryLogClearResponse
}

@Serializable
data class QueryLogResponse(val items: List<QueryLogItemDto> = emptyList(), val count: Int = 0)

@Serializable
data class QueryLogItemDto(
    val timestamp: String = "",
    @SerialName("client_ip") val clientIp: String = "",
    @SerialName("account_name") val accountName: String = "",
    val domain: String = "",
    val protocol: String = "",
    @SerialName("rule_matched") val ruleMatched: String = "",
    val action: String = "",
    @SerialName("latency_ms") val latencyMs: Double = 0.0,
    val cached: Boolean = false,
    val qtype: Int = 0,
    val rcode: Int = 0,
    @SerialName("matched_pattern") val matchedPattern: String = "",
    @SerialName("relay_egress_name") val relayEgressName: String = "",
)

@Serializable
data class QueryLogSettingsEnvelope(val settings: QueryLogSettingsDto = QueryLogSettingsDto())

@Serializable
data class QueryLogSettingsDto(
    val enabled: Boolean = false,
    @SerialName("retention_days") val retentionDays: Int = 7,
    val anonymize: Boolean = true,
    @SerialName("opt_out") val optOut: List<String> = emptyList(),
    @SerialName("max_mb") val maxMb: Int = 256,
)

@Serializable
data class QueryLogClearResponse(val ok: Boolean = false)
