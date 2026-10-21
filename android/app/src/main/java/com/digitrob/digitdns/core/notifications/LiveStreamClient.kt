package com.digitrob.digitdns.core.notifications

import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.sse.EventSource
import okhttp3.sse.EventSourceListener
import okhttp3.sse.EventSources
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The OkHttp EventSource wrapper around `GET /api/v3/events/stream`.
 *
 * The stream is authenticated by a **one-time ticket in the query string**, not by
 * the usual bearer header: `EventSource` (and the browser it exists for) cannot set
 * request headers, so the server mints a short-lived ticket (`POST /auth/sse-ticket`)
 * that the stream route accepts as `?ticket=`. The app therefore mints a ticket and
 * builds the URL itself, rather than reusing [com.digitrob.digitdns.core.network.AuthInterceptor],
 * which only attaches headers.
 *
 * The [OkHttpClient] is the shared singleton: it already carries the TOFU trust
 * manager, the disabled read timeout the idle stream needs, and the connection pool.
 */
@Singleton
class LiveStreamClient @Inject constructor(
    private val client: OkHttpClient,
) {
    /** What the service hears back. Mirrors the EventSource callbacks the service uses. */
    interface Listener {
        fun onOpen()
        fun onEvent(frame: SseFrame)
        /** The stream failed or was closed; [retryable] distinguishes a drop from a refusal. */
        fun onClosed(retryable: Boolean, reason: String?)
    }

    /**
     * Open the stream. The caller owns the returned [EventSource] and must call
     * [EventSource.cancel] to stop it.
     *
     * [streamUrl] is the absolute URL including the admin path and the ticket query.
     */
    fun open(streamUrl: String, listener: Listener): EventSource {
        val request = Request.Builder()
            .url(streamUrl)
            .header("Accept", "text/event-stream")
            .header("Cache-Control", "no-cache")
            .build()

        val bridge = object : EventSourceListener() {
            override fun onOpen(eventSource: EventSource, response: Response) {
                listener.onOpen()
            }

            override fun onEvent(
                eventSource: EventSource,
                id: String?,
                type: String?,
                data: String,
            ) {
                listener.onEvent(SseFrame(type = type, data = data))
            }

            override fun onClosed(eventSource: EventSource) {
                listener.onClosed(retryable = true, reason = null)
            }

            override fun onFailure(
                eventSource: EventSource,
                t: Throwable?,
                response: Response?,
            ) {
                // A 4xx (a spent or rejected ticket, or a revoked session) is not
                // worth an immediate reconnect storm; a transport failure is.
                val code = response?.code ?: NO_RESPONSE_CODE
                val retryable = code == NO_RESPONSE_CODE ||
                    code >= SERVER_ERROR_CODE ||
                    code == REQUEST_TIMEOUT_CODE
                listener.onClosed(retryable = retryable, reason = response?.message ?: t?.message)
            }
        }

        return EventSources.createFactory(client).newEventSource(request, bridge)
    }

    private companion object {
        /** No HTTP response at all — a pure transport failure, always retryable. */
        const val NO_RESPONSE_CODE = 0

        /** 5xx: the server is there but failing; back off and retry. */
        const val SERVER_ERROR_CODE = 500

        /** 408 Request Timeout: the connection died mid-handshake; retry. */
        const val REQUEST_TIMEOUT_CODE = 408
    }
}
