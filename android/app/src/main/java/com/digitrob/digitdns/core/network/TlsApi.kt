package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST

/**
 * TLS certificate issuance via the built-in ACME client.
 *
 * [issue] is purpose-scoped: `panel` (the dashboard itself), `subscription` (the
 * subscriber portal), or `dot` (DNS-over-TLS). It returns an `issuing` flag — true
 * when a run has started and the caller should poll [acmeStatus], false when a valid
 * certificate was already on disk and simply applied.
 *
 * [autoRenew] toggles only the **DoT/DoH** renewal flag despite the route name; the
 * panel's own `auto_renew_acme` is part of the settings TLS snapshot.
 *
 * [acmeStatus] reports the in-progress run: `running`, a coarse `stage` from the ACME
 * state machine, `progress` 0–100, and human `detail`/`error`. There is no `staging`
 * or `issuer` field anywhere in this surface; certificate validity lives in the
 * settings TLS snapshot.
 */
interface TlsApi {

    @POST("tls/issue")
    suspend fun issue(@Body body: TlsIssueRequest): TlsIssueResponse

    @POST("tls/auto-renew")
    suspend fun autoRenew(@Body body: TlsAutoRenewRequest): TlsAutoRenewResponse

    @GET("tls/acme/status")
    suspend fun acmeStatus(): AcmeStatus
}

@Serializable
data class TlsIssueRequest(
    val domain: String,
    val email: String = "",
    /** `panel` | `subscription` | `dot`. Defaults to panel server-side. */
    val purpose: String = "panel",
    /** When true, skips the server's fast-path and forces a fresh ACME order. */
    val force: Boolean = false,
)

@Serializable
data class TlsIssueResponse(
    val success: Boolean = false,
    val domain: String = "",
    /** True when a certificate run just started; poll [TlsApi.acmeStatus]. */
    val issuing: Boolean = false,
    val detail: String = "",
)

@Serializable
data class TlsAutoRenewRequest(
    val enabled: Boolean,
)

@Serializable
data class TlsAutoRenewResponse(
    val success: Boolean = false,
    val enabled: Boolean = false,
)

@Serializable
data class AcmeStatus(
    val running: Boolean = false,
    val purpose: String = "",
    val domain: String = "",
    val stage: String = "",
    val progress: Int = 0,
    val detail: String = "",
    val error: String = "",
    @SerialName("last_detail") val lastDetail: String = "",
    @SerialName("last_run_at") val lastRunAt: String = "",
)
