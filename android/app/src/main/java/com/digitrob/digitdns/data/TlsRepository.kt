package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.TlsApi
import com.digitrob.digitdns.core.network.TlsAutoRenewRequest
import com.digitrob.digitdns.core.network.TlsCertInstallRequest
import com.digitrob.digitdns.core.network.TlsCertInstallResponse
import com.digitrob.digitdns.core.network.TlsCertResponse
import com.digitrob.digitdns.core.network.TlsIssueRequest
import com.digitrob.digitdns.domain.model.AcmeRun
import javax.inject.Inject
import javax.inject.Singleton

/** The outcome of a certificate issue request. */
data class TlsIssueResult(
    val success: Boolean,
    val domain: String,
    /** True when a run started; poll [TlsRepository.acmeStatus]. */
    val issuing: Boolean,
    val detail: String,
)

/** The outcome of an operator-supplied certificate install. */
data class TlsCertInstallResult(
    val success: Boolean,
    val purpose: String,
    val domain: String,
    val certPath: String,
    val keyPath: String,
    val detail: String,
)

/**
 * TLS certificate issuance. There is no certificate *state* here — issuance is an
 * action, and the run progress comes from [acmeStatus]. Whether the panel is HTTPS
 * and its configured domain come from the settings TLS snapshot.
 */
@Singleton
class TlsRepository @Inject constructor(
    private val api: TlsApi,
) {
    suspend fun issue(
        domain: String,
        email: String,
        purpose: String,
        force: Boolean = false,
    ): TlsIssueResult = issue(
        TlsIssueRequest(domain = domain, email = email, purpose = purpose, force = force),
    )

    suspend fun issue(request: TlsIssueRequest): TlsIssueResult {
        val response = api.issue(request)
        return TlsIssueResult(response.success, response.domain, response.issuing, response.detail)
    }

    suspend fun installCertificate(request: TlsCertInstallRequest): TlsCertInstallResult {
        val response: TlsCertInstallResponse = api.installCertificate(request)
        return TlsCertInstallResult(
            success = response.success,
            purpose = response.purpose,
            domain = response.domain,
            certPath = response.certPath,
            keyPath = response.keyPath,
            detail = response.detail,
        )
    }

    suspend fun certificateSources(): TlsCertResponse = api.certificateSources()

    /** Toggle DoT/DoH ACME auto-renew. Returns the flag the server now holds. */
    suspend fun setAutoRenew(enabled: Boolean): Boolean = api.autoRenew(TlsAutoRenewRequest(enabled)).enabled

    suspend fun acmeStatus(): AcmeRun {
        val dto = api.acmeStatus()
        return AcmeRun(
            running = dto.running,
            purpose = dto.purpose,
            domain = dto.domain,
            stage = dto.stage,
            progress = dto.progress,
            detail = dto.detail,
            error = dto.error,
            lastDetail = dto.lastDetail,
            lastRunAt = dto.lastRunAt,
        )
    }
}
