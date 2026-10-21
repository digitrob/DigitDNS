package com.digitrob.digitdns.core.network

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import javax.inject.Inject
import javax.inject.Singleton

/**
 * A one-way "the server answered an authenticated call" signal.
 *
 * The connection pill's honest source is the SSE handshake, but that route needs a
 * ticket mint and a connect round-trip before it can report up, so a freshly opened
 * screen can sit on "Connecting" for a beat while an authed REST call — the stats
 * poll, the client list — has already proven the install is reachable. The OkHttp
 * interceptor flips this on any successful response; the stream observes it and
 * reports Online immediately rather than waiting for its own handshake.
 *
 * It is deliberately its own object, not a method on the stream: the stream builds
 * the client the interceptor runs on, so a direct reference would be a dependency
 * cycle. A counter that only ever moves forward as calls succeed is the seam that
 * breaks it.
 */
@Singleton
class Reachability @Inject constructor() {

    private val _hits = MutableStateFlow(0L)

    /** Increments once per successful authenticated response. */
    val hits: StateFlow<Long> = _hits.asStateFlow()

    /** Record that the server just answered a call. */
    fun markReachable() {
        _hits.value = _hits.value + 1
    }
}
