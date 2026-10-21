package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST

/**
 * The subscription portal — the small web app that hands clients their config.
 *
 * As with settings, **save is POST**: PUT is advertised but fails through to a 405.
 * The save response carries the refreshed snapshot plus the computed
 * `subscription_origin` (the scheme/host/port the portal will actually serve, which
 * the Connect tab already reads from `/config`).
 *
 * Several fields are server-derived on save (`cert_path`, `key_path`,
 * `use_panel_certificate`, `listen_ip`) — sending them is pointless because the
 * sanitizer overrides them, so the request DTO omits them.
 */
interface SubscriptionApi {

    @GET("settings/subscription")
    suspend fun get(): SubscriptionSnapshot

    @POST("settings/subscription")
    suspend fun save(@Body body: SubscriptionSaveRequest): SubscriptionSaveResponse
}

@Serializable
data class SubscriptionSnapshot(
    val enabled: Boolean = false,
    @SerialName("listen_ip") val listenIp: String = "",
    val domain: String = "",
    val port: Int = 0,
    @SerialName("uri_path") val uriPath: String = "",
    val title: String = "",
    @SerialName("theme_css") val themeCss: String = "",
    @SerialName("theme_css_source") val themeCssSource: String = "",
    @SerialName("theme_css_path") val themeCssPath: String = "",
    @SerialName("theme_css_url") val themeCssUrl: String = "",
    @SerialName("cert_path") val certPath: String = "",
    @SerialName("key_path") val keyPath: String = "",
    @SerialName("use_panel_certificate") val usePanelCertificate: Boolean = false,
    @SerialName("port_explicit") val portExplicit: Boolean = false,
    @SerialName("auto_renew_acme") val autoRenewAcme: Boolean = false,
)

/** The save body: the operator-editable fields only. */
@Serializable
data class SubscriptionSaveRequest(
    val enabled: Boolean,
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

@Serializable
data class SubscriptionSaveResponse(
    val success: Boolean = false,
    @SerialName("restart_required") val restartRequired: Boolean = false,
    val subscription: SubscriptionSnapshot? = null,
    @SerialName("subscription_origin") val subscriptionOrigin: String = "",
    @SerialName("listener_error") val listenerError: String = "",
)
