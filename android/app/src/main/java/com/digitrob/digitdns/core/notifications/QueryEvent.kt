package com.digitrob.digitdns.core.notifications

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * One resolved query, as the server puts it on the SSE stream.
 *
 * This mirrors `database.QueryLogItem` (`internal/database/models.go`): the same JSON
 * the dashboard's live-log consumes under the `query` event, and the elements of the
 * `history` snapshot. The field names are the wire names, so the class documents the
 * server's shape rather than inventing one.
 */
@Serializable
data class QueryEvent(
    val id: Long = 0,
    val timestamp: String = "",
    @SerialName("client_ip") val clientIp: String = "",
    @SerialName("account_name") val accountName: String = "",
    /** "UDP", "TCP", "DoT" or "DoH". */
    val protocol: String = "",
    val domain: String = "",
    @SerialName("rule_matched") val ruleMatched: String = "",
    /** "PROXY", "DIRECT" or "BLOCK". */
    val action: String = "",
    @SerialName("latency_ms") val latencyMs: Double = 0.0,
    val cached: Boolean = false,
)
