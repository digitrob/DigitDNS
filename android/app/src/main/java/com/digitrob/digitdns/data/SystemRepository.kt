package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.SystemApi
import com.digitrob.digitdns.domain.model.UpdateProgressView
import com.digitrob.digitdns.domain.model.UpdateStatus
import javax.inject.Inject
import javax.inject.Singleton

/** The install's health summary: status string, version and exposure. */
data class SystemSummary(
    val status: String,
    val version: String,
    val display: String,
    val channel: String,
    val commit: String,
    val publicIp: String,
    val apiExposure: String,
    val apiBindList: List<String>,
)

/**
 * The install's own state. Update checking and applying are separate on purpose: the
 * check is cheap and may be polled, the apply starts a run that replaces the binary
 * and restarts the process.
 */
@Singleton
class SystemRepository @Inject constructor(
    private val api: SystemApi,
) {
    suspend fun summary(): SystemSummary {
        val dto = api.status()
        val version = dto.version
        return SystemSummary(
            status = dto.status,
            version = version?.version ?: "",
            display = version?.display ?: "",
            channel = version?.channel ?: "",
            commit = version?.commit ?: "",
            publicIp = dto.publicIp,
            apiExposure = dto.apiExposure,
            apiBindList = dto.apiBindList,
        )
    }

    suspend fun checkUpdate(): UpdateStatus = UpdateStatus.from(api.checkUpdate())

    suspend fun applyUpdate(): Boolean = api.applyUpdate().started

    suspend fun updateProgress(): UpdateProgressView = UpdateProgressView.from(api.updateStatus())

    /** Flush the resolver cache. Returns the server's acknowledgement flag. */
    suspend fun flushCache(): Boolean = api.flushCache().flushed
}
