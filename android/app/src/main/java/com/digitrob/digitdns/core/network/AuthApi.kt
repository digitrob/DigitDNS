package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST

/**
 * The v3 auth surface, from `internal/api/openapi/v3.json`.
 *
 * Paths are relative to the API root (`/api/v3/`); [AuthInterceptor] prepends the
 * install's hidden admin path and rewrites the origin.
 */
interface AuthApi {

    /** `POST /auth/login` — keyless; mints a `digitdns_session_` token. */
    @POST("auth/login")
    suspend fun login(@Body request: LoginRequest): LoginResponse

    /** `POST /auth/logout` — revokes the presented session. */
    @POST("auth/logout")
    suspend fun logout(): LogoutResponse

    /** `GET /auth/me` — current identity and 2FA state. */
    @GET("auth/me")
    suspend fun me(): MeResponse

    /** `GET /auth/2fa/status` — whether TOTP is enabled/configured/enrolling. */
    @GET("auth/2fa/status")
    suspend fun twoFactorStatus(): TwoFactorStatus

    /** `POST /auth/sse-ticket` — a one-time ticket for the SSE stream. */
    @POST("auth/sse-ticket")
    suspend fun sseTicket(): SseTicketResponse
}

@Serializable
data class LoginRequest(
    val username: String,
    val password: String,
    /** TOTP code; only sent when the operator has supplied one. */
    val code: String? = null,
)

@Serializable
data class LoginResponse(
    val token: String = "",
    val username: String = "",
    @SerialName("password_weak") val passwordWeak: Boolean = false,
    @SerialName("is_default_password") val isDefaultPassword: Boolean = false,
    /** Idle window in seconds; the session lapses if unused for this long. */
    @SerialName("expires_in") val expiresIn: Int = 0,
)

@Serializable
data class LogoutResponse(val ok: Boolean = false)

@Serializable
data class MeResponse(
    val username: String = "",
    @SerialName("password_weak") val passwordWeak: Boolean = false,
    @SerialName("is_default_password") val isDefaultPassword: Boolean = false,
    val auth: AuthSnapshot? = null,
)

@Serializable
data class AuthSnapshot(
    @SerialName("totp_enabled") val totpEnabled: Boolean = false,
    @SerialName("totp_configured") val totpConfigured: Boolean = false,
    @SerialName("totp_enrolling") val totpEnrolling: Boolean = false,
    @SerialName("otpauth_uri") val otpAuthUri: String = "",
)

@Serializable
data class TwoFactorStatus(
    @SerialName("totp_enabled") val totpEnabled: Boolean = false,
    @SerialName("totp_configured") val totpConfigured: Boolean = false,
    @SerialName("totp_enrolling") val totpEnrolling: Boolean = false,
)

@Serializable
data class SseTicketResponse(
    val ticket: String = "",
    /** Seconds the ticket stays valid; 60 on the current server. */
    @SerialName("expires_in") val expiresIn: Int = 0,
)
