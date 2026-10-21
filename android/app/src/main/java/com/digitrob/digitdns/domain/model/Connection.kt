package com.digitrob.digitdns.domain.model

/**
 * How the app is authenticated to an install.
 *
 * A companion app holds a live dashboard session (username/password + TOTP) because
 * that is what an operator has; a script holds the master REST key. The v3 server
 * accepts either on the same routes (`securityV3` unions them), so the app models
 * both and lets the operator choose — with the session path as the default, since a
 * master key is a step up in blast radius and the panel treats it that way.
 */
enum class CredentialMode {
    /** Username/password (+ TOTP) exchanged for a `digitdns_session_` bearer. */
    SESSION,

    /** The master REST API key, sent as `X-API-Key`. */
    API_KEY,
}

/**
 * A saved connection to one install.
 *
 * Every field is here because none of them can be guessed:
 *  - The panel port is operator-chosen (the deployed install uses 37586, not 443).
 *  - The admin path is a random per-install secret (`/ad90ecbb08bfa890/` on the
 *    author's install); `openapi/v3.json` describes paths only *after* it, because
 *    the handler is mounted under it and the spec cannot know the value.
 *  - The certificate is self-signed until the operator configures a real domain.
 *
 * [isHttps] is explicit rather than inferred from the port: an operator may
 * terminate TLS on a non-443 port or, rarely, run cleartext on a LAN — the app must
 * neither assume nor forbid either.
 */
data class Connection(
    /** Host or IP literal, no scheme, no trailing slash. */
    val host: String,
    /** TCP port. */
    val port: Int,
    /** The hidden admin path segment, without surrounding slashes. */
    val adminPath: String,
    /** Whether to speak https (true) or http (false). */
    val isHttps: Boolean = true,
    /** Which credential the app presents. */
    val credentialMode: CredentialMode = CredentialMode.SESSION,
    /** A human label for the saved connection list; falls back to host:port. */
    val label: String = "",
    /**
     * Stable identity for the saved install, used to key its stored credentials and
     * to mark which install is active. Generated once when the profile is created and
     * never shown to the operator. Random rather than derived from host:port because
     * two profiles may legitimately point at the same address (a re-added install),
     * and a derived key would make them share one credential slot. Blank on a
     * freshly-typed connection, which [ConnectionStore.add] fills in.
     */
    val id: String = "",
) {
    /** `host:port`, for display and for the saved-connection list. */
    val authority: String get() = "$host:$port"

    /** The label to show, never blank. */
    val displayName: String get() = label.ifBlank { authority }

    /**
     * The absolute base URL of the *install*, ending in a slash:
     * `https://host:port/adminsanitised/`. The admin path is trimmed of any slashes
     * the operator may have typed so a paste of `/abc/` and `abc` both land here.
     *
     * This is the origin only — it does **not** include the API version segment.
     * Callers that need the API root use [apiV3BaseUrl], and the SSE URL is built by
     * [streamUrl]; three separate concerns kept apart so the version segment lives in
     * exactly one place ([ApiPathPrefix]).
     */
    val baseUrl: String
        get() {
            val scheme = if (isHttps) "https" else "http"
            val path = adminPath.trim('/')
            return if (path.isEmpty()) {
                "$scheme://$host:$port/"
            } else {
                "$scheme://$host:$port/$path/"
            }
        }

    /**
     * The absolute base URL of the v3 API, ending in a slash:
     * `https://host:port/adminsanitised/api/v3/`.
     *
     * The version segment is not decoration: the server mounts the v3 handlers at
     * `/api/v3/...` *after* stripping the hidden admin path, so a request to
     * `/<admin>/auth/me` is a 404. Every typed Retrofit path is relative to *this*
     * root, and [com.digitrob.digitdns.core.network.ServerUrlProvider] joins them here.
     */
    val apiV3BaseUrl: String get() = baseUrl + ApiPathPrefix.V3

    /**
     * The absolute URL of the live event stream, including the one-time ticket.
     *
     * Built here rather than through Retrofit because the SSE client sets its
     * credential in the query string, not as a header — see
     * [com.digitrob.digitdns.core.notifications.LiveStreamClient]. [apiV3BaseUrl]
     * already ends in `api/v3/`, so only the stream resource is appended. The ticket
     * is percent-encoded because a ticket is opaque server-minted text that may
     * contain `+`, `/` or `=`.
     */
    fun streamUrl(ticket: String): String =
        apiV3BaseUrl + "events/stream?ticket=" + encodeQueryValue(ticket)

    private fun encodeQueryValue(value: String): String =
        java.net.URLEncoder.encode(value, Charsets.UTF_8.name())
}
