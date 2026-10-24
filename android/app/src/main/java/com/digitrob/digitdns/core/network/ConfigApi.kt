package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.http.GET

/**
 * The slice of `GET /api/v3/config` the Connect tab needs.
 *
 * The full config response is large (settings, TLS, access, rules, certificates) and
 * nearly all of it is Phase 4's problem. The two fields used here are the ones the
 * panel's own Connect Guide reads to build subscriber links and the DoH endpoint:
 * `subscription_origin` (scheme/host/port the daemon will actually serve, which is
 * not always the panel's own) and `doh_url`. Unknown members are ignored, so the
 * rest of the blob costs this client nothing to skip.
 */
interface ConfigApi {

    @GET("config")
    suspend fun config(): ConfigResponse
}

@Serializable
data class ConfigResponse(
    @SerialName("subscription_origin") val subscriptionOrigin: String = "",
    @SerialName("doh_url") val dohUrl: String = "",
    val server: ServerView? = null,
    val tls: TlsView? = null,
    val subscription: SubscriptionConfigView? = null,
    @SerialName("dot_cert") val dotCert: CertView? = null,
    @SerialName("panel_cert") val panelCert: CertView? = null,
    @SerialName("subscription_cert") val subscriptionCert: CertView? = null,
)

/**
 * The subscription portal settings as `/config` reports them. The SSL/TLS screen needs
 * the whole snapshot, not just the certificate: its "Certificate Auto-renew" switch
 * writes that flag through `POST settings/subscription`, and the save replaces every
 * field, so it has to send back the values it loaded.
 *
 * [autoRenewAcme] is nullable on purpose. The daemon stores the negated form and older
 * builds omit the key entirely; absent means "renewal keeps running", not "off", so the
 * screen must be able to tell those apart.
 */
@Serializable
data class SubscriptionConfigView(
    val enabled: Boolean = false,
    val domain: String = "",
    val port: Int = 0,
    @SerialName("uri_path") val uriPath: String = "",
    val title: String = "",
    @SerialName("theme_css") val themeCss: String = "",
    @SerialName("theme_css_source") val themeCssSource: String = "",
    @SerialName("theme_css_path") val themeCssPath: String = "",
    @SerialName("theme_css_url") val themeCssUrl: String = "",
    @SerialName("auto_renew_acme") val autoRenewAcme: Boolean? = null,
)

/**
 * A certificate's details, as `GET /api/v3/config` reports them under `dot_cert`,
 * `panel_cert` and `subscription_cert`. The same shape the panel's SSL/TLS card reads
 * to show a validity window and how many days are left — so the phone can show the
 * same thing rather than just "issued".
 *
 * [notBefore]/[notAfter] are RFC 3339 timestamps; [expired] is the server's own verdict
 * and is trusted over recomputing it from the clock.
 */
@Serializable
data class CertView(
    val domain: String = "",
    @SerialName("not_before") val notBefore: String = "",
    @SerialName("not_after") val notAfter: String = "",
    val expired: Boolean = false,
    val subject: String = "",
)

/**
 * The `tls` slice the Connect Guide reads: the DoT hostname prefers the dedicated
 * DoH/DoT domain when one is set, because that is the only name the port-853
 * certificate is guaranteed to cover; otherwise it falls back to the panel domain.
 *
 * [dotAutoRenew] is the DoH/DoT renewal flag (`no_dot_auto_renew_acme` on the wire,
 * reported in its positive form). Nullable for the same reason as
 * [SubscriptionConfigView.autoRenewAcme]: a missing key means renewal is on.
 */
@Serializable
data class TlsView(
    @SerialName("dot_domain") val dotDomain: String = "",
    @SerialName("domain") val domain: String = "",
    @SerialName("dot_auto_renew_acme") val dotAutoRenew: Boolean? = null,
) {
    /** The hostname to put in Android's Private DNS field. */
    val dotHostname: String get() = dotDomain.ifBlank { domain }
}

/**
 * The `server` settings view, of which the DNS listener address is the only thing the
 * Connect tab shows as "point your device here". The remaining fields belong to the
 * Settings screen in Phase 4.
 */
@Serializable
data class ServerView(
    @SerialName("public_ip") val publicIp: String = "",
    @SerialName("dns_port") val dnsPort: Int = 0,
    @SerialName("domain") val domain: String = "",
)
