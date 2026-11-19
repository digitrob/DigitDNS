package com.digitrob.digitdns.ui.system

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class SettingsHubTest {
    @Test
    fun emptyQueryReturnsEverySection() {
        assertEquals(24, filterSettingsSections("").size)
        assertEquals(SettingsSection.entries, filterSettingsSections("   "))
    }

    @Test
    fun searchMatchesNamesAndKeywords() {
        assertTrue(filterSettingsSections("dnssec").any { it.name == "Dnssec" })
        assertTrue(filterSettingsSections("tokens").any { it.name == "Access" })
        assertTrue(filterSettingsSections("certificate").any { it.name == "Tls" })
        assertTrue(filterSettingsSections("wireguard").any { it.name == "RelayAgents" })
    }

    @Test
    fun searchIsCaseInsensitiveAndTrimmed() {
        assertEquals(filterSettingsSections("DNSSEC"), filterSettingsSections("  dnssec  "))
    }

    @Test
    fun unknownQueryIsEmpty() {
        assertTrue(filterSettingsSections("zzzzz").isEmpty())
    }

    @Test
    fun everySectionBelongsToAGroup() {
        val grouped = SettingsGroup.entries.sumOf { group ->
            SettingsSection.entries.count { it.group == group }
        }
        assertEquals(SettingsSection.entries.size, grouped)
    }
}
