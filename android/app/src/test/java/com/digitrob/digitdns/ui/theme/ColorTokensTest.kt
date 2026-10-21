package com.digitrob.digitdns.ui.theme

import androidx.compose.ui.graphics.Color
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Test

/**
 * The token port is only correct if the numbers land where the panel's CSS says they
 * do. These tests compare the ported colours against literals spelled the same way
 * (through Compose's own [Color]) so a transposed hex digit fails here rather than
 * showing up as a slightly-wrong card border nobody notices.
 *
 * The comparison is Color-to-Color rather than against a raw Int: Compose packs a
 * colour as a colour-space-tagged ULong, and in a JVM unit test `toArgb()` returns 0
 * (the Android graphics stub), so extracting an Int would prove nothing. Two Colors
 * built from the same hex are equal by their packed value, which is the property the
 * theme actually relies on.
 */
class ColorTokensTest {

    @Test
    fun pageBackgroundMatchesThePanelDarkSurface() {
        // bg-main = rgb(c-slate-900) and c-slate-900: 6 9 19 in the dark block.
        assertEquals(Color(0xFF060913), PanelColors.Slate900Dark)
    }

    @Test
    fun brandCyanMatchesThePanelAccent() {
        // c-cyan-400: 0 240 255 — the accent every icon and glow reads from.
        assertEquals(Color(0xFF00F0FF), PanelColors.Cyan400Dark)
    }

    @Test
    fun darkAndLightTokensActuallyDiffer() {
        // The whole reason the palette is defined twice: the light page is not the dark
        // page. If these ever match, one of the blocks was dropped in the port.
        assertNotEquals(PanelColors.Slate900Dark, PanelColors.Slate900Light)
        assertNotEquals(PanelColors.Cyan500Dark, PanelColors.Cyan500Light)
    }

    @Test
    fun extendedColorsResolvePerTheme() {
        val dark = digitDnsExtendedColors(dark = true)
        val light = digitDnsExtendedColors(dark = false)
        assertEquals(PanelColors.Slate900Dark, dark.bgMain)
        assertEquals(PanelColors.Slate900Light, light.bgMain)
        // The Discord blurple is brand-fixed and must be identical in both themes.
        assertEquals(dark.discord, light.discord)
    }
}
