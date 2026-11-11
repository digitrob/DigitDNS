package com.digitrob.digitdns.core.security

import android.content.Context
import android.os.Build
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricPrompt
import androidx.core.content.ContextCompat
import androidx.fragment.app.FragmentActivity
import kotlinx.coroutines.suspendCancellableCoroutine
import javax.inject.Inject
import javax.inject.Singleton
import kotlin.coroutines.resume

/** What the device can offer for the app lock. */
enum class BiometricAvailability {
    AVAILABLE,

    /** No fingerprint/face sensor and no usable class-3 biometric. */
    NO_HARDWARE,

    /** Hardware exists, but the user has not enrolled a fingerprint or face. */
    NOT_ENROLLED,

    /** The device has no secure lock screen (PIN/pattern/password). */
    NO_DEVICE_CREDENTIAL,

    /** The vendor security patch is too old for the biometric stack. */
    SECURITY_UPDATE_REQUIRED,

    /** The platform/library combination cannot offer the requested authenticators. */
    UNSUPPORTED,
}

/**
 * One place for the app-lock prompt.
 *
 * Biometrics are a local convenience, not a replacement for the server password:
 * the prompt unlocks the credential the app already holds. Device credential is
 * allowed as a fallback, so a forgotten fingerprint is not a lockout. On API 29
 * the combined authenticator constant is unavailable, so the builder uses the
 * older `setDeviceCredentialAllowed` form.
 */
@Singleton
class BiometricAuthenticator @Inject constructor(
    private val context: Context,
) {
    fun availability(): BiometricAvailability {
        val manager = BiometricManager.from(context)
        val code = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            manager.canAuthenticate(BIOMETRIC_STRONG or DEVICE_CREDENTIAL)
        } else {
            manager.canAuthenticate(BIOMETRIC_STRONG)
        }
        return when (code) {
            BiometricManager.BIOMETRIC_SUCCESS -> BiometricAvailability.AVAILABLE
            BiometricManager.BIOMETRIC_ERROR_NO_HARDWARE -> BiometricAvailability.NO_HARDWARE
            BiometricManager.BIOMETRIC_ERROR_NONE_ENROLLED -> BiometricAvailability.NOT_ENROLLED
            BiometricManager.BIOMETRIC_ERROR_SECURITY_UPDATE_REQUIRED -> BiometricAvailability.SECURITY_UPDATE_REQUIRED
            BiometricManager.BIOMETRIC_ERROR_UNSUPPORTED -> BiometricAvailability.UNSUPPORTED
            else -> BiometricAvailability.NO_DEVICE_CREDENTIAL
        }
    }

    /**
     * Show the prompt and resume with true only on a successful authentication.
     * Cancel, lockout and hardware errors all resume false; the caller keeps the
     * app locked and can offer a retry or a sign-out.
     */
    suspend fun authenticate(
        activity: FragmentActivity,
        title: String,
        subtitle: String,
    ): Boolean = suspendCancellableCoroutine { continuation ->
        val prompt = BiometricPrompt(
            activity,
            ContextCompat.getMainExecutor(activity),
            object : BiometricPrompt.AuthenticationCallback() {
                override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) {
                    if (continuation.isActive) continuation.resume(true)
                }

                override fun onAuthenticationError(errorCode: Int, errString: CharSequence) {
                    if (continuation.isActive) continuation.resume(false)
                }
            },
        )
        val info = BiometricPrompt.PromptInfo.Builder()
            .setTitle(title)
            .setSubtitle(subtitle)
            .apply {
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                    setAllowedAuthenticators(BIOMETRIC_STRONG or DEVICE_CREDENTIAL)
                } else {
                    @Suppress("DEPRECATION")
                    setDeviceCredentialAllowed(true)
                }
            }
            .build()
        prompt.authenticate(info)
        continuation.invokeOnCancellation { prompt.cancelAuthentication() }
    }

    private companion object {
        const val BIOMETRIC_STRONG = BiometricManager.Authenticators.BIOMETRIC_STRONG
        const val DEVICE_CREDENTIAL = BiometricManager.Authenticators.DEVICE_CREDENTIAL
    }
}
