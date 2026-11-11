package com.digitrob.digitdns.core.network

import kotlinx.serialization.Serializable
import kotlinx.serialization.SerialName
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST

/**
 * The upstream resolver pool: an ordered list of addresses, most-preferred first.
 *
 * There is no upstream object — the pool is a plain `[]string` in preference order,
 * and the protocol is encoded in the address itself (`8.8.8.8:53` is UDP/TCP,
 * `tls://dns.google` is DoT, `https://cloudflare-dns.com/dns-query` is DoH). The
 * server canonicalises whatever is typed into one of those forms on add.
 *
 * **Delete** exists on the v3 surface as `POST /upstreams/delete` with the address in
 * the body (POST, not DELETE, because the address is the payload and removing the
 * pool's last working resolver is not a safe verb).
 */
interface UpstreamsApi {

    @GET("upstreams")
    suspend fun list(): UpstreamsResponse

    /**
     * `GET /upstreams/stats`. Per-upstream latency/health, sorted fastest-first —
     * the same snapshot the panel's resolver-racer card reads.
     */
    @GET("upstreams/stats")
    suspend fun stats(): UpstreamStatsResponse

    @POST("upstreams")
    suspend fun add(@Body body: UpstreamAddRequest): UpstreamsResponse

    /**
     * `GET /upstreams/presets`. The curated public-provider catalogue, shared
     * with the web add-resolver browser. Provider logos are bundled; only the
     * stable provider id comes over the wire.
     */
    @GET("upstreams/presets")
    suspend fun presets(): UpstreamPresetsResponse

    /** `POST /upstreams/delete`. Removes the canonical match for [body]'s address. */
    @POST("upstreams/delete")
    suspend fun delete(@Body body: UpstreamDeleteRequest): UpstreamsResponse

    /**
     * `POST /upstreams/reorder`. `order` must list **every** current upstream exactly
     * once; a shorter list, a duplicate, or an unknown entry is a 400.
     */
    @POST("upstreams/reorder")
    suspend fun reorder(@Body body: UpstreamReorderRequest): UpstreamsResponse

    @POST("upstreams/benchmark")
    suspend fun benchmark(): BenchmarkResponse
}

@Serializable
data class UpstreamsResponse(
    val items: List<String> = emptyList(),
)

@Serializable
data class UpstreamStatsResponse(
    val items: List<UpstreamStat> = emptyList(),
)

/**
 * One row of `GET /upstreams/stats`, from `internal/core/upstream/racer.go`.
 *
 * `latency` is nanoseconds (the field the panel divides by 1e6); `latencyMs` is the
 * server's own pre-divided value, so a client should prefer it and fall back to the
 * nanosecond field only if it is absent.
 */
@Serializable
data class UpstreamStat(
    val address: String = "",
    val latency: Long = 0,
    @SerialName("latency_ms") val latencyMs: Double = 0.0,
    @SerialName("latency_p50_ms") val latencyP50Ms: Double = 0.0,
    @SerialName("latency_p95_ms") val latencyP95Ms: Double = 0.0,
    @SerialName("measured") val measured: Boolean? = null,
    @SerialName("latency_is_probe") val latencyIsProbe: Boolean = false,
    @SerialName("samples") val samples: Long = 0,
    val hits: Long = 0,
    val errors: Long = 0,
    @SerialName("transport_errors") val transportErrors: Long = 0,
    @SerialName("policy_errors") val policyErrors: Long = 0,
    @SerialName("recent_samples") val recentSamples: Int = 0,
    @SerialName("recent_error_rate") val recentErrorRate: Double = 0.0,
    @SerialName("consecutive_failures") val consecutiveFailures: Int = 0,
    @SerialName("circuit_open") val circuitOpen: Boolean = false,
    @SerialName("last_error") val lastError: String = "",
    @SerialName("last_error_at") val lastErrorAt: String = "",
    @SerialName("probe_hits") val probeHits: Long = 0,
    @SerialName("probe_errors") val probeErrors: Long = 0,
    @SerialName("probe_policy") val probePolicy: Long = 0,
    @SerialName("last_probe_ms") val lastProbeMs: Double = 0.0,
    @SerialName("last_probe_ok") val lastProbeOk: Boolean = false,
    @SerialName("last_probe_at") val lastProbeAt: String = "",
) {
    /** Milliseconds to show, preferring the server's value. */
    val displayLatencyMs: Double get() = if (latencyMs > 0.0) latencyMs else latency / 1_000_000.0

    /**
     * Whether the racer has actually timed this upstream. The server's `measured`
     * flag is authoritative; the numeric fallback keeps an older server working.
     * A freshly added resolver, or one that has never answered, has no measurement —
     * showing it as "0.0 ms, fast" would dress an untested entry in the green that
     * means "fastest". Matches the panel, which treats only a finite, positive
     * latency as measured.
     */
    val isMeasured: Boolean get() = measured
        ?: (displayLatencyMs.isFinite() && displayLatencyMs > 0.0)

    /**
     * Current health, not lifetime history: an open circuit, a failure streak, or
     * a high error rate in the recent window. A timeout an hour ago does not make a
     * working resolver look broken.
     */
    val isHealthy: Boolean get() = !circuitOpen &&
        consecutiveFailures == 0 &&
        !(recentSamples > 0 && recentErrorRate >= 0.1)
}

@Serializable
data class UpstreamPresetsResponse(
    val providers: List<UpstreamPresetProvider> = emptyList(),
)

@Serializable
data class UpstreamPresetProvider(
    val id: String = "",
    val name: String = "",
    val note: String = "",
    val servers: List<UpstreamPresetServer> = emptyList(),
)

@Serializable
data class UpstreamPresetServer(
    val address: String = "",
    val label: String = "",
    val protocol: String = "",
)

@Serializable
data class UpstreamAddRequest(
    val address: String,
)

@Serializable
data class UpstreamDeleteRequest(
    val address: String,
)

@Serializable
data class UpstreamReorderRequest(
    val order: List<String>,
)

@Serializable
data class BenchmarkResponse(
    val started: Boolean = false,
    val running: Boolean = false,
    val message: String = "",
)
