package com.digitrob.digitdns.domain.model

import com.digitrob.digitdns.core.network.AgentDto
import java.time.Instant
import java.util.Locale
import kotlin.math.roundToInt

/**
 * One relay agent as the Relay Agents screen needs it.
 *
 * [pingMs] is the last stored TCP test, not a heartbeat estimate: zero means the
 * connection has never been tested, which the UI shows as an em dash rather than a
 * perfect score. [isOnline] mirrors the panel's two-minute heartbeat window.
 */
data class RelayAgentInfo(
    val id: String,
    val name: String,
    val country: String,
    val publicIp: String,
    val relayPort: Int,
    val version: String,
    val enabled: Boolean,
    val createdAt: String,
    val lastSeen: String,
    val pingMs: Double,
    val pingAt: String,
    val bytesIn: Long,
    val bytesOut: Long,
    /** Server-computed online state; null on an older hub. */
    val online: Boolean? = null,
    /** The agent's globally routable IPv6; blank until an enabled agent reports one. */
    val publicIpv6: String = "",
    /** The per-agent opt-in. The global Proxy IPv6 switch must also be on. */
    val ipv6Enabled: Boolean = false,
    /** The control-plane revision this agent last applied; blank on an old hub. */
    val catalogueRevision: String = "",
    /** Age of that applied revision, in seconds. */
    val syncAgeSeconds: Long = 0,
) {
    val displayName: String get() = name.ifBlank { id }
    val address: String get() = publicIp.ifBlank { "not reported" }
    val ipv6Address: String get() = publicIpv6.ifBlank { "not reported" }
    val hasPing: Boolean
        get() = pingMs > 0 && pingAt.isNotBlank() && !pingAt.startsWith("0001")

    /** Compact "applied X ago" label; a blank revision means the hub did not report one. */
    val syncLabel: String
        get() {
            if (catalogueRevision.isBlank()) return "—"
            val age = syncAgeSeconds.coerceAtLeast(0)
            return when {
                age < 5 -> "just now"
                age < 60 -> "${age}s ago"
                age < 3600 -> "${age / 60}m ago"
                else -> "${age / 3600}h ago"
            }
        }

    fun isOnline(now: Instant = Instant.now()): Boolean =
        online ?: (enabled &&
            lastSeen.isNotBlank() &&
            !lastSeen.startsWith("0001") &&
            runCatching { Instant.parse(lastSeen) }.getOrNull()
                ?.isAfter(now.minusSeconds(ONLINE_WINDOW_SECONDS)) == true)

    /** Relay bytes in both directions, in the panel's binary units. */
    val trafficLabel: String
        get() {
            val total = bytesIn + bytesOut
            if (total < 1024) return "$total B"
            val units = listOf("KiB", "MiB", "GiB", "TiB")
            var value = total.toDouble()
            var unit = -1
            do {
                value /= 1024.0
                unit++
            } while (value >= 1024.0 && unit < units.lastIndex)
            return String.format(Locale.US, "%.1f %s", value, units[unit])
        }

    companion object {
        private const val ONLINE_WINDOW_SECONDS = 90L

        fun from(dto: AgentDto): RelayAgentInfo = RelayAgentInfo(
            id = dto.id,
            name = dto.name,
            country = dto.country,
            publicIp = dto.publicIp,
            relayPort = dto.relayPort,
            version = dto.version,
            enabled = dto.enabled,
            createdAt = dto.createdAt,
            lastSeen = dto.lastSeen,
            pingMs = dto.pingMs,
            pingAt = dto.pingAt,
            bytesIn = dto.bytesIn,
            bytesOut = dto.bytesOut,
            online = dto.online,
            publicIpv6 = dto.publicIpv6,
            ipv6Enabled = dto.ipv6Enabled,
            catalogueRevision = dto.catalogueRevision,
            syncAgeSeconds = dto.syncAgeSeconds,
        )
    }
}

/** A fresh agent plus the key the server only shows once. */
data class CreatedAgent(
    val agent: RelayAgentInfo,
    val key: String,
)

/** The outcome of the operator-facing TCP test. */
data class AgentTestResult(
    val ok: Boolean,
    val pingMs: Double,
    val address: String,
    val error: String,
    val agent: RelayAgentInfo?,
)

/** Time since a timestamp, written the way the panel writes it. */
fun relayRelativeAge(iso: String, now: Instant = Instant.now()): String {
    val then = if (iso.isBlank() || iso.startsWith("0001")) {
        null
    } else {
        runCatching { Instant.parse(iso) }.getOrNull()
    }
    if (then == null) return "never"
    val seconds = (now.epochSecond - then.epochSecond).coerceAtLeast(0)
    return when {
        seconds < SECONDS_PER_MINUTE -> "${seconds}s ago"
        seconds < SECONDS_PER_HOUR -> "${(seconds / SECONDS_PER_MINUTE.toDouble()).roundToInt()}m ago"
        seconds < SECONDS_PER_DAY -> "${(seconds / SECONDS_PER_HOUR.toDouble()).roundToInt()}h ago"
        else -> "${(seconds / SECONDS_PER_DAY.toDouble()).roundToInt()}d ago"
    }
}

private const val SECONDS_PER_MINUTE = 60
private const val SECONDS_PER_HOUR = 3_600
private const val SECONDS_PER_DAY = 86_400
