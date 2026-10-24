package com.digitrob.digitdns.domain.model

import com.digitrob.digitdns.core.network.DiagnosticResultDto
import com.digitrob.digitdns.core.network.DiagnosticTargetDto
import com.digitrob.digitdns.core.network.ServerSettings
import com.digitrob.digitdns.core.network.SubscriptionSnapshot
import com.digitrob.digitdns.core.network.TlsSnapshot
import com.digitrob.digitdns.core.network.UpdateCheck
import com.digitrob.digitdns.core.network.UpdateProgress

/**
 * The Phase 4 domain shapes. These are thin views of the DTOs — the server does the
 * computing — but a few carry meaning worth naming once: whether an update can
 * actually be applied here, whether an ACME run is mid-flight, and whether the
 * subscription portal is reachable at the computed origin.
 */

/** A diagnostic probe target. */
data class DiagnosticTarget(val name: String, val host: String) {
    companion object {
        fun from(dto: DiagnosticTargetDto) = DiagnosticTarget(dto.name, dto.host)
    }
}

/** One probe's result, with its latency in milliseconds. */
data class DiagnosticResult(
    val name: String,
    val target: String,
    val success: Boolean,
    val latencyMs: Double,
) {
    companion object {
        fun from(dto: DiagnosticResultDto) = DiagnosticResult(dto.name, dto.target, dto.success, dto.latencyMs)
    }
}

/** The outcome of a diagnostics run, including the overall grade. */
data class DiagnosticReport(
    val overallScore: Int,
    val overallQuality: String,
    val avgLatencyMs: Double,
    val reachable: Int,
    val total: Int,
    val results: List<DiagnosticResult>,
    val timestamp: String,
)

/** The server settings view, with the API key and admin path the operator may rotate. */
data class ServerConfig(
    val publicIp: String,
    val bindHost: String,
    val webPort: Int,
    val adminUsername: String,
    val adminPasswordWeak: Boolean,
    val apiKey: String,
    val apiBind: String,
    val apiBindList: List<String>,
    val sessionIdleMinutes: Int,
    val adminPath: String,
    /** ISO 3166-1 alpha-2 country for [publicIp], from the server's GeoIP database. */
    val country: String = "",
) {
    companion object {
        fun from(dto: ServerSettings) = ServerConfig(
            publicIp = dto.publicIp,
            bindHost = dto.bindHost,
            webPort = dto.webPort,
            adminUsername = dto.adminUsername,
            adminPasswordWeak = dto.adminPasswordWeak,
            apiKey = dto.apiKey,
            apiBind = dto.apiBind,
            apiBindList = dto.apiBindList,
            sessionIdleMinutes = dto.sessionIdleMinutes,
            adminPath = dto.adminPath,
            country = dto.country,
        )
    }
}

/** The TLS settings snapshot the settings screen shows. */
data class TlsSettings(
    val domain: String,
    val email: String,
    val autoRenewAcme: Boolean,
    val dotDomain: String,
    val panelHttps: Boolean,
    val dotAutoRenewAcme: Boolean,
) {
    companion object {
        fun from(dto: TlsSnapshot) = TlsSettings(
            domain = dto.domain,
            email = dto.email,
            autoRenewAcme = dto.autoRenewAcme,
            dotDomain = dto.dotDomain,
            panelHttps = dto.panelHttps,
            dotAutoRenewAcme = dto.dotAutoRenewAcme,
        )
    }
}

/** The subscription portal settings. */
data class SubscriptionSettings(
    val enabled: Boolean,
    val domain: String,
    val port: Int,
    val uriPath: String,
    val title: String,
    val themeCss: String,
    val themeCssSource: String,
    val themeCssPath: String,
    val themeCssUrl: String,
    val usePanelCertificate: Boolean,
    val autoRenewAcme: Boolean,
) {
    companion object {
        fun from(dto: SubscriptionSnapshot) = SubscriptionSettings(
            enabled = dto.enabled,
            domain = dto.domain,
            port = dto.port,
            uriPath = dto.uriPath,
            title = dto.title,
            themeCss = dto.themeCss,
            themeCssSource = dto.themeCssSource,
            themeCssPath = dto.themeCssPath,
            themeCssUrl = dto.themeCssUrl,
            usePanelCertificate = dto.usePanelCertificate,
            autoRenewAcme = dto.autoRenewAcme,
        )
    }
}

/** The self-update check. [error] is set when the check itself could not run. */
data class UpdateStatus(
    val current: String,
    val latest: String,
    val updateAvailable: Boolean,
    val supported: Boolean,
    val note: String,
    val error: String,
) {
    /** Whether the "Apply" action makes sense: an update exists and can be applied here. */
    val canApply: Boolean get() = updateAvailable && supported && error.isBlank()

    companion object {
        fun from(dto: UpdateCheck) = UpdateStatus(
            current = dto.current,
            latest = dto.latest,
            updateAvailable = dto.updateAvailable,
            supported = dto.supported,
            note = dto.note,
            error = dto.error,
        )
    }
}

/** The progress of a running self-update. */
data class UpdateProgressView(
    val running: Boolean,
    val phase: String,
    val percent: Int,
    val message: String,
    val restarting: Boolean,
    val error: String,
) {
    companion object {
        fun from(dto: UpdateProgress) = UpdateProgressView(
            running = dto.running,
            phase = dto.phase,
            percent = dto.percent,
            message = dto.message,
            restarting = dto.restarting,
            error = dto.error,
        )
    }
}

/** The ACME issuance run state. */
data class AcmeRun(
    val running: Boolean,
    val purpose: String,
    val domain: String,
    val stage: String,
    val progress: Int,
    val detail: String,
    val error: String,
    val lastDetail: String,
    val lastRunAt: String,
)
