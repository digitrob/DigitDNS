package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST

/**
 * Server settings.
 *
 * The read is an envelope — `{server, tls}` — while the **save response is the bare
 * server view**, not the envelope. That asymmetry is the server's, so the DTOs mirror
 * it rather than pretending they match.
 *
 * **Save uses POST, not PUT.** The v3 router advertises PUT on this path, but it
 * delegates with the original method to a dashboard handler that only implements
 * GET/POST and answers 405 — which the v3 layer surfaces as a 400 problem. POST is the
 * only verb that actually saves.
 *
 * `api_key` lives in the server view (this is the one place to read it), and the
 * credential fields are deliberately absent from the save body — changing the password
 * is not this route's job.
 */
interface SettingsApi {

    @GET("settings")
    suspend fun get(): SettingsResponse

    @POST("settings")
    suspend fun save(@Body body: SettingsSaveRequest): ServerSettings

    @POST("settings/regenerate-api-key")
    suspend fun regenerateApiKey(@Body body: RegenerateApiKeyRequest): RegenerateApiKeyResponse

    @POST("settings/regenerate-admin-path")
    suspend fun regenerateAdminPath(@Body body: RegenerateAdminPathRequest): RegenerateAdminPathResponse

    @GET("settings/resolution")
    suspend fun getResolution(): ResolutionSettings

    @POST("settings/resolution")
    suspend fun saveResolution(@Body body: ResolutionSaveRequest): ResolutionSettings

    @GET("settings/dnssec")
    suspend fun getDnssec(): DnssecSettings

    @POST("settings/dnssec")
    suspend fun saveDnssec(@Body body: DnssecSettingsSave): DnssecSettings

    @GET("settings/proxy-leak")
    suspend fun getProxyLeak(): ProxyLeakSettings

    @POST("settings/proxy-leak")
    suspend fun saveProxyLeak(@Body body: ProxyLeakSettingsSave): ProxyLeakSettings

    @GET("settings/transports")
    suspend fun getTransports(): TransportSettingsDto

    @POST("settings/transports")
    suspend fun saveTransports(@Body body: TransportSettingsSave): TransportSettingsDto
}

/**
 * The startup-only DNSSEC policy. `default_anchors` is the built-in IANA root KSK
 * list the server would use when `trust_anchors` is empty; `running` is what the
 * daemon installed at boot, which can differ from `enabled` until a restart.
 */
@Serializable
data class DnssecSettings(
    val enabled: Boolean = false,
    @SerialName("trust_anchors") val trustAnchors: List<String> = emptyList(),
    @SerialName("default_anchors") val defaultAnchors: List<String> = emptyList(),
    val running: Boolean = false,
    @SerialName("restart_required") val restartRequired: Boolean = false,
)

/**
 * The save body deliberately has no defaults. kotlinx.serialization omits
 * default-valued properties, and an omitted `enabled` or `trust_anchors` would
 * silently mean "leave the built-in anchors" on the server. Every field is
 * therefore always emitted, including an empty anchor list.
 */
@Serializable
data class DnssecSettingsSave(
    val enabled: Boolean,
    @SerialName("trust_anchors") val trustAnchors: List<String>,
)

/**
 * The live proxy leak guard. `recommended` is always true on current servers:
 * the UI keeps the warning visible even after an operator turns the guard off.
 */
@Serializable
data class ProxyLeakSettings(
    val enabled: Boolean = true,
    val recommended: Boolean = true,
)

@Serializable
data class ProxyLeakSettingsSave(
    val enabled: Boolean,
)

/**
 * The client-facing QUIC transports. [doh3Listening] and [doqListening] are the
 * server's actual bind state, not the saved switches — a port that could not be
 * opened reports false here.
 */
@Serializable
data class TransportSettingsDto(
    @SerialName("doq_enabled") val doqEnabled: Boolean = false,
    @SerialName("doq_port") val doqPort: Int = 853,
    @SerialName("doq_auth") val doqAuth: String = "ip",
    @SerialName("doq_token_domain") val doqTokenDomain: String = "",
    @SerialName("doh3_enabled") val doh3Enabled: Boolean = false,
    @SerialName("doh3_port") val doh3Port: Int = 8443,
    @SerialName("doq_tokens") val doqTokens: List<String> = emptyList(),
    @SerialName("doh3_listening") val doh3Listening: Boolean = false,
    @SerialName("doq_listening") val doqListening: Boolean = false,
    @SerialName("ip_allowlist") val ipAllowlist: TransportIpAllowlistDto = TransportIpAllowlistDto(),
    @SerialName("last_error") val lastError: String = "",
    @SerialName("plain_dns_enabled") val plainDNSEnabled: Boolean = true,
    @SerialName("plain_dns_port") val plainDNSPort: Int = 53,
    @SerialName("plain_dns_listening") val plainDNSListening: Boolean = false,
    @SerialName("server_host") val serverHost: String = "",
    @SerialName("dot_host") val dotHost: String = "",
    @SerialName("doh_enabled") val dohEnabled: Boolean = true,
    @SerialName("doh_port") val dohPort: Int = 8443,
    @SerialName("doh_listening") val dohListening: Boolean = false,
    @SerialName("dot_enabled") val dotEnabled: Boolean = true,
    @SerialName("dot_listening") val dotListening: Boolean = false,
    @SerialName("dot_auth") val dotAuth: String = "ip",
    @SerialName("dot_port") val dotPort: Int = 853,
    @SerialName("dot_token_domain") val dotTokenDomain: String = "",
    @SerialName("dot_tokens") val dotTokens: List<String> = emptyList(),
    @SerialName("dnscrypt_enabled") val dnscryptEnabled: Boolean = false,
    @SerialName("dnscrypt_port") val dnscryptPort: Int = 5443,
    @SerialName("dnscrypt_provider_name") val dnscryptProviderName: String = "",
    @SerialName("dnscrypt_stamp") val dnscryptStamp: String = "",
    @SerialName("dnscrypt_cert_not_after") val dnscryptCertNotAfter: Long = 0,
    @SerialName("dnscrypt_listening") val dnscryptListening: Boolean = false,
    @SerialName("mtls_enabled") val mtlsEnabled: Boolean = false,
    @SerialName("mtls_ca_pem") val mtlsCaPem: String = "",
)

/**
 * Where DoQ's IP-auth mode gets its sources. Both lists are shared with the
 * resolver: per-client bindings and the server-wide resolver allowlist.
 * `allowAll` (Public Mode) means neither list is consulted.
 */
@Serializable
data class TransportIpAllowlistDto(
    @SerialName("allow_all") val allowAll: Boolean = false,
    @SerialName("client_accounts") val clientAccounts: Int = 0,
    @SerialName("client_entries") val clientEntries: Int = 0,
    @SerialName("resolver_entries") val resolverEntries: Int = 0,
)

/**
 * Every transport switch and port must be emitted on every save.
 *
 * kotlinx.serialization omits properties that equal their default unless the
 * Json instance sets encodeDefaults. A defaulted `dnscrypt_enabled = false`
 * therefore disappeared from the body, the server kept its previous `true`, and
 * the Android switch snapped back on. The same bug made DoT impossible to turn
 * back on (`true` was the default and was omitted). These fields deliberately
 * have no defaults, like StatsSettingsSave.
 */
@Serializable
data class TransportSettingsSave(
    @SerialName("doh3_enabled") val doh3Enabled: Boolean,
    @SerialName("doq_enabled") val doqEnabled: Boolean,
    @SerialName("doq_port") val doqPort: Int,
    @SerialName("doq_auth") val doqAuth: String,
    @SerialName("doq_token_domain") val doqTokenDomain: String,
    @SerialName("doq_tokens") val doqTokens: List<String>,
    @SerialName("dnscrypt_enabled") val dnscryptEnabled: Boolean,
    @SerialName("dnscrypt_port") val dnscryptPort: Int,
    @SerialName("dnscrypt_provider_name") val dnscryptProviderName: String,
    @SerialName("plain_dns_enabled") val plainDNSEnabled: Boolean,
    @SerialName("plain_dns_port") val plainDNSPort: Int,
    @SerialName("doh_enabled") val dohEnabled: Boolean,
    @SerialName("doh_port") val dohPort: Int,
    @SerialName("dot_enabled") val dotEnabled: Boolean,
    @SerialName("dot_port") val dotPort: Int,
    @SerialName("dot_auth") val dotAuth: String,
    @SerialName("dot_token_domain") val dotTokenDomain: String,
    @SerialName("dot_tokens") val dotTokens: List<String>,
    @SerialName("mtls_enabled") val mtlsEnabled: Boolean,
    @SerialName("mtls_ca_pem") val mtlsCaPem: String,
)

/**
 * One per-domain resolution override. Absent fields inherit the global default:
 * `transport` is "any", "encrypted" or "regular", and empty means Inherit Global.
 */
@Serializable
data class ResolutionProfileDto(
    val upstream: String = "",
    val strategy: String = "",
    val transport: String = "",
)

/**
 * The global resolution defaults plus the saved overrides. `upstreams` is read-only:
 * the save body must not carry it (the server decodes with DisallowUnknownFields).
 */
@Serializable
data class ResolutionSettings(
    val transport: String = "any",
    /** False when the resolver list holds no DoT/DoH/DoQ upstream: the picker disables it. */
    @SerialName("encrypted_available") val encryptedAvailable: Boolean = false,
    /** The subset carried over DoT/DoH/DoQ/DNSCrypt, for disabling and dimming. */
    @SerialName("encrypted_upstreams") val encryptedUpstreams: List<String> = emptyList(),
    @SerialName("default_upstream") val defaultUpstream: String = "",
    @SerialName("default_strategy") val defaultStrategy: String = "",
    val profiles: Map<String, ResolutionProfileDto> = emptyMap(),
    val upstreams: List<String> = emptyList(),
)

@Serializable
data class ResolutionSaveRequest(
    val transport: String = "any",
    @SerialName("default_upstream") val defaultUpstream: String = "",
    @SerialName("default_strategy") val defaultStrategy: String = "",
    val profiles: Map<String, ResolutionProfileDto> = emptyMap(),
)

@Serializable
data class SettingsResponse(
    val server: ServerSettings? = null,
    val tls: TlsSnapshot? = null,
)

/** The server view: the settings screen's fields plus the readable API key. */
@Serializable
data class ServerSettings(
    @SerialName("public_ip") val publicIp: String = "",
    @SerialName("public_ipv6") val publicIpv6: String = "",
    @SerialName("proxy_ipv6_enabled") val proxyIpv6Enabled: Boolean = false,
    /** ISO 3166-1 alpha-2 country the server's public address resolves to. */
    val country: String = "",
    @SerialName("bind_host") val bindHost: String = "",
    @SerialName("web_port") val webPort: Int = 0,
    @SerialName("admin_username") val adminUsername: String = "",
    @SerialName("admin_password_weak") val adminPasswordWeak: Boolean = false,
    @SerialName("api_key") val apiKey: String = "",
    @SerialName("api_bind") val apiBind: String = "",
    @SerialName("api_bind_list") val apiBindList: List<String> = emptyList(),
    @SerialName("session_idle_minutes") val sessionIdleMinutes: Int = 0,
    @SerialName("admin_path") val adminPath: String = "",
)

/**
 * The save body. Only these four are accepted; credentials are not. `api_bind_list`
 * and `session_idle_minutes` are nullable so "absent" means "leave unchanged" — the
 * server reads a null as "do not touch", and the converter drops nulls.
 */
@Serializable
data class SettingsSaveRequest(
    @SerialName("public_ip") val publicIp: String = "",
    @SerialName("api_bind") val apiBind: String = "",
    @SerialName("api_bind_list") val apiBindList: List<String>? = null,
    @SerialName("session_idle_minutes") val sessionIdleMinutes: Int? = null,
    // Null means "leave unchanged". An empty string is the explicit clear.
    @SerialName("public_ipv6") val publicIpv6: String? = null,
    @SerialName("proxy_ipv6_enabled") val proxyIpv6Enabled: Boolean? = null,
)

@Serializable
data class TlsSnapshot(
    val domain: String = "",
    val email: String = "",
    @SerialName("cert_path") val certPath: String = "",
    @SerialName("key_path") val keyPath: String = "",
    @SerialName("auto_renew_acme") val autoRenewAcme: Boolean = false,
    @SerialName("dot_domain") val dotDomain: String = "",
    @SerialName("panel_https") val panelHttps: Boolean = false,
    @SerialName("redirect_port") val redirectPort: Int = 0,
    @SerialName("dot_auto_renew_acme") val dotAutoRenewAcme: Boolean = false,
)

@Serializable
data class RegenerateApiKeyRequest(
    @SerialName("current_password") val currentPassword: String,
    val code: String? = null,
)

@Serializable
data class RegenerateApiKeyResponse(
    val success: Boolean = false,
    @SerialName("api_key") val apiKey: String = "",
)

@Serializable
data class RegenerateAdminPathRequest(
    val confirm: Boolean,
)

@Serializable
data class RegenerateAdminPathResponse(
    val success: Boolean = false,
    @SerialName("admin_path") val adminPath: String = "",
)
