package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.DnssecSettings
import com.digitrob.digitdns.core.network.DnssecSettingsSave
import com.digitrob.digitdns.core.network.ProxyLeakSettings
import com.digitrob.digitdns.core.network.ProxyLeakSettingsSave
import com.digitrob.digitdns.core.network.Doh3SettingsSave
import com.digitrob.digitdns.core.network.DoqSettingsSave
import com.digitrob.digitdns.core.network.TransportSettingsDto
import com.digitrob.digitdns.core.network.SettingsApi
import com.digitrob.digitdns.core.network.SettingsSaveRequest
import com.digitrob.digitdns.core.network.RegenerateApiKeyRequest
import com.digitrob.digitdns.core.network.RegenerateAdminPathRequest
import com.digitrob.digitdns.core.network.ResolutionSaveRequest
import com.digitrob.digitdns.core.network.ResolutionSettings
import com.digitrob.digitdns.domain.model.ServerConfig
import com.digitrob.digitdns.domain.model.TlsSettings
import javax.inject.Inject
import javax.inject.Singleton

/** The settings read: the server view and the TLS snapshot together. */
data class SettingsBundle(
    val server: ServerConfig?,
    val tls: TlsSettings?,
)

/**
 * Server settings. The save uses POST (see [SettingsApi]); the two regenerate calls
 * rotate credentials and are the reason this repository hands back the new values
 * rather than swallowing them — the caller must persist the new key/path.
 */
@Singleton
class SettingsRepository @Inject constructor(
    private val api: SettingsApi,
) {
    suspend fun load(): SettingsBundle {
        val response = api.get()
        return SettingsBundle(
            server = response.server?.let(ServerConfig::from),
            tls = response.tls?.let(TlsSettings::from),
        )
    }

    /**
     * Save the operator-editable server fields. [apiBindList] and [sessionIdleMinutes]
     * are nullable: null leaves the stored value untouched (the server reads a null as
     * "do not change"), which is how the screen saves a single field without clearing
     * the others.
     */
    suspend fun save(
        publicIp: String,
        apiBind: String,
        apiBindList: List<String>?,
        sessionIdleMinutes: Int?,
        publicIpv6: String? = null,
        proxyIpv6Enabled: Boolean? = null,
    ): ServerConfig = ServerConfig.from(
        api.save(
            SettingsSaveRequest(
                publicIp = publicIp,
                apiBind = apiBind,
                apiBindList = apiBindList,
                sessionIdleMinutes = sessionIdleMinutes,
                publicIpv6 = publicIpv6,
                proxyIpv6Enabled = proxyIpv6Enabled,
            ),
        ),
    )

    /** Save only the live proxy-IPv6 policy; omitted fields stay untouched. */
    suspend fun saveIpv6(enabled: Boolean, address: String): ServerConfig = ServerConfig.from(
        api.save(
            SettingsSaveRequest(
                publicIpv6 = address,
                proxyIpv6Enabled = enabled,
            ),
        ),
    )

    /** Rotate the API key. [code] is required when 2FA is on. Returns the new key. */
    suspend fun regenerateApiKey(currentPassword: String, code: String?): String =
        api.regenerateApiKey(RegenerateApiKeyRequest(currentPassword, code)).apiKey

    /**
     * Rotate the admin path. Destructive: every session and bookmark under the old
     * path stops working, so the caller persists the new path or the app locks itself
     * out. Returns the new path.
     */
    suspend fun regenerateAdminPath(confirm: Boolean): String =
        api.regenerateAdminPath(RegenerateAdminPathRequest(confirm)).adminPath

    /** The global resolution defaults and the per-domain overrides. */
    suspend fun loadResolution(): ResolutionSettings = api.getResolution()

    /** Save the resolution settings and hand back the server's canonical answer. */
    suspend fun saveResolution(request: ResolutionSaveRequest): ResolutionSettings =
        api.saveResolution(request)

    /** The saved DNSSEC policy, the built-in anchors and the running state. */
    suspend fun loadDnssec(): DnssecSettings = api.getDnssec()

    /** Save the startup-only DNSSEC policy and hand back the server's answer. */
    suspend fun saveDnssec(enabled: Boolean, trustAnchors: List<String>): DnssecSettings =
        api.saveDnssec(DnssecSettingsSave(enabled = enabled, trustAnchors = trustAnchors))

    /** The live proxy leak guard, which applies to the running resolver immediately. */
    suspend fun loadProxyLeak(): ProxyLeakSettings = api.getProxyLeak()

    /** Save the live proxy leak guard and hand back the server's canonical answer. */
    suspend fun saveProxyLeak(enabled: Boolean): ProxyLeakSettings =
        api.saveProxyLeak(ProxyLeakSettingsSave(enabled = enabled))

    /** The client-facing DoH3/DoQ policy and its actual listener state. */
    suspend fun loadTransports(): TransportSettingsDto = api.getTransports()

    /** Apply only the DoH3 switch; DoQ settings are left untouched. */
    suspend fun saveDoh3(enabled: Boolean): TransportSettingsDto =
        api.saveDoh3(Doh3SettingsSave(doh3Enabled = enabled))

    /** Apply the staged DoQ configuration. */
    suspend fun saveDoq(request: DoqSettingsSave): TransportSettingsDto =
        api.saveDoq(request)
}
