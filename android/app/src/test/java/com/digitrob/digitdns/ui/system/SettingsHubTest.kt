package com.digitrob.digitdns.ui.system

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class SettingsHubTest {
    @Test
    fun emptyQueryReturnsEverySection() {
        assertEquals(26, filterSettingsSections("").size)
        assertEquals(SettingsSection.entries, filterSettingsSections("   "))
    }

    @Test
    fun searchMatchesNamesAndKeywords() {
        assertTrue(filterSettingsSections("dnssec").any { it.name == "Dnssec" })
        assertTrue(filterSettingsSections("tokens").any { it.name == "Access" })
        assertTrue(filterSettingsSections("certificate").any { it.name == "Tls" })
        assertTrue(filterSettingsSections("wireguard").any { it.name == "RelayAgents" })
        assertTrue(filterSettingsSections("upstream list import").any { it.name == "AdGuardUpstreamImport" })
        assertTrue(filterSettingsSections("mobileconfig").any { it.name == "DnsProfiles" })
    }

    @Test
    fun adGuardImportSitsBesideImportConfigUnderPolicy() {
        val section = SettingsSection.AdGuardUpstreamImport
        assertEquals(SettingsGroup.Policy, section.group)
        assertEquals(
            SettingsSection.Import.ordinal + 1,
            section.ordinal,
        )
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
