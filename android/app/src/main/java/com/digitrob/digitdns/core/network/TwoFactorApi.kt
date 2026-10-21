package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST

/**
 * TOTP two-factor enrolment.
 *
 * The secret and the `otpauth://` URI are returned **only** by [setup] — the status
 * route never carries them (it reports the literal `"present"` while an enrolment is
 * open, never the URI). So the QR is drawn from the setup response, and a screen that
 * only polls status cannot rebuild it; setup must be re-run to show the code again.
 *
 * Every write carries `username` and `current_password`, and [enable]/[disable] also
 * need `code`. Enabling invalidates all sessions, so the app must be ready to
 * re-authenticate afterwards.
 */
interface TwoFactorApi {

    @GET("auth/2fa/status")
    suspend fun status(): TwoFactorStatus

    @POST("auth/2fa/setup")
    suspend fun setup(@Body body: TwoFactorCredentials): TwoFactorSetupResponse

    @POST("auth/2fa/enable")
    suspend fun enable(@Body body: TwoFactorCodeRequest): SimpleSuccess

    @POST("auth/2fa/disable")
    suspend fun disable(@Body body: TwoFactorCodeRequest): SimpleSuccess
}

/** The setup/enable/disable body: the identify-and-prove fields every 2FA write needs. */
@Serializable
data class TwoFactorCredentials(
    val username: String,
    @SerialName("current_password") val currentPassword: String,
)

@Serializable
data class TwoFactorCodeRequest(
    val username: String,
    @SerialName("current_password") val currentPassword: String,
    val code: String,
)

@Serializable
data class TwoFactorSetupResponse(
    val success: Boolean = false,
    /** The base32 TOTP secret, shown once for manual entry. */
    val secret: String = "",
    /** The `otpauth://` URI the QR is rendered from. */
    @SerialName("otpauth_uri") val otpauthUri: String = "",
)
