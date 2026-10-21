package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.DiagnosticTargetDto
import com.digitrob.digitdns.core.network.DiagnosticsApi
import com.digitrob.digitdns.domain.model.DiagnosticReport
import com.digitrob.digitdns.domain.model.DiagnosticResult
import com.digitrob.digitdns.domain.model.DiagnosticTarget
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Reachability diagnostics. Add and delete both return the full refreshed target list,
 * so the repository returns it rather than making the caller re-fetch.
 */
@Singleton
class DiagnosticsRepository @Inject constructor(
    private val api: DiagnosticsApi,
) {
    suspend fun targets(): List<DiagnosticTarget> = api.targets().targets.map(DiagnosticTarget::from)

    suspend fun addTarget(name: String, host: String): List<DiagnosticTarget> =
        api.addTarget(DiagnosticTargetDto(name = name, host = host)).targets.map(DiagnosticTarget::from)

    suspend fun deleteTarget(host: String): List<DiagnosticTarget> =
        api.deleteTarget(host).targets.map(DiagnosticTarget::from)

    suspend fun run(): DiagnosticReport {
        val dto = api.run()
        return DiagnosticReport(
            overallScore = dto.overallScore,
            overallQuality = dto.overallQuality,
            avgLatencyMs = dto.avgLatencyMs,
            reachable = dto.reachable,
            total = dto.total,
            results = dto.results.map(DiagnosticResult::from),
            timestamp = dto.timestamp,
        )
    }
}
