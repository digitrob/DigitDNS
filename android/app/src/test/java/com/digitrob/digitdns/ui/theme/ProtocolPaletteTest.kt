package com.digitrob.digitdns.ui.theme

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * The web panel and the app share one seven-colour transport ramp. These
 * assertions pin the Kotlin half to the same roles declared in the .proto-*
 * table and PROTOCOL_ROLES map; the Go contract test reads both source tables.
 */
class ProtocolPaletteTest {

    private val dark = digitDnsExtendedColors(dark = true)
    private val light = digitDnsExtendedColors(dark = false)
    private val grey = digitDnsExtendedColors(dark = true, grey = true)

    @Test
    fun everyTransportKeepsItsColourInEveryTheme() {
        for (colors in listOf(dark, light, grey)) {
            assertEquals(colors.info, protocolToneColor("UDP", colors))
            assertEquals(colors.blue, protocolToneColor("TCP", colors))
            assertEquals(colors.success, protocolToneColor("DoT", colors))
            assertEquals(colors.purple, protocolToneColor("DoH", colors))
            assertEquals(colors.warning, protocolToneColor("DoQ", colors))
            assertEquals(colors.orange, protocolToneColor("DoH3", colors))
            assertEquals(colors.fuchsia, protocolToneColor("DNSCrypt", colors))
            assertEquals(colors.textMuted, protocolToneColor("SOMETHING", colors))
        }
    }

    @Test
    fun transportKeysAndProtocolKeysResolveToTheSameColour() {
        for ((transport, protocol) in listOf(
            "plain" to "UDP",
            "dot" to "DoT",
            "doh" to "DoH",
            "doh3" to "DoH3",
            "doq" to "DoQ",
            "dnscrypt" to "DNSCrypt",
        )) {
            assertEquals(protocolToneColor(protocol, dark), transportToneColor(transport, dark))
            assertEquals(protocolToneColor(protocol, light), transportToneColor(transport, light))
        }
    }
}
