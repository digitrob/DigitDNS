package com.digitrob.digitdns.core.widget

import com.digitrob.digitdns.core.network.FilteringSettingsDto
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test

/** Pins the two widget behaviours that would otherwise only show up on a phone. */
class WidgetValuesTest {

    @Test
    fun `qps is rendered with one decimal and bad readings are honest`() {
        assertEquals("12.3", WidgetValues.formatQps(12.34f))
        assertEquals("0.0", WidgetValues.formatQps(0f))
        assertEquals("—", WidgetValues.formatQps(Float.NaN))
    }

    @Test
    fun `toggling filtering preserves the rest of the saved settings`() {
        val settings = FilteringSettingsDto(
            enabled = true,
            blockMode = "nxdomain",
            blockTtl = 45,
            updateIntervalHours = 12,
            allowlist = listOf("example.com"),
            customRules = "||ads.test^",
        )

        val request = WidgetValues.filteringToggleRequest(settings, enabled = false)

        assertFalse(request.enabled)
        assertEquals("nxdomain", request.blockMode)
        assertEquals(45, request.blockTtl)
        assertEquals(12, request.updateIntervalHours)
        assertEquals(listOf("example.com"), request.allowlist)
        assertEquals("||ads.test^", request.customRules)
    }
}
