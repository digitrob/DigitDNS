package com.digitrob.digitdns.core.widget

import com.digitrob.digitdns.core.network.FilteringSettingsDto
import com.digitrob.digitdns.data.ThemeMode
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test

/** Pins the widget behaviours that would otherwise only show up on a phone. */
class WidgetValuesTest {

    @Test
    fun `qps is rendered with one decimal and bad readings are honest`() {
        assertEquals("12.3", WidgetValues.formatQps(12.34f))
        assertEquals("0.0", WidgetValues.formatQps(0f))
        assertEquals("—", WidgetValues.formatQps(Float.NaN))
    }

    @Test
    fun `resolver widget latency renders one decimal for live readings`() {
        assertEquals("8.3 ms", WidgetValues.resolverLatencyText(8.34))
        assertEquals("0.0 ms", WidgetValues.resolverLatencyText(0.04))
        assertEquals("—", WidgetValues.resolverLatencyText(0.0))
    }

    @Test
    fun `resolver widget theme resolves the per-widget override over the app theme`() {
        assertEquals(ThemeMode.DARK, WidgetValues.resolveWidgetTheme(ThemeMode.DARK, ThemeMode.LIGHT, systemDark = true))
        assertEquals(ThemeMode.LIGHT, WidgetValues.resolveWidgetTheme(ThemeMode.LIGHT, ThemeMode.DARK, systemDark = false))
    }

    @Test
    fun `resolver widget theme follows the system override only when chosen`() {
        assertEquals(ThemeMode.LIGHT, WidgetValues.resolveWidgetTheme(ThemeMode.SYSTEM, ThemeMode.DARK, systemDark = false))
        assertEquals(ThemeMode.DARK, WidgetValues.resolveWidgetTheme(ThemeMode.SYSTEM, ThemeMode.LIGHT, systemDark = true))
        assertEquals(ThemeMode.GREY, WidgetValues.resolveWidgetTheme(null, ThemeMode.GREY, systemDark = false))
    }

    @Test
    fun `resolver widget colors follow the resolved theme`() {
        assertEquals(0xFF00F0FF.toInt(), WidgetValues.resolverWidgetColors(ThemeMode.DARK).value)
        assertEquals(0xFF0E7490.toInt(), WidgetValues.resolverWidgetColors(ThemeMode.LIGHT).value)
        assertEquals(0xFFA1A1AA.toInt(), WidgetValues.resolverWidgetColors(ThemeMode.GREY).value)
        assertEquals(0xFF10B981.toInt(), WidgetValues.resolverWidgetColors(ThemeMode.DARK).toggleOn)
        assertEquals(0xFF065840.toInt(), WidgetValues.resolverWidgetColors(ThemeMode.LIGHT).toggleOn)
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
