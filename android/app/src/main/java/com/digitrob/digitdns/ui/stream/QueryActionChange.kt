package com.digitrob.digitdns.ui.stream

import com.digitrob.digitdns.core.notifications.QueryEvent
import java.util.LinkedHashMap
import java.util.Locale

/**
 * A session-only note that a rule was saved after [event] was answered.
 *
 * The query keeps its historical action. This marker sits beside it so the badge
 * still says what the server decided at query time while the operator can see
 * that a newer rule now points somewhere else.
 */
data class QueryActionChange(val from: String, val to: String, val at: Long)

/** Id when the server sent one; otherwise the same fallback the panel uses. */
fun queryActionChangeKey(event: QueryEvent): String = if (event.id != 0L) {
    "id:${event.id}"
} else {
    listOf(event.domain, event.timestamp, event.clientIp).joinToString("|")
}

/** The marker for one row, if a quick action changed its rule this session. */
fun QueryEvent.actionChange(changes: Map<String, QueryActionChange>): QueryActionChange? =
    changes[queryActionChangeKey(this)]

/**
 * Mark every buffered [events] row for [domain] whose historical action differs
 * from the newly saved [toAction]. Existing markers keep their original `from`,
 * so repeated saves do not forget what the query actually answered.
 */
@Suppress("LongParameterList")
fun markQueryActionChanges(
    changes: Map<String, QueryActionChange>,
    events: List<QueryEvent>,
    domain: String,
    toAction: String,
    at: Long,
    limit: Int = STREAM_CHANGE_MARKER_LIMIT,
): Map<String, QueryActionChange> {
    val badge = toAction.uppercase(Locale.ROOT)
    if (badge !in MARKABLE_ACTIONS) return changes

    val next = LinkedHashMap(changes)
    events.forEach { event ->
        if (event.domain == domain && !event.action.equals(badge, ignoreCase = true)) {
            val key = queryActionChangeKey(event)
            next[key] = QueryActionChange(next[key]?.from ?: event.action, badge, at)
        }
    }
    while (next.size > limit) {
        val oldest = next.keys.firstOrNull() ?: break
        next.remove(oldest)
    }
    return next
}

/** Matches the panel's recent-log depth: a marker lives only as long as its row. */
const val STREAM_CHANGE_MARKER_LIMIT = 200

private val MARKABLE_ACTIONS = setOf("PROXY", "BLOCK", "DIRECT")
