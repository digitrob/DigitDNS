package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Query

/**
 * The v3 historical aggregate surface.
 *
 * These are the same routes the web panel's Traffic History cards read. The
 * Android client consumes the already-aggregated counters; it never receives a
 * raw query log, and the distinct-client figure is the server's HLL estimate
 * rather than a local calculation.
 */
interface HistoryApi {

    /**
     * One range's summary, series and every ranked table in one response. The
     * app used to fan out to eight endpoints on every refresh; the combined
     * route removes seven round trips and one JSON envelope per call.
     */
    @GET("stats/history")
    suspend fun history(
        @Query("range") range: String,
        @Query("interval") interval: String? = null,
        @Query("limit") limit: Int,
    ): StatsHistory

    @GET("stats/settings")
    suspend fun settings(): StatsSettings

    @POST("stats/settings")
    suspend fun saveSettings(@Body body: StatsSettingsSave): StatsSettingsResponse
}

@Serializable
data class StatsSummary(
    val range: String = "",
    val from: String = "",
    val to: String = "",
    val queries: Long = 0,
    val blocked: Long = 0,
    val proxied: Long = 0,
    val cached: Long = 0,
    val failed: Long = 0,
    @SerialName("blocked_pct") val blockedPct: Double = 0.0,
    @SerialName("proxied_pct") val proxiedPct: Double = 0.0,
    @SerialName("cache_hit_pct") val cacheHitPct: Double = 0.0,
    @SerialName("active_clients") val activeClients: Int = 0,
    /** True only when the range includes pre-HLL buckets; see the server docs. */
    @SerialName("active_clients_approx") val activeClientsApprox: Boolean = false,
    @SerialName("latency_avg_ms") val latencyAvgMs: Double = 0.0,
    val buckets: Int = 0,
)

@Serializable
data class StatsSeries(
    val range: String = "",
    val interval: String = "",
    val points: List<StatsSeriesPoint> = emptyList(),
)

@Serializable
data class StatsSeriesPoint(
    val start: String = "",
    val queries: Long = 0,
    val blocked: Long = 0,
    val proxied: Long = 0,
    val cached: Long = 0,
    val failed: Long = 0,
    @SerialName("latency_ms") val latencyMs: Double = 0.0,
) {
    /** Everything the stacked chart draws as "allowed": neither blocked nor proxied. */
    val allowed: Long get() = (queries - blocked - proxied).coerceAtLeast(0)
}

@Serializable
data class StatsHistory(
    val range: String = "",
    val summary: StatsSummary = StatsSummary(),
    val series: StatsSeries = StatsSeries(),
    val tops: Map<String, StatsTop> = emptyMap(),
)

@Serializable
data class StatsTop(
    val range: String = "",
    val dimension: String = "",
    val items: List<StatsTopItem> = emptyList(),
    @SerialName("other_count") val otherCount: Long = 0,
)

@Serializable
data class StatsTopItem(
    val value: String = "",
    val count: Long = 0,
)

/** The server-wide aggregate policy, shared by the panel and this client. */
@Serializable
data class StatsSettings(
    val enabled: Boolean = true,
    @SerialName("retention_days") val retentionDays: Int = 90,
    @SerialName("top_domains") val topDomains: Int = 1000,
    @SerialName("top_clients") val topClients: Int = 500,
    @SerialName("anonymize_clients") val anonymizeClients: Boolean = false,
    // The server encodes a nil Go slice as JSON null; absence is also valid.
    @SerialName("exclude_clients") val excludeClients: List<String>? = null,
)

/**
 * The save body deliberately has no property defaults. kotlinx.serialization omits
 * default-valued properties, and a missing `retention_days` would reach the server
 * as zero — which its validator rejects. Every field is therefore always emitted,
 * including an empty `exclude_clients` list.
 */
@Serializable
data class StatsSettingsSave(
    val enabled: Boolean,
    @SerialName("retention_days") val retentionDays: Int,
    @SerialName("top_domains") val topDomains: Int,
    @SerialName("top_clients") val topClients: Int,
    @SerialName("anonymize_clients") val anonymizeClients: Boolean,
    @SerialName("exclude_clients") val excludeClients: List<String>,
)

@Serializable
data class StatsSettingsResponse(
    val success: Boolean = false,
    @SerialName("restart_required") val restartRequired: Boolean = false,
    val settings: StatsSettings? = null,
)
