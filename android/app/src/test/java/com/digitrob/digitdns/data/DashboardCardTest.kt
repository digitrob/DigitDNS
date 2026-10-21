package com.digitrob.digitdns.data

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The dashboard layout lives on disk as two strings (an order and a disabled set), so
 * these cover the parsing that decides what a returning user sees: an upgrade must not
 * hide a newly added card, and a renamed one must not crash the screen.
 */
class DashboardCardTest {

    @Test
    fun `default order lists every card`() {
        assertEquals(DashboardCard.entries.toList(), DashboardCard.defaultOrder)
    }

    @Test
    fun `blank stored order falls back to the default`() {
        assertEquals(DashboardCard.defaultOrder, DashboardCard.parseOrder(null))
        assertEquals(DashboardCard.defaultOrder, DashboardCard.parseOrder(""))
        assertEquals(DashboardCard.defaultOrder, DashboardCard.parseOrder("   "))
    }

    @Test
    fun `stored order is honoured`() {
        val stored = "SNI_PROXY,CPU_USAGE"
        val parsed = DashboardCard.parseOrder(stored)
        assertEquals(DashboardCard.SNI_PROXY, parsed.first())
        assertEquals(DashboardCard.CPU_USAGE, parsed[1])
    }

    @Test
    fun `a card the stored order does not name is appended, not lost`() {
        // Simulates an upgrade: the stored order predates a newly added card.
        val stored = "SNI_PROXY"
        val parsed = DashboardCard.parseOrder(stored)
        assertEquals(DashboardCard.entries.size, parsed.size)
        assertTrue(parsed.containsAll(DashboardCard.entries.toList()))
    }

    @Test
    fun `unknown names in the stored order are ignored`() {
        val parsed = DashboardCard.parseOrder("QUERY_RATE,NOT_A_CARD,RAM_CACHE")
        assertEquals(DashboardCard.QUERY_RATE, parsed.first())
        assertEquals(DashboardCard.RAM_CACHE, parsed[1])
    }

    @Test
    fun `an empty stored disabled set uses the off-by-default cards`() {
        assertEquals(DashboardCard.defaultDisabled, DashboardCard.parseDisabled(null))
        assertEquals(DashboardCard.defaultDisabled, DashboardCard.parseDisabled(""))
    }

    @Test
    fun `disabled set parses and ignores unknown names`() {
        val disabled = DashboardCard.parseDisabled("RAM_CACHE,BOGUS,CPU_USAGE")
        assertEquals(setOf(DashboardCard.RAM_CACHE, DashboardCard.CPU_USAGE), disabled)
    }

    @Test
    fun `every card is on by default`() {
        assertTrue(DashboardCard.QUERY_RATE.enabledByDefault)
        assertTrue(DashboardCard.RAM_USAGE.enabledByDefault)
        assertTrue(DashboardCard.defaultDisabled.isEmpty())
    }

    // A round-trip: what the store writes must parse back to the same order.
    @Test
    fun `order round-trips through its stored form`() {
        val order = DashboardCard.entries.reversed().toList()
        val stored = order.joinToString(",") { it.name }
        assertEquals(order, DashboardCard.parseOrder(stored))
    }
}
