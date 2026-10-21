package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.AccessApi
import com.digitrob.digitdns.core.network.AccessModeRequest
import com.digitrob.digitdns.core.network.AccessResponse
import com.digitrob.digitdns.core.network.AccessUpdateRequest
import javax.inject.Inject
import javax.inject.Singleton

/** The access gate state: the mode, the allowlist, and the DoH tokens. */
data class AccessSettings(
    val allowAll: Boolean,
    val allowedIps: List<String>,
    val dohTokens: List<String>,
)

/**
 * The resolver access gate.
 *
 * Two writes, deliberately separate — the server ignores `allow_all` on the token/IP
 * save so that toggling the mode is an explicit act. [setMode] is that act; [save]
 * only ever changes the allowlist and tokens.
 */
@Singleton
class AccessRepository @Inject constructor(
    private val api: AccessApi,
) {
    suspend fun load(): AccessSettings = api.get().toDomain()

    /** Save the allowlist and DoH tokens. Does not change the mode. */
    suspend fun save(allowedIps: List<String>, dohTokens: List<String>): Boolean =
        api.update(
            AccessUpdateRequest(
                allowAll = false,
                dohTokens = dohTokens,
                allowedIps = allowedIps,
            ),
        ).success

    /** Flip between open (allow_all) and allowlist mode. */
    suspend fun setMode(allowAll: Boolean): Boolean =
        api.setMode(AccessModeRequest(allowAll)).allowAll == allowAll

    private fun AccessResponse.toDomain() = AccessSettings(
        allowAll = allowAll,
        allowedIps = allowedIps,
        dohTokens = dohTokens,
    )
}
