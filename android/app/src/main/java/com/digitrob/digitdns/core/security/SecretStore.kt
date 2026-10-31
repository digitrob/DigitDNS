package com.digitrob.digitdns.core.security

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import javax.inject.Inject
import javax.inject.Singleton
import timber.log.Timber

/**
 * Credentials and pinned certificates, encrypted at rest.
 *
 * `EncryptedSharedPreferences` seals both keys and values under a Keystore master
 * key, which is the platform's own answer to "I need a small secret on disk". The
 * session token is the prize here — it is a live dashboard session, so it is a far
 * more valuable theft than the host and port — and the certificate pins are secrets
 * of a sort too: writing them to a plain file would let anyone who can edit that
 * file silently MITM the app by substituting their own pin.
 *
 * A failure to open the encrypted store (a corrupted Keystore, a device that lost
 * its keys) is not fatal: the app treats it as "no saved secrets" and the operator
 * logs in again. Losing a session is recoverable; crashing on launch is not.
 */
@Singleton
@Suppress("TooManyFunctions")
class SecretStore @Inject constructor(
    private val context: Context,
) {
    private object Keys {
        const val PIN_PREFIX = "tls_pin_"

        // Credentials are keyed per saved install so two profiles do not share a
        // session. The suffix is the Connection id; the prefix keeps the keys
        // readable in a `run-as` dump during development.
        const val SESSION_TOKEN_PREFIX = "session_token_"
        const val API_KEY_PREFIX = "api_key_"
        const val SAVED_USERNAME_PREFIX = "saved_username_"
        const val SAVED_PASSWORD_PREFIX = "saved_password_"

        // The subscriber portal link is a read credential too (its path carries the
        // token), so it lives here beside the operator's session rather than in a
        // plain preference file. The registration secret is the write credential.
        const val SUBSCRIBER_LINK = "subscriber_link"
        const val SUBSCRIBER_SECRET = "subscriber_secret"
        // The account name the portal reports, kept beside the link so the saved
        // subscription reads as a name rather than a hostname in the server list.
        const val SUBSCRIBER_LABEL = "subscriber_label"

        // Legacy single-connection keys, read once so an existing install keeps its
        // session across the upgrade to multi-server, then removed.
        const val LEGACY_SESSION_TOKEN = "session_token"
        const val LEGACY_API_KEY = "api_key"
        const val LEGACY_SAVED_USERNAME = "saved_username"
        const val LEGACY_SAVED_PASSWORD = "saved_password"
    }

    private val prefs: SharedPreferences? by lazy {
        runCatching {
            val masterKey = MasterKey.Builder(context)
                .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
                .build()
            EncryptedSharedPreferences.create(
                context,
                "digitdns_secrets",
                masterKey,
                EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
                EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
            )
        }.onFailure { Timber.w(it, "encrypted storage unavailable; secrets will not persist") }
            .getOrNull()
    }

    fun sessionToken(id: String): String? = prefs?.getString(Keys.SESSION_TOKEN_PREFIX + id, null)

    fun setSessionToken(id: String, token: String?) = put(Keys.SESSION_TOKEN_PREFIX + id, token)

    fun apiKey(id: String): String? = prefs?.getString(Keys.API_KEY_PREFIX + id, null)

    fun setApiKey(id: String, key: String?) = put(Keys.API_KEY_PREFIX + id, key)

    /**
     * Login credentials kept only when the operator ticks "save credentials".
     *
     * The session token alone is enough for a device that stays online, but the v3
     * session has an idle window, so a phone left alone for a day comes back to a
     * lapsed token. Replaying the password is the only way to silently re-mint it, and
     * replaying a password means *storing* one — hence the explicit opt-in. Both fields
     * are encrypted by [EncryptedSharedPreferences] like every other secret here, and
     * [clearCredentials] drops them on logout.
     */
    fun savedUsername(id: String): String? = prefs?.getString(Keys.SAVED_USERNAME_PREFIX + id, null)

    fun setSavedUsername(id: String, username: String?) = put(Keys.SAVED_USERNAME_PREFIX + id, username)

    fun savedPassword(id: String): String? = prefs?.getString(Keys.SAVED_PASSWORD_PREFIX + id, null)

    fun setSavedPassword(id: String, password: String?) = put(Keys.SAVED_PASSWORD_PREFIX + id, password)

    /** True when [id] holds a complete saved login pair, for per-install status display. */
    fun hasSavedCredentials(id: String): Boolean =
        !savedUsername(id).isNullOrBlank() && savedPassword(id) != null

    private fun put(key: String, value: String?) {
        prefs?.edit()?.apply {
            if (value == null) remove(key) else putString(key, value)
        }?.apply()
    }

    fun subscriberLink(): String? = prefs?.getString(Keys.SUBSCRIBER_LINK, null)

    fun setSubscriberLink(link: String?) = put(Keys.SUBSCRIBER_LINK, link)

    fun subscriberSecret(): String? = prefs?.getString(Keys.SUBSCRIBER_SECRET, null)

    fun setSubscriberSecret(secret: String?) = put(Keys.SUBSCRIBER_SECRET, secret)

    fun subscriberLabel(): String? = prefs?.getString(Keys.SUBSCRIBER_LABEL, null)

    fun setSubscriberLabel(label: String?) = put(Keys.SUBSCRIBER_LABEL, label)

    /** Forget the saved subscription link, its secret, and this host's certificate pin. */
    fun clearSubscriber(host: String? = null) {
        prefs?.edit()?.apply {
            remove(Keys.SUBSCRIBER_LINK)
            remove(Keys.SUBSCRIBER_SECRET)
            remove(Keys.SUBSCRIBER_LABEL)
            if (!host.isNullOrBlank()) remove(Keys.PIN_PREFIX + host)
        }?.apply()
    }

    fun pinnedFingerprint(host: String): String? =
        prefs?.getString(Keys.PIN_PREFIX + host, null)

    fun pinFingerprint(host: String, fingerprint: String) {
        prefs?.edit()?.putString(Keys.PIN_PREFIX + host, fingerprint)?.apply()
    }

    fun forgetFingerprint(host: String) {
        prefs?.edit()?.remove(Keys.PIN_PREFIX + host)?.apply()
    }

    /**
     * Drop every stored credential for one install — used on logout and on "forget
     * this install". The saved username/password go too: a logout that left them
     * behind would silently log the next person in, which is the opposite of what
     * logout means. The certificate pin is kept; it belongs to the host, not the
     * credential, and forgetting it would force a fresh TOFU prompt.
     */
    fun clearCredentials(id: String) {
        prefs?.edit()?.apply {
            remove(Keys.SESSION_TOKEN_PREFIX + id)
            remove(Keys.API_KEY_PREFIX + id)
            remove(Keys.SAVED_USERNAME_PREFIX + id)
            remove(Keys.SAVED_PASSWORD_PREFIX + id)
        }?.apply()
    }

    /**
     * Adopt any legacy single-connection credential under [id], then delete the legacy
     * keys. Called once when the connection store migrates a pre-multi-server install,
     * so the upgrade does not silently log the operator out.
     */
    fun adoptLegacyCredential(id: String) {
        val prefsValue = prefs ?: return
        prefsValue.edit().apply {
            prefsValue.getString(Keys.LEGACY_SESSION_TOKEN, null)?.let { putString(Keys.SESSION_TOKEN_PREFIX + id, it) }
            prefsValue.getString(Keys.LEGACY_API_KEY, null)?.let { putString(Keys.API_KEY_PREFIX + id, it) }
            prefsValue.getString(Keys.LEGACY_SAVED_USERNAME, null)?.let { putString(Keys.SAVED_USERNAME_PREFIX + id, it) }
            prefsValue.getString(Keys.LEGACY_SAVED_PASSWORD, null)?.let { putString(Keys.SAVED_PASSWORD_PREFIX + id, it) }
            remove(Keys.LEGACY_SESSION_TOKEN)
            remove(Keys.LEGACY_API_KEY)
            remove(Keys.LEGACY_SAVED_USERNAME)
            remove(Keys.LEGACY_SAVED_PASSWORD)
        }.apply()
    }
}
