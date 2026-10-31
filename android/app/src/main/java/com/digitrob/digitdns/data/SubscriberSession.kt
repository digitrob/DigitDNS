package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.security.SecretStore
import com.digitrob.digitdns.domain.model.SubscriberLink
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The subscriber session: the portal link and, optionally, the out-of-band
 * registration secret.
 *
 * Both are credentials — the token in the link reads the account, the secret moves its
 * bound address — so "save" means encrypted storage and "don't save" means memory for
 * this process only. A saved link is adopted at construction so a relaunch can open
 * the subscriber surface without re-typing it.
 */
@Singleton
class SubscriberSession @Inject constructor(
    private val secretStore: SecretStore,
) {
    private val _link = MutableStateFlow(SubscriberLink.parse(secretStore.subscriberLink().orEmpty()))
    val link: StateFlow<SubscriberLink?> = _link.asStateFlow()

    // The account name the portal reports. It is display-only — the token is the
    // credential — but it is what the Servers list shows, so it is kept beside the
    // link and forgotten with it.
    private val _label = MutableStateFlow(secretStore.subscriberLabel().orEmpty())
    val label: StateFlow<String> = _label.asStateFlow()

    private var registrationSecret: String = secretStore.subscriberSecret().orEmpty()

    /** Whether [adopt] wrote this link to disk; sign-out keeps a saved link for prefill. */
    private var savedOnDevice: Boolean = !secretStore.subscriberLink().isNullOrBlank()

    fun currentLink(): SubscriberLink? = _link.value

    fun currentLabel(): String = _label.value

    fun secret(): String = registrationSecret

    fun hasSavedLink(): Boolean = !secretStore.subscriberLink().isNullOrBlank()

    /** True when a secret is available for registration, saved or from this session. */
    fun hasSecret(): Boolean =
        registrationSecret.isNotBlank() || !secretStore.subscriberSecret().isNullOrBlank()

    /** True only when the secret is on disk, for the "saved" badge. */
    fun hasSavedSecret(): Boolean = !secretStore.subscriberSecret().isNullOrBlank()

    /**
     * Adopt a verified link. [save] persists it (and the secret) encrypted; without it
     * the session lives in memory and a relaunch asks for the link again.
     */
    fun adopt(link: SubscriberLink, secret: String, save: Boolean) {
        // A different portal is a different account: the previous name must not
        // label it, and the next status fetch will supply the real one.
        if (_link.value?.portalUrl != link.portalUrl) _label.value = ""
        _link.value = link
        registrationSecret = secret.trim()
        savedOnDevice = save
        if (save) {
            secretStore.setSubscriberLink(link.portalUrl)
            secretStore.setSubscriberSecret(registrationSecret.ifBlank { null })
            secretStore.setSubscriberLabel(_label.value.ifBlank { null })
        } else {
            secretStore.setSubscriberLink(null)
            secretStore.setSubscriberSecret(null)
            secretStore.setSubscriberLabel(null)
        }
    }

    /**
     * Record the account name the portal returned. It reaches the app after the link
     * has already been adopted, so this is a second write; a session-only link stays
     * in memory, exactly like the link itself.
     */
    fun updateLabel(value: String) {
        val trimmed = value.trim()
        if (trimmed == _label.value) return
        _label.value = trimmed
        if (savedOnDevice) secretStore.setSubscriberLabel(trimmed.ifBlank { null })
    }

    /** Keep a corrected registration secret, honouring the same save choice. */
    fun updateSecret(secret: String, save: Boolean) {
        registrationSecret = secret.trim()
        if (save) secretStore.setSubscriberSecret(registrationSecret.ifBlank { null })
    }

    /**
     * Sign out of the subscriber surface. A link saved on the device stays in memory
     * so the login form can prefill it; a session-only link is discarded here.
     */
    fun endSession() {
        if (savedOnDevice) return
        _link.value = null
        registrationSecret = ""
    }

    /** Forget the link, the secret and the host's certificate pin. */
    fun forget() {
        val host = _link.value?.host
        _link.value = null
        _label.value = ""
        registrationSecret = ""
        secretStore.clearSubscriber(host)
    }
}
