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
    /** DNS question type, e.g. 1 (A) or 28 (AAAA). Zero when the server omitted it. */
    val qtype: Int = 0,
    /** DNS response code: 0 NOERROR, 2 SERVFAIL, 3 NXDOMAIN, 5 REFUSED. */
    val rcode: Int = 0,
    /** The rule pattern that decided the action; empty for the direct fallback. */
    @SerialName("matched_pattern") val matchedPattern: String = "",
    /** The resolver that answered: "cache" for a RAM hit, empty for local answers. */
    val upstream: String = "",
    /** How that resolver was queried (UDP/TCP/DoT/DoH/DoQ/DoH3/DNSCrypt); empty for cache/local. */
    @SerialName("upstream_protocol") val upstreamProtocol: String = "",
    /** The relay agent this proxied query has left through, if any. */
    @SerialName("relay_agent_name") val relayAgentName: String = "",
    @SerialName("relay_country") val relayCountry: String = "",
    /** A/AAAA IP addresses from the DNS response; empty when not an address record. */
    @SerialName("resolved_ips") val resolvedIPs: List<String> = emptyList(),
    /** The answer section in wire order (CNAME chain, then addresses), with TTLs. */
    val answers: List<DnsAnswer> = emptyList(),
)

/** One record from the resolver's answer section, e.g. "A" / "13.227.155.73" / 8. */
@Serializable
data class DnsAnswer(
    val type: String = "",
    val value: String = "",
    val ttl: Long = 0,
)
