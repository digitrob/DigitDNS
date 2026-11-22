package com.digitrob.digitdns.ui.theme

import com.digitrob.digitdns.ui.stream.queryActionStyle
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The importer must not introduce a fourth badge palette. Its review states and
 * route chips resolve through the same theme tokens the Domains list and Live Log
 * use, so a PROXY pill or a danger-coloured state reads the same on every screen.
 */
class AdGuardImportPaletteTest {

    private val themes = listOf(
        digitDnsExtendedColors(dark = true),
        digitDnsExtendedColors(dark = false),
        digitDnsExtendedColors(dark = true, grey = true),
    )

    @Test
    fun bucketColorsArePanelTokensAndDistinct() {
        themes.forEach { colors ->
            val mapped = listOf(
                AdGuardImportPalette.bucketColor("covered", colors),
                AdGuardImportPalette.bucketColor("adguardOnly", colors),
                AdGuardImportPalette.bucketColor("conflict", colors),
                AdGuardImportPalette.bucketColor("unchanged", colors),
            )
            assertTrue(mapped.all { it in colors.paletteTokens() })
            assertEquals(4, mapped.toSet().size)
        }
    }

    @Test
    fun statusColorsFollowTheSharedVocabulary() {
        themes.forEach { colors ->
            assertEquals(colors.danger, AdGuardImportPalette.statusColor("Apply failed", colors))
            assertEquals(colors.danger, AdGuardImportPalette.statusColor("Server unavailable", colors))
            assertEquals(colors.success, AdGuardImportPalette.statusColor("Preview ready", colors))
            assertEquals(colors.success, AdGuardImportPalette.statusColor("Ready", colors))
            assertEquals(colors.success, AdGuardImportPalette.statusColor("List saved", colors))
            assertEquals(colors.info, AdGuardImportPalette.statusColor("Previewing…", colors))
            assertEquals(colors.textMuted, AdGuardImportPalette.statusColor("Idle", colors))
        }
    }

    @Test
    fun routeBadgesUseTheSharedActionPalette() {
        themes.forEach { colors ->
            listOf("proxy", "direct", "block").forEach { action ->
                assertEquals(
                    actionBadgeColor(action, colors),
                    AdGuardImportPalette.routeColor(action, colors),
                )
            }
        }
        assertEquals("Proxy", queryActionStyle("proxy").label)
        assertNotEquals("Proxy", queryActionStyle("block").label)
    }

    private fun ExtendedColors.paletteTokens() = listOf(
        success,
        warning,
        danger,
        info,
        proxy,
        purple,
        textMuted,
    )
}
