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
}

@Serializable
data class SettingsResponse(
    val server: ServerSettings? = null,
    val tls: TlsSnapshot? = null,
)

/** The server view: the settings screen's fields plus the readable API key. */
@Serializable
data class ServerSettings(
    @SerialName("public_ip") val publicIp: String = "",
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
