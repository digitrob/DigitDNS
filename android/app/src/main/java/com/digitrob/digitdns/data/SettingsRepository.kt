package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.SettingsApi
import com.digitrob.digitdns.core.network.SettingsSaveRequest
import com.digitrob.digitdns.core.network.RegenerateApiKeyRequest
import com.digitrob.digitdns.core.network.RegenerateAdminPathRequest
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
    ): ServerConfig = ServerConfig.from(
        api.save(
            SettingsSaveRequest(
                publicIp = publicIp,
                apiBind = apiBind,
                apiBindList = apiBindList,
                sessionIdleMinutes = sessionIdleMinutes,
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
}
