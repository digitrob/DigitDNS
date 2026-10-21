package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.TwoFactorApi
import com.digitrob.digitdns.core.network.TwoFactorCodeRequest
import com.digitrob.digitdns.core.network.TwoFactorCredentials
import javax.inject.Inject
import javax.inject.Singleton

/** The one-time enrolment material returned by setup. */
data class TwoFactorEnrolment(
    val secret: String,
    val otpauthUri: String,
)

/**
 * TOTP two-factor. The secret and URI live only in [startEnrolment]; [status] never
 * carries them, so the enrolment QR cannot be rebuilt from a status poll.
 */
@Singleton
class TwoFactorRepository @Inject constructor(
    private val api: TwoFactorApi,
) {
    suspend fun status() = api.status()

    suspend fun startEnrolment(username: String, currentPassword: String): TwoFactorEnrolment {
        val response = api.setup(TwoFactorCredentials(username, currentPassword))
        return TwoFactorEnrolment(secret = response.secret, otpauthUri = response.otpauthUri)
    }

    suspend fun enable(username: String, currentPassword: String, code: String): Boolean =
        api.enable(TwoFactorCodeRequest(username, currentPassword, code)).success

    suspend fun disable(username: String, currentPassword: String, code: String): Boolean =
        api.disable(TwoFactorCodeRequest(username, currentPassword, code)).success
}
