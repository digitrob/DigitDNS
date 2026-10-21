package com.digitrob.digitdns.ui

import java.util.Locale
import java.util.concurrent.TimeUnit
import kotlin.math.abs
import kotlin.math.roundToLong

/**
 * The formatting the panel already does in JavaScript, ported so a phone shows the
 * same numbers as the browser. These are deliberately small, pure and tested — a
 * byte count rendered as "1.0 GB" when it is really 1.02 GB is the kind of drift
 * that erodes trust in a monitoring tool.
 */

/** Render a byte count in binary units, e.g. `1.5 GB`, `900 MB`, `12 KB`. */
fun formatBytes(bytes: Long): String {
    if (bytes < 1024) return "$bytes B"
    val units = listOf("KB", "MB", "GB", "TB", "PB")
    var value = bytes.toDouble() / 1024.0
    var unit = 0
    while (value >= 1024.0 && unit < units.lastIndex) {
        value /= 1024.0
        unit++
    }
    return if (value >= 100.0) {
        String.format(Locale.US, "%.0f %s", value, units[unit])
    } else {
        String.format(Locale.US, "%.1f %s", value, units[unit])
    }
}

/** Render a transfer rate, e.g. `2.4 MB/s` from a KB/s figure. */
fun formatRate(kbps: Double): String {
    if (kbps < 1024.0) return String.format(Locale.US, "%.0f KB/s", kbps)
    return formatBytes((kbps * 1024).toLong()) + "/s"
}

/** Render an uptime as `3d 4h`, `4h 12m`, or `12m`. */
fun formatUptime(seconds: Long): String {
    val days = TimeUnit.SECONDS.toDays(seconds)
    val hours = TimeUnit.SECONDS.toHours(seconds) % 24
    val minutes = TimeUnit.SECONDS.toMinutes(seconds) % 60
    return when {
        days > 0 -> "${days}d ${hours}h"
        hours > 0 -> "${hours}h ${minutes}m"
        else -> "${minutes}m"
    }
}

/**
 * Render a signed day delta the way an operator reads an expiry: "in 6 days",
 * "today", "3 days ago". The panel shows a date; a phone shows a relative count,
 * because "is this about to lapse" is the question being asked.
 */
fun formatDayDelta(days: Long): String = when {
    days > 1 -> "in $days days"
    days == 1L -> "tomorrow"
    days == 0L -> "today"
    days == -1L -> "yesterday"
    else -> "${abs(days)} days ago"
}

/** A compact number with thousands separators, e.g. `1,234,567`. */
fun formatCount(value: Long): String = String.format(Locale.US, "%,d", value)

/** Convenience overload for the metrics the server reports as a Go `int`. */
fun formatCount(value: Int): String = formatCount(value.toLong())

/**
 * A probe latency. Sub-second values read as whole milliseconds, which is the
 * resolution the probe itself has; anything over a second falls back to one decimal
 * place so a slow target is not rounded to a misleading `1000 ms`.
 */
fun formatLatency(ms: Double): String = when {
    ms < 1000.0 -> "${ms.roundToLong()} ms"
    else -> String.format(Locale.US, "%.1f s", ms / 1000.0)
}
