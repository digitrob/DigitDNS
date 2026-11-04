package com.digitrob.digitdns.core.network

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import com.digitrob.digitdns.domain.model.HistoryRange
import org.junit.Test

/**
 * The DTOs the dashboard racer and traffic tiles read. The v3 stats payload has no
 * `total_bytes_transferred` (the panel's own endpoint computes it), so the app derives
 * it — and the upstream stat carries latency both as nanoseconds and as the server's
 * pre-divided milliseconds, which must agree.
 */
class StatsModelTest {

    @Test
    fun `total bytes transferred sums sent and received`() {
        val stats = LiveStats(bytesSent = 1_500, bytesRecv = 500)
        assertEquals(2_000L, stats.totalBytesTransferred)
    }

    @Test
    fun `upstream stat prefers the server's millisecond value`() {
        val stat = UpstreamStat(address = "8.8.8.8:53", latency = 12_000_000, latencyMs = 12.5)
        assertEquals(12.5, stat.displayLatencyMs, 0.0001)
    }

    @Test
    fun `upstream stat falls back to nanoseconds when milliseconds are absent`() {
        val stat = UpstreamStat(address = "8.8.8.8:53", latency = 12_000_000, latencyMs = 0.0)
        assertEquals(12.0, stat.displayLatencyMs, 0.0001)
    }

    @Test
    fun `an upstream with no errors reads healthy, one with errors does not`() {
        assertTrue(UpstreamStat(errors = 0).isHealthy)
        assertFalse(UpstreamStat(errors = 1).isHealthy)
    }

    @Test
    fun `history allowed subtracts both blocked and proxied`() {
        val point = StatsSeriesPoint(queries = 100, blocked = 20, proxied = 15)
        assertEquals(65L, point.allowed)
    }

    @Test
    fun `history allowed never goes negative`() {
        assertEquals(0L, StatsSeriesPoint(queries = 5, blocked = 4, proxied = 4).allowed)
    }

    @Test
    fun `history range wire values round-trip`() {
        assertEquals("24h", HistoryRange.fromWire("24h").wire)
        assertEquals("90d", HistoryRange.fromWire("90d").wire)
        assertEquals(HistoryRange.LAST_24H, HistoryRange.fromWire("bogus"))
    }

    @Test
    fun `a latency snapshot with no observations reports no data`() {
        assertFalse(LatencySnapshot().hasData)
        assertTrue(LatencySnapshot(count = 1, p50Ms = 3.0).hasData)
    }
}
