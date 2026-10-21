package com.digitrob.digitdns.ui.dashboard

import com.digitrob.digitdns.data.DashboardCard
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** The dashboard's derived layout: what actually renders, given the saved prefs. */
class DashboardLayoutTest {

    @Test
    fun `visible keeps the order and drops the disabled cards`() {
        val layout = DashboardLayout(
            order = listOf(DashboardCard.SNI_PROXY, DashboardCard.CPU_USAGE, DashboardCard.QUERY_RATE),
            disabled = setOf(DashboardCard.CPU_USAGE),
        )
        assertEquals(listOf(DashboardCard.SNI_PROXY, DashboardCard.QUERY_RATE), layout.visible)
    }

    @Test
    fun `an all-disabled layout renders nothing`() {
        val layout = DashboardLayout(disabled = DashboardCard.entries.toSet())
        assertTrue(layout.visible.isEmpty())
    }

    @Test
    fun `default layout shows every card`() {
        val layout = DashboardLayout()
        assertEquals(DashboardCard.defaultOrder, layout.visible)
        assertTrue(layout.visible.contains(DashboardCard.QUERY_RATE))
        assertTrue(layout.visible.contains(DashboardCard.RESOLVER_LATENCY))
    }
}
