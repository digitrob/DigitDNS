package com.digitrob.digitdns.core.notifications

import javax.inject.Inject
import javax.inject.Singleton

/**
 * Decides which stream events earn a notification, and what it says.
 *
 * The stream carries *every* query, and notifying on all of them would be a firehose
 * — the resolver answers thousands an hour and the phone would be unusable. So the
 * mapper is a filter with an opinion, and the opinion is the one the service exists
 * for: tell the operator about the queries that were **blocked**, because a blocked
 * domain is the one event in this stream that is a policy decision rather than
 * background noise.
 *
 * Pure, so the filter is tested without a notification manager or a socket. The
 * caller (the service) owns the actual `Notification` and the channel; this type owns
 * only the question "is this worth the operator's attention, and what does it say".
 */
@Singleton
class NotificationMapper @Inject constructor() {

    /** A notification that should be raised, and the text it carries. */
    data class Alert(
        /** A stable per-domain id, so repeat hits update one notification, not many. */
        val id: Int,
        val title: String,
        val text: String,
    )

    /**
     * Map one query to an alert, or null when it is not worth surfacing.
     *
     * Only a blocked query qualifies. `action` is compared case-insensitively because
     * the server documents the value as the literal `"BLOCK"` but a value the panel
     * renders by its own casing is not something to be brittle about.
     */
    fun map(event: QueryEvent): Alert? {
        if (!event.action.equals(ACTION_BLOCK, ignoreCase = true)) return null
        val domain = event.domain.ifBlank { "unknown domain" }
        val who = event.accountName.ifBlank { event.clientIp.ifBlank { "a client" } }
        return Alert(
            // Keyed on the domain so a domain hit repeatedly collapses to one
            // notification rather than a stack of them.
            id = domain.hashCode(),
            title = "Blocked: $domain",
            text = "$who tried $domain over ${event.protocol.ifBlank { "DNS" }}",
        )
    }

    private companion object {
        const val ACTION_BLOCK = "BLOCK"
    }
}
