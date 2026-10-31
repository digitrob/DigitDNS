package com.digitrob.digitdns.ui.subscriber

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/** The figures the subscriber cards show, matched to the portal's arithmetic. */
class SubscriberFormatTest {

    private val gb = 1024L * 1024L * 1024L

    @Test
    fun `traffic splits into used and limit`() {
        assertEquals("1.00 GB / 100.0 GB", SubscriberFormat.traffic(gb, 100.0))
        assertEquals("512.00 MB / 100.0 GB", SubscriberFormat.traffic(gb / 2, 100.0))
        assertEquals("1.00 GB / Unlimited", SubscriberFormat.traffic(gb, 0.0))
    }

    @Test
    fun `remaining never goes negative`() {
        assertEquals("99.00 GB", SubscriberFormat.remaining(gb, 100.0))
        assertEquals("0.00 MB", SubscriberFormat.remaining(2 * gb, 1.0))
        assertEquals("Unlimited", SubscriberFormat.remaining(gb, 0.0))
    }

    @Test
    fun `the traffic bar is clamped`() {
        assertEquals(0.5f, SubscriberFormat.trafficFraction(gb, 2.0))
        assertEquals(1f, SubscriberFormat.trafficFraction(5 * gb, 1.0))
        assertEquals(0f, SubscriberFormat.trafficFraction(gb, 0.0))
    }

    @Test
    fun `countdown reports days hours and minutes`() {
        val now = Instant.parse("2026-10-30T11:00:00Z").toEpochMilli()
        val countdown = SubscriberFormat.countdown("2026-11-01T12:30:00Z", now)
        assertFalse(countdown.lifetime)
        assertEquals(2, countdown.days)
        assertEquals(1, countdown.hours)
        assertEquals(30, countdown.minutes)
    }

    @Test
    fun `a lifetime plan has no countdown`() {
        val countdown = SubscriberFormat.countdown("0001-01-01T00:00:00Z", 0)
        assertTrue(countdown.lifetime)
        assertNull(SubscriberFormat.expiry("0001-01-01T00:00:00Z"))
    }

    @Test
    fun `status words map to the header label`() {
        assertEquals("Active", SubscriberFormat.statusLabel("active"))
        assertEquals("Suspended", SubscriberFormat.statusLabel("suspended"))
        assertEquals("Expired", SubscriberFormat.statusLabel("expired"))
        assertEquals("Quota spent", SubscriberFormat.statusLabel("quota"))
    }

    @Test
    fun `a missing reset renders nothing`() {
        assertNull(SubscriberFormat.reset(null))
        assertNull(SubscriberFormat.reset(""))
        assertNotNull(SubscriberFormat.reset("2026-11-01T10:00:00Z"))
    }
}
