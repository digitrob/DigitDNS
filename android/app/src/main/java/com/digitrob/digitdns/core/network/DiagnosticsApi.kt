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
}

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
