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
     *
     * [serverLabel] is the active install's display name, supplied only when more than
     * one server is saved — a single-install operator gains nothing from a tag that
     * has nothing to disambiguate. With it, the tag rides the title (not the text,
     * which already reflows over several lines) and joins the alert id: two installs
     * blocking the same domain are two decisions, not one, so their notifications must
     * not collapse into each other.
     */
    fun map(event: QueryEvent, serverLabel: String? = null): Alert? {
        if (!event.action.equals(ACTION_BLOCK, ignoreCase = true)) return null
        val domain = event.domain.ifBlank { "unknown domain" }
        val who = event.accountName.ifBlank { event.clientIp.ifBlank { "a client" } }
        val tag = serverLabel.orEmpty()
        return Alert(
            // Keyed on the domain so a domain hit repeatedly collapses to one
            // notification rather than a stack of them — and on the server tag too,
            // once one exists, so installs block independently.
            id = if (tag.isBlank()) domain.hashCode() else "$domain@$tag".hashCode(),
            title = if (tag.isBlank()) "Blocked: $domain" else "Blocked: $domain @ $tag",
            text = "$who tried $domain over ${event.protocol.ifBlank { "DNS" }}",
        )
    }

    private companion object {
        const val ACTION_BLOCK = "BLOCK"
    }
}
