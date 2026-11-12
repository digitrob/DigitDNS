package com.digitrob.digitdns.domain.model

import com.digitrob.digitdns.core.network.EgressDto
import java.time.Instant
import java.util.Locale

/** One hub-side SOCKS5 or WireGuard egress as the options screen needs it. */
data class EgressInfo(
    val id: String,
    val kind: String,
    val name: String,
    val country: String,
    val enabled: Boolean,
    val failClosed: Boolean,
    val address: String,
    val username: String,
    val passwordSet: Boolean,
    val configSet: Boolean,
    val resolve: String,
    val health: String,
    val latencyMs: Double,
    val lastError: String,
    val lastSeen: String,
    val bytesIn: Long,
    val bytesOut: Long,
) {
    val displayName: String get() = name.ifBlank { id }
    val isWireGuard: Boolean get() = kind == "wireguard"
    val kindLabel: String get() = if (isWireGuard) "WireGuard" else "SOCKS5"
    val value: String get() = "egress:$id"
    val online: Boolean get() = enabled && health == "online"
    val offline: Boolean get() = enabled && health == "offline"
    val statusLabel: String
        get() = when {
            !enabled -> "disabled"
            health == "online" -> "online"
            health == "offline" -> "offline"
            else -> "unknown"
        }
    val detail: String
        get() = if (isWireGuard) {
            if (configSet) "Configuration saved" else "No configuration"
        } else {
            address.ifBlank { "no address" }
        }
    val latencyLabel: String
        get() = if (latencyMs > 0) String.format(Locale.US, "%.0f ms", latencyMs) else "—"
    val trafficLabel: String get() = egressTrafficLabel(bytesIn + bytesOut)
    val lastSeenLabel: String
        get() = if (lastSeen.isBlank() || lastSeen.startsWith("0001") || lastSeen.startsWith("1970")) {
            "—"
        } else {
            relayRelativeAge(lastSeen)
        }

    fun isOnline(now: Instant = Instant.now()): Boolean = online

    companion object {
        fun from(dto: EgressDto) = EgressInfo(
            id = dto.id,
            kind = dto.kind,
            name = dto.name,
            country = dto.country,
            enabled = dto.enabled,
            failClosed = dto.failClosed,
            address = dto.address,
            username = dto.username,
            passwordSet = dto.passwordSet,
            configSet = dto.configSet,
            resolve = dto.resolve,
            health = dto.health,
            latencyMs = dto.latencyMs,
            lastError = dto.lastError,
            lastSeen = dto.lastSeen,
            bytesIn = dto.bytesIn,
            bytesOut = dto.bytesOut,
        )
    }
}

/** Bytes in the panel's binary units, shared by the summary strip and a card. */
fun egressTrafficLabel(total: Long): String {
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

/** The outcome of the operator-facing egress test. */
data class EgressTestResult(
    val ok: Boolean,
    val latencyMs: Double,
    val error: String,
)
