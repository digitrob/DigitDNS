package com.digitrob.digitdns.core.notifications

/**
 * A parsed event from the live stream.
 *
 * The server sends two event names (`internal/service/stats_service.go`):
 *  - `history` — the recent-log ring buffer, once, when a subscriber connects.
 *  - `query`   — one resolved query, pushed as it happens.
 *
 * Anything else (an unknown name, or a frame that fails to decode) is dropped rather
 * than guessed at: a stream carries every client's traffic, and surfacing a
 * half-decoded event as a notification would be worse than missing it.
 */
sealed interface StreamEvent {
    /** The initial snapshot; a list of the most recent queries. */
    data class History(val events: List<QueryEvent>) : StreamEvent

    /** A single query, as it was resolved. */
    data class Query(val event: QueryEvent) : StreamEvent
}
