package com.digitrob.digitdns.domain.model

/**
 * The one place the API version segment is written down.
 *
 * The v3 handlers are mounted at `/api/v3/...`, and the hidden admin path is
 * stripped *before* that segment is matched — so the effective path is
 * `/<admin-path>/api/v3/<resource>`. That `api/v3` piece is easy to drop (it was,
 * once: every request went to `/<admin-path>/<resource>` and 404ed, which the auth
 * layer then misreported as a bad credential), so it lives here and nowhere else.
 *
 * Both the typed Retrofit paths (joined by [com.digitrob.digitdns.core.network.ServerUrlProvider])
 * and the hand-built SSE URL ([Connection.streamUrl]) read it from here, so they
 * cannot drift.
 */
object ApiPathPrefix {
    /** The v3 segment including surrounding slashes: `api/v3/`. */
    const val V3 = "api/v3/"
}
