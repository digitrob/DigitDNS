package com.digitrob.digitdns.core.notifications

/**
 * A decoded SSE frame: the event name and its data payload.
 *
 * OkHttp's `EventSource` hands the listener `onEvent(id, type, data)`, so the framing
 * (the `event:`/`data:` lines, the blank-line terminator, the `: comment` keep-alives)
 * is already stripped by the time a frame reaches us. This type is what survives that
 * stripping, and is the input the parser turns into a domain event.
 */
data class SseFrame(
    /** The `event:` name, or null when the frame carried only `data:`. */
    val type: String?,
    val data: String,
)
