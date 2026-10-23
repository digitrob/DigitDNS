package com.digitrob.digitdns.domain.model

import com.digitrob.digitdns.core.network.ClientDto
import java.time.Instant

/**
 * A subscriber account, as the UI thinks about it.
 *
 * The DTO carries wire shapes the screen should not reason about: bytes where a
 * person reads GB, an RFC 3339 string where the UI wants "expires in 6 days", and a
 * `traffic_reset_cycle` string that may be empty. Deriving those once here — rather
 * than in each composable — keeps the formatting rules in one place and testable
 * without a renderer.
 */
data class Client(
    val id: String,
    val name: String,
    val uuid: String,
    val token: String,
    val allowedIps: List<String>,
    val trafficLimitGb: Double,
    val trafficUsedBytes: Long,
    val trafficResetCycle: String,
    val quotaExceeded: Boolean,
    val expiresAt: Instant?,
    val enabled: Boolean,
    val note: String,
    val customPolicies: List<String>,
    val maxDevices: Int,
    val registerSecret: String,
    /** Queries served for this account since it was created. */
    val totalQueries: Long = 0,
    /** Last time a query was served, or null when the account was never seen. */
    val lastSeen: Instant? = null,
    /** When the traffic allowance next rolls over, or null with no reset cycle. */
    val nextTrafficReset: Instant? = null,
) {
    /** Bytes used as a fraction of the limit; 0 when unlimited (limit ≤ 0). */
    val quotaFraction: Float
        get() {
            if (trafficLimitGb <= 0.0) return 0f
            val limitBytes = trafficLimitGb * BYTES_PER_GB
            if (limitBytes <= 0.0) return 0f
            return (trafficUsedBytes / limitBytes).toFloat().coerceIn(0f, 1f)
        }

    /** True when the limit is a real cap (not 0/unlimited). */
    val hasQuota: Boolean get() = trafficLimitGb > 0.0

    /** Whether the account is currently usable: enabled, not over quota, not expired. */
    val isActive: Boolean
        get() = enabled && !quotaExceeded && !isExpired

    /** True when an expiry is set and it is in the past. */
    val isExpired: Boolean
        get() = expiresAt?.isBefore(Instant.now()) == true

    /**
     * Days until expiry, negative when past; null when no expiry is set. Rounded to
     * the nearest day rather than floored: a date 6 days out is "in 6 days", not "in 5",
     * and the clock moving between when the value was written and when it is read would
     * otherwise shave a day off every count.
     */
    val daysUntilExpiry: Long?
        get() = expiresAt?.let {
            val seconds = java.time.Duration.between(Instant.now(), it).seconds
            if (seconds >= 0) {
                (seconds + SECONDS_PER_DAY / 2) / SECONDS_PER_DAY
            } else {
                -((-seconds + SECONDS_PER_DAY / 2) / SECONDS_PER_DAY)
            }
        }

    companion object {
        const val BYTES_PER_GB = 1024.0 * 1024.0 * 1024.0
        private const val SECONDS_PER_DAY = 86_400L

        /** Map the wire DTO to the domain shape. */
        fun from(dto: ClientDto): Client = Client(
            id = dto.id,
            name = dto.name,
            uuid = dto.uuid,
            token = dto.token,
            allowedIps = dto.allowedIps,
            trafficLimitGb = dto.trafficLimitGb,
            trafficUsedBytes = dto.trafficUsedBytes,
            trafficResetCycle = dto.trafficResetCycle,
            quotaExceeded = dto.quotaExceeded,
            // The server omits expires_at when the account never expires, so an empty
            // string is "no expiry", not "expired at the epoch".
            expiresAt = dto.expiresAt.takeIf { it.isNotBlank() }
                ?.let { runCatching { Instant.parse(it) }.getOrNull() },
            enabled = dto.enabled,
            note = dto.note,
            customPolicies = dto.customPolicies,
            maxDevices = dto.maxDevices,
            registerSecret = dto.registerSecret,
            totalQueries = dto.totalQueries,
            // API 3.1 omits both when they have no value, so blank is "never" and
            // "no cycle" rather than the zero instant.
            lastSeen = dto.lastSeen.takeIf { it.isNotBlank() }
                ?.let { runCatching { Instant.parse(it) }.getOrNull() },
            nextTrafficReset = dto.nextTrafficReset.takeIf { it.isNotBlank() }
                ?.let { runCatching { Instant.parse(it) }.getOrNull() },
        )
    }
}
