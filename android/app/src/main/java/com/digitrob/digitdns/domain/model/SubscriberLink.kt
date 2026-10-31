package com.digitrob.digitdns.domain.model

import java.net.URI

/**
 * The subscriber portal link a customer pastes into the app.
 *
 * Unlike an admin [Connection], this link is the credential: the token in its path is
 * what the server treats as the read credential, and it carries no admin path or
 * session. The app keeps it verbatim (encrypted when saved) and builds every call
 * from [origin].
 */
data class SubscriberLink(
    /** `scheme://host[:port]`, no trailing slash, no `/sub/` part. */
    val origin: String,
    /** The opaque subscription token from `/sub/<token>`. */
    val token: String,
    /** Hostname or IP literal, without the port; the TLS pin is keyed by this. */
    val host: String,
) {
    /** The link the subscriber was given. */
    val portalUrl: String get() = "$origin/sub/$token"

    /** The read-only JSON the app refreshes from. */
    val statusUrl: String get() = "$origin/api/sub/$token"

    /** The secret-gated write endpoint that binds an address. */
    val registerUrl: String get() = "$origin/ip/$token"

    /** `host:port` for display. */
    val authority: String get() = origin.substringAfter("://")

    /** A short token form for screenshots and lists; the full value stays copyable. */
    val maskedToken: String
        get() = if (token.length <= 10) "…" else "${token.take(6)}…${token.takeLast(4)}"

    /** The URL with the token masked, for settings rows. */
    val maskedPortalUrl: String get() = "$origin/sub/$maskedToken"

    companion object {
        /**
         * Parse a pasted link. Accepts a missing scheme (https is assumed), a trailing
         * slash, a query string, or the full page URL. Returns null when the text is
         * not a subscription link at all.
         */
        @Suppress("ReturnCount")
        fun parse(raw: String): SubscriberLink? {
            // Pasted links arrive wrapped across lines, or with a stray space from a
            // chat bubble. A URL never contains whitespace, so drop all of it.
            var value = raw.replace(Regex("\\s+"), "")
            if (value.isEmpty()) return null
            if (!value.contains("://")) value = "https://$value"
            val uri = runCatching { URI(value) }.getOrNull() ?: return null
            val scheme = uri.scheme?.lowercase() ?: return null
            if (scheme != "http" && scheme != "https") return null
            val authority = uri.rawAuthority?.takeIf { it.isNotBlank() } ?: return null
            val path = uri.path ?: return null
            val marker = path.indexOf("/sub/")
            if (marker < 0) return null
            val token = path.substring(marker + "/sub/".length).trim().trim('/').substringBefore('/')
            if (token.isBlank() || token.any { it.isWhitespace() }) return null
            return SubscriberLink(
                origin = "$scheme://$authority",
                token = token,
                host = uri.host ?: authority.substringBefore(':').removeSurrounding("[", "]"),
            )
        }
    }
}
