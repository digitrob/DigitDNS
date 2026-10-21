package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.TlsApi
import com.digitrob.digitdns.core.network.TlsAutoRenewRequest
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

/**
 * TLS certificate issuance. There is no certificate *state* here — issuance is an
 * action, and the run progress comes from [acmeStatus]. Whether the panel is HTTPS
 * and its configured domain come from the settings TLS snapshot.
 */
@Singleton
class TlsRepository @Inject constructor(
    private val api: TlsApi,
) {
    suspend fun issue(domain: String, email: String, purpose: String, force: Boolean = false): TlsIssueResult {
        val response = api.issue(TlsIssueRequest(domain = domain, email = email, purpose = purpose, force = force))
        return TlsIssueResult(response.success, response.domain, response.issuing, response.detail)
    }

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
