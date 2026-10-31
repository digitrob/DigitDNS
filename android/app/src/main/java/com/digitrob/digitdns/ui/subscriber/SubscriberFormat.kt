package com.digitrob.digitdns.ui.subscriber

import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale
import kotlin.math.max
import kotlin.math.min

/** One countdown rendering: days/hours/minutes, or the lifetime infinity state. */
data class SubscriberCountdown(
    val days: Long,
    val hours: Long,
    val minutes: Long,
    val lifetime: Boolean,
)

private const val BYTES_PER_MB = 1024.0 * 1024.0
private const val BYTES_PER_GB = BYTES_PER_MB * 1024.0
private const val MILLIS_PER_MINUTE = 60_000L
private const val MINUTES_PER_HOUR = 60
private const val MINUTES_PER_DAY = 24 * MINUTES_PER_HOUR
private const val PERCENT = 100.0
private const val ZERO = 0.0
private const val LIFETIME_MAX_YEAR = 1

/**
 * Formatting shared by the subscriber screens. The portal renders these figures in Go;
 * the app does the same arithmetic on the same fields (traffic bytes, limit GB, RFC3339
 * expiry) so both surfaces agree.
 */
object SubscriberFormat {
    private val DATE = DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm").withZone(ZoneId.systemDefault())

    fun bytes(value: Long): String = if (value >= BYTES_PER_GB) {
        String.format(Locale.US, "%.2f GB", value / BYTES_PER_GB)
    } else {
        String.format(Locale.US, "%.2f MB", value / BYTES_PER_MB)
    }

    fun traffic(usedBytes: Long, limitGb: Double): String =
        if (limitGb <= ZERO) {
            "${bytes(usedBytes)} / Unlimited"
        } else {
            "${bytes(usedBytes)} / ${String.format(Locale.US, "%.1f GB", limitGb)}"
        }

    fun remaining(usedBytes: Long, limitGb: Double): String {
        if (limitGb <= ZERO) return "Unlimited"
        val limitBytes = limitGb * BYTES_PER_GB
        return bytes(max(ZERO, limitBytes - usedBytes).toLong())
    }

    /** 0..1, clamped; unlimited accounts stay at 0 so the bar reads as idle. */
    fun trafficFraction(usedBytes: Long, limitGb: Double): Float {
        if (limitGb <= ZERO) return 0f
        return (usedBytes / (limitGb * BYTES_PER_GB)).toFloat().coerceIn(0f, 1f)
    }

    /** Parse the server's RFC3339 expiry; a zero/lifetime date yields null. */
    fun expiry(value: String): Instant? = runCatching { Instant.parse(value) }
        .getOrNull()
        ?.takeUnless { it.atZone(ZoneId.systemDefault()).year <= LIFETIME_MAX_YEAR }

    fun date(value: String): String = expiry(value)?.let { DATE.format(it) } ?: "Lifetime"

    fun countdown(expiresAt: String, nowMillis: Long): SubscriberCountdown {
        val expiry = expiry(expiresAt)
            ?: return SubscriberCountdown(days = 0, hours = 0, minutes = 0, lifetime = true)
        val remaining = max(0L, expiry.toEpochMilli() - nowMillis)
        val minutesTotal = remaining / MILLIS_PER_MINUTE
        return SubscriberCountdown(
            days = minutesTotal / MINUTES_PER_DAY,
            hours = (minutesTotal / MINUTES_PER_HOUR) % 24,
            minutes = minutesTotal % MINUTES_PER_HOUR,
            lifetime = false,
        )
    }

    /** A short human label for the server's status word. */
    fun statusLabel(status: String): String = when (status.lowercase(Locale.US)) {
        "suspended" -> "Suspended"
        "expired" -> "Expired"
        "quota" -> "Quota spent"
        else -> "Active"
    }

    /** The next reset date, or null when the plan has no recurring quota. */
    fun reset(value: String?): String? {
        if (value.isNullOrBlank()) return null
        return runCatching { DATE.format(Instant.parse(value)) }.getOrNull()
    }

    /** Percentage for the traffic read-out, clamped to a whole number. */
    fun percentLabel(usedBytes: Long, limitGb: Double): String {
        if (limitGb <= ZERO) return "Unlimited"
        val percent = (usedBytes / (limitGb * BYTES_PER_GB) * PERCENT).toInt()
        return "${min(PERCENT.toInt(), max(0, percent))}%"
    }
}
