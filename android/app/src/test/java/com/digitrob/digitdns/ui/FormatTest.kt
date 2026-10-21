package com.digitrob.digitdns.ui

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Formatting is small but user-visible, and a monitor that renders 1.02 GB as
 * "1.0 GB" or a rate as nonsense erodes trust in every other number on the screen.
 */
class FormatTest {

    @Test
    fun bytesUseBinaryUnits() {
        assertEquals("512 B", formatBytes(512))
        assertEquals("1.0 KB", formatBytes(1024))
        assertEquals("1.0 MB", formatBytes(1024L * 1024))
        assertEquals("1.0 GB", formatBytes(1024L * 1024 * 1024))
    }

    @Test
    fun largeValuesDropTheDecimal() {
        // 900 GB reads better as an integer than "900.0 GB".
        assertEquals("900 GB", formatBytes((900.0 * 1024 * 1024 * 1024).toLong()))
    }

    @Test
    fun ratesSwitchToPerSecondUnits() {
        assertEquals("512 KB/s", formatRate(512.0))
        assertEquals("2.0 MB/s", formatRate(2048.0))
    }

    @Test
    fun uptimeShrinksToTheLargestTwoUnits() {
        assertEquals("12m", formatUptime(720))
        assertEquals("4h 12m", formatUptime(4 * 3600 + 12 * 60))
        assertEquals("3d 4h", formatUptime(3 * 86_400L + 4 * 3600))
    }

    @Test
    fun dayDeltaReadsRelatively() {
        assertEquals("in 6 days", formatDayDelta(6))
        assertEquals("tomorrow", formatDayDelta(1))
        assertEquals("today", formatDayDelta(0))
        assertEquals("yesterday", formatDayDelta(-1))
        assertEquals("3 days ago", formatDayDelta(-3))
    }

    @Test
    fun countsAreGrouped() {
        assertEquals("1,234,567", formatCount(1_234_567))
        assertEquals("42", formatCount(42))
    }
}
