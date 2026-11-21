package com.digitrob.digitdns.ui.theme

import com.digitrob.digitdns.ui.stream.QueryActionTone
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Test

/**
 * The AdGuard review pills carry review meanings ("add", "already covered",
 * "send back"), not query-action meanings. A pastel collision with the action
 * palette made a conflicting domain look like a routine BLOCK answer.
 */
class AdGuardImportPaletteTest {

    private val roles = listOf(
        "already" to AdGuardImportPalette.already,
        "add" to AdGuardImportPalette.add,
        "conflict" to AdGuardImportPalette.conflict,
        "sendBack" to AdGuardImportPalette.sendBack,
        "broad" to AdGuardImportPalette.broad,
        "externalUpstream" to AdGuardImportPalette.externalUpstream,
    )

    @Test
    fun allSixPillsAreDistinct() {
        assertEquals(6, roles.size)
        assertEquals(6, AdGuardImportPalette.all().toSet().size)
        roles.forEach { (name, color) ->
            roles.filter { it.first != name }.forEach { (otherName, other) ->
                assertNotEquals("$name must not match $otherName", other, color)
            }
        }
    }

    @Test
    fun noPillReusesAnActionOrStatusToken() {
        val themes = listOf(
            digitDnsExtendedColors(dark = true),
            digitDnsExtendedColors(dark = false),
            digitDnsExtendedColors(dark = true, grey = true),
        )
        val existing = themes.flatMap { colors -> colors.statusTokens() + colors.actionTokens() }
        roles.forEach { (name, color) ->
            existing.forEach { token ->
                assertNotEquals("$name must not reuse an existing palette colour", token, color)
            }
        }
    }

    @Test
    fun bucketsMapToTheirOwnPill() {
        assertEquals(AdGuardImportPalette.already, AdGuardImportPalette.forBucket("covered"))
        assertEquals(AdGuardImportPalette.add, AdGuardImportPalette.forBucket("adguardOnly"))
        assertEquals(AdGuardImportPalette.sendBack, AdGuardImportPalette.forBucket("digitdnsOnly"))
        assertEquals(AdGuardImportPalette.conflict, AdGuardImportPalette.forBucket("conflict"))
    }

    private fun ExtendedColors.statusTokens() = listOf(
        success, warning, danger, info, proxy, purple, discord, blue, orange, fuchsia, textMuted,
    )

    private fun ExtendedColors.actionTokens() = QueryActionTone.entries.map { actionToneColor(it, this) }
}
