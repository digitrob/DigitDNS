package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.http.GET

/**
 * The v3 live telemetry surface.
 *
 * `GET /stats` returns the same `LiveStatsResponse` the panel's dashboard tiles read
 * (`internal/service/stats_service.go`). The app renders a subset of it — the tiles a
 * phone screen fits — but the DTO declares the fields it uses, and unknown members
 * are ignored, so the server may add a metric without breaking this client.
 */
interface StatsApi {

    @GET("stats")
    suspend fun live(): LiveStats
}

@Serializable
data class LiveStats(
    @SerialName("total_queries") val totalQueries: Long = 0,
    val qps: Double = 0.0,
    @SerialName("active_relays") val activeRelays: Long = 0,
    @SerialName("total_relays") val totalRelays: Long = 0,
    @SerialName("bytes_sent") val bytesSent: Long = 0,
    @SerialName("bytes_recv") val bytesRecv: Long = 0,
    @SerialName("cache_items") val cacheItems: Int = 0,
    @SerialName("cache_hits") val cacheHits: Long = 0,
    @SerialName("cache_misses") val cacheMisses: Long = 0,
    @SerialName("cache_hit_rate") val cacheHitRate: Double = 0.0,
    @SerialName("cpu_usage_percent") val cpuUsagePercent: Double = 0.0,
    @SerialName("ram_usage_mb") val ramUsageMb: Double = 0.0,
    @SerialName("num_cpu") val numCpu: Int = 0,
    @SerialName("num_goroutines") val numGoroutines: Int = 0,
    @SerialName("speed_in_kbps") val speedInKbps: Double = 0.0,
    @SerialName("speed_out_kbps") val speedOutKbps: Double = 0.0,
    @SerialName("uptime_sec") val uptimeSec: Long = 0,
    @SerialName("system_cpu_percent") val systemCpuPercent: Double = 0.0,
    @SerialName("system_mem_used_mb") val systemMemUsedMb: Double = 0.0,
    @SerialName("system_mem_total_mb") val systemMemTotalMb: Double = 0.0,
    @SerialName("system_mem_percent") val systemMemPercent: Double = 0.0,
    // The per-source rate limiter: how many were dropped, and the limit in force.
    @SerialName("rate_limited") val rateLimited: Long = 0,
    @SerialName("rate_limit_qps") val rateLimitQps: Int = 0,
    // SNI proxy connections that never became relays.
    @SerialName("relays_refused") val relaysRefused: Long = 0,
    @SerialName("relays_unreadable") val relaysUnreadable: Long = 0,
    // Cache serve-stale / background refresh health.
    @SerialName("stale_served") val staleServed: Long = 0,
    @SerialName("refresh_started") val refreshStarted: Long = 0,
    @SerialName("refresh_failed") val refreshFailed: Long = 0,
    @SerialName("refresh_dropped") val refreshDropped: Long = 0,
    // Query service-time distributions: all queries, and cache-misses only.
    val latency: LatencySnapshot = LatencySnapshot(),
    @SerialName("latency_uncached") val latencyUncached: LatencySnapshot = LatencySnapshot(),
) {
    /** Total bytes moved: what the panel's "Live Traffic" footer shows. */
    val totalBytesTransferred: Long get() = bytesSent + bytesRecv
}

/** Service-time percentiles in milliseconds, from `internal/service/latency.go`. */
@Serializable
data class LatencySnapshot(
    val count: Long = 0,
    @SerialName("avg_ms") val avgMs: Double = 0.0,
    @SerialName("p50_ms") val p50Ms: Double = 0.0,
    @SerialName("p95_ms") val p95Ms: Double = 0.0,
    @SerialName("p99_ms") val p99Ms: Double = 0.0,
    @SerialName("max_ms") val maxMs: Double = 0.0,
) {
    /** True when at least one observation has been recorded, so a dash is honest. */
    val hasData: Boolean get() = count > 0
}
