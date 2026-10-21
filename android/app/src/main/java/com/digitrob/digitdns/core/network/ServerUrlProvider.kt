package com.digitrob.digitdns.core.network

import com.digitrob.digitdns.data.ConnectionStore
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.flow.first
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import retrofit2.Retrofit
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Turns a request's relative path into the absolute URL the install actually
 * answers on.
 *
 * Retrofit needs a base URL at build time, but the install's host, port and hidden
 * admin path are all operator- or install-specific and only known after setup, so a
 * constant base would be wrong for every install. The chosen shape is a placeholder
 * base on the [Retrofit] instance plus an interceptor that rewrites each request's
 * scheme, host, port and path prefix from the stored connection. That keeps
 * Retrofit's typed interface (`@GET("clients")`) while letting the actual origin
 * move without rebuilding the client.
 *
 * The [ConnectionStore] is read with `runBlocking` inside the interceptor because
 * OkHttp interceptors are synchronous by contract; the read is a single cached
 * value, so this is not the I/O-in-hot-path it looks like — see [Cache] below.
 */
@Singleton
class ServerUrlProvider @Inject constructor(
    private val connectionStore: ConnectionStore,
) {
    @Volatile
    private var cached: com.digitrob.digitdns.domain.model.Connection? = null

    @Volatile
    private var loaded = false

    /**
     * The current connection, reading through to storage only until the first read.
     * Writes elsewhere call [invalidate] so a re-configure is picked up at once.
     */
    fun current(): com.digitrob.digitdns.domain.model.Connection? {
        if (!loaded) {
            synchronized(this) {
                if (!loaded) {
                    cached = runBlocking { connectionStore.connection.first() }
                    loaded = true
                }
            }
        }
        return cached
    }

    /** Forget the cached connection; the next [current] re-reads storage. */
    fun invalidate() {
        synchronized(this) {
            loaded = false
            cached = null
        }
    }

    /**
     * Rebase [requestUrl] onto the stored connection: same path and query, but the
     * scheme, host, port, admin path **and API-version segment** come from the install.
     *
     * Path handling: the typed interface declares paths *after* the API root
     * (`clients`, `auth/me`), and Retrofit resolves them against the placeholder base
     * (`https://127.0.0.1/`), so [requestUrl]'s path is the resource alone. The install
     * contributes the origin, the hidden admin path, and the `/api/v3/` segment — the
     * whole prefix a request needs to actually match a mounted handler:
     *
     *     https://host:port/<admin>/api/v3/<resource>
     *
     * Omitting the `api/v3` segment is the bug this method exists to prevent: the
     * server strips `/<admin>` and matches the remainder against its route table,
     * which only knows `/api/v3/...`, so a missing segment is an indistinguishable
     * 404 — not a "wrong path" the operator can see.
     */
    fun rebase(requestUrl: HttpUrl): HttpUrl {
        // Already rebased: the URL carries the "/api/v3/" segment, which no typed
        // Retrofit path ever contains. The SSE stream builds its absolute URL itself
        // (ticket in the query string) and still rides the same OkHttpClient, so its
        // interceptor would otherwise join the prefix a second time and 404. Returning
        // it untouched makes this rebase idempotent.
        if (requestUrl.isRebased()) return requestUrl

        // The install base ends in ".../<admin>/api/v3/" and the request path is the
        // resource alone, so they are joined rather than the resource overwriting the
        // prefix. A null base (no install configured) leaves the request untouched.
        val base = current()?.apiV3BaseUrl?.toHttpUrlOrNull()
        val prefix = base?.encodedPath?.trimEnd('/')
        val apiPath = requestUrl.encodedPath.trimStart('/')
        return if (base == null || prefix == null) {
            requestUrl
        } else {
            base.newBuilder()
                .encodedPath("$prefix/$apiPath")
                .encodedQuery(requestUrl.encodedQuery)
                .build()
        }
    }

    /** True when [this] already carries the `/api/v3/` API-version segment. */
    private fun HttpUrl.isRebased(): Boolean =
        encodedPath.contains("/${com.digitrob.digitdns.domain.model.ApiPathPrefix.V3}")

    /**
     * True when [url] is already an absolute install URL (carries `/api/v3/`), so it
     * needs no stored connection to be reachable — the SSE stream builds its own.
     */
    fun isAbsoluteApiUrl(url: HttpUrl): Boolean = url.isRebased()
}
