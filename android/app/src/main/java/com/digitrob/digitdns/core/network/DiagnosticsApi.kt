package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Query

/**
 * Reachability diagnostics: a list of named probe targets and a run that dials them.
 *
 * The list uses a `targets` envelope — **not** the `items` convention the other v3
 * lists use — so a DTO reading `items` would silently see an empty list. Add returns
 * the whole refreshed snapshot (200, not 201), matching the delete.
 *
 * [run] takes no parameters: the body is ignored server-side and the probe is a fixed
 * TCP connect (2.5s timeout). It reports a score and per-target latency.
 */
interface DiagnosticsApi {

    @GET("diagnostics/targets")
    suspend fun targets(): DiagnosticsTargetsResponse

    @POST("diagnostics/targets")
    suspend fun addTarget(@Body body: DiagnosticTargetDto): DiagnosticsTargetsResponse

    @DELETE("diagnostics/targets")
    suspend fun deleteTarget(@Query("host") host: String): DiagnosticsTargetsResponse

    @POST("diagnostics/run")
    suspend fun run(): DiagnosticReport

    /** The resolver's ordered decision for one name, optionally for a client IP. */
    @GET("explain")
    suspend fun explain(
        @Query("domain") domain: String,
        @Query("client") client: String? = null,
    ): ExplainDto
}

@Serializable
data class ExplainDto(
    val domain: String = "",
    @SerialName("client_ip") val clientIp: String = "",
    val account: String = "",
    val engine: Boolean = true,
    val action: String = "",
    val rule: String = "",
    val pattern: String = "",
    val source: String = "",
    @SerialName("blocklist_source") val blocklistSource: String = "",
    @SerialName("blocklist_pattern") val blocklistPattern: String = "",
    @SerialName("allow_source") val allowSource: String = "",
    @SerialName("allow_pattern") val allowPattern: String = "",
    @SerialName("suppressed_source") val suppressedSource: String = "",
    @SerialName("suppressed_pattern") val suppressedPattern: String = "",
    @SerialName("custom_record") val customRecord: String = "",
    val target: ExplainTargetDto? = null,
)

@Serializable
data class ExplainTargetDto(
    val kind: String = "",
    @SerialName("agent_id") val agentId: String = "",
    val name: String = "",
    val country: String = "",
    @SerialName("egress_id") val egressId: String = "",
    @SerialName("egress_kind") val egressKind: String = "",
    val healthy: Boolean = true,
    @SerialName("fail_closed") val failClosed: Boolean = false,
    @SerialName("fallback_reason") val fallbackReason: String = "",
)

@Serializable
data class DiagnosticsTargetsResponse(
    val targets: List<DiagnosticTargetDto> = emptyList(),
)

@Serializable
data class DiagnosticTargetDto(
    val name: String = "",
    val host: String = "",
)

@Serializable
data class DiagnosticReport(
    @SerialName("overall_score") val overallScore: Int = 0,
    @SerialName("overall_quality") val overallQuality: String = "",
    @SerialName("avg_latency_ms") val avgLatencyMs: Double = 0.0,
    val reachable: Int = 0,
    val total: Int = 0,
    val results: List<DiagnosticResultDto> = emptyList(),
    val timestamp: String = "",
)

@Serializable
data class DiagnosticResultDto(
    val name: String = "",
    val target: String = "",
    val success: Boolean = false,
    @SerialName("latency_ms") val latencyMs: Double = 0.0,
)
