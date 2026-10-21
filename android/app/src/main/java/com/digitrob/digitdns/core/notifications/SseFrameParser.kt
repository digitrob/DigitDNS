package com.digitrob.digitdns.core.notifications

import kotlinx.serialization.json.Json
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Decodes an [SseFrame] into a [StreamEvent], or null when it is not one we handle.
 *
 * Pure and dependency-light on purpose — this is where the stream's contract is
 * checked, so it is unit-tested without a socket. The two decided cases are the
 * server's two event names; a keep-alive never reaches here (OkHttp strips `:`
 * comment lines), and an unknown name returns null so a future server event does not
 * crash an older client.
 */
@Singleton
class SseFrameParser @Inject constructor() {

    private val json = Json { ignoreUnknownKeys = true }

    fun parse(frame: SseFrame): StreamEvent? {
        if (frame.data.isBlank()) return null
        return when (frame.type) {
            EVENT_HISTORY -> decodeHistory(frame.data)
            EVENT_QUERY -> decodeQuery(frame.data)
            else -> null
        }
    }

    private fun decodeQuery(data: String): StreamEvent.Query? =
        runCatching { json.decodeFromString(QueryEvent.serializer(), data) }
            .getOrNull()
            ?.let(StreamEvent::Query)

    private fun decodeHistory(data: String): StreamEvent.History? =
        runCatching { json.decodeFromString(HISTORY_SERIALIZER, data) }
            .getOrNull()
            ?.let { StreamEvent.History(it) }

    private companion object {
        const val EVENT_HISTORY = "history"
        const val EVENT_QUERY = "query"

        /** `ListSerializer` is a dependency of the generated one; declared for clarity. */
        val HISTORY_SERIALIZER = kotlinx.serialization.builtins.ListSerializer(QueryEvent.serializer())
    }
}
