package com.digitrob.digitdns.domain.model

import com.digitrob.digitdns.core.network.CustomGroupDto
import com.digitrob.digitdns.core.network.RelayAgentDto
import com.digitrob.digitdns.core.network.RulesResponse
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The mappings between the wire DTOs and the domain shapes. Small, but each one
 * carries a decision (unknown action falls back; a blank id means "not saved yet")
 * that a wrong default would spread through the UI.
 */
class RulesTest {

    @Test
    fun ruleListsMirrorTheWireKeys() {
        val lists = RuleLists.from(
            RulesResponse(
                customProxied = listOf("a.com"),
                customProxiedDisabled = listOf("b.com"),
                customBlocked = listOf("c.com"),
                customDirect = listOf("d.com"),
                customRecords = mapOf("e.com" to "10.0.0.1"),
            ),
        )

        assertEquals(listOf("a.com"), lists.proxied)
        assertEquals(listOf("b.com"), lists.proxiedDisabled)
        assertEquals(listOf("c.com"), lists.blocked)
        assertEquals(listOf("d.com"), lists.direct)
        assertEquals("10.0.0.1", lists.records["e.com"])
    }

    @Test
    fun ruleListWireKeysMatchTheServer() {
        assertEquals("custom_proxied", RuleList.Proxied.wireKey)
        assertEquals("custom_proxied_disabled", RuleList.ProxiedDisabled.wireKey)
        assertEquals("custom_blocked", RuleList.Blocked.wireKey)
        assertEquals("custom_direct", RuleList.Direct.wireKey)
    }

    @Test
    fun ruleListsCarryTheServiceMaps() {
        val lists = RuleLists.from(
            RulesResponse(
                services = mapOf("bbc" to "BBC iPlayer"),
                domainServices = mapOf("bbc.co.uk" to "bbc"),
                serviceIcons = mapOf("bbc" to "https://icons.duckduckgo.com/ip3/bbc.co.uk.ico"),
            ),
        )

        assertEquals("BBC iPlayer", lists.services["bbc"])
        assertEquals("bbc", lists.serviceOf("bbc.co.uk"))
        assertEquals("bbc", lists.serviceOf("BBC.CO.UK."))
        assertTrue(lists.serviceIcons["bbc"]!!.startsWith("https://"))
    }

    @Test
    fun relayOverridesAndServiceDefaultsResolve() {
        val lists = RuleLists.from(
            RulesResponse(
                services = mapOf("tv" to "German TV"),
                domainServices = mapOf("ard.example" to "tv", "zdf.example" to "tv", "service.example" to "tv"),
                domainAgents = mapOf("zdf.example" to "us", "*.ard.example" to "de"),
                serviceAgents = mapOf("tv" to "de"),
                relayAgents = mapOf(
                    "de" to RelayAgentDto(name = "Frankfurt", country = "DE"),
                    "us" to RelayAgentDto(name = "New York", country = "US"),
                ),
            ),
        )

        // A domain override wins over the service default.
        assertEquals("us", lists.relayFor("zdf.example")?.id)
        // A wildcard assignment covers the parent's subdomains, not the parent.
        assertEquals("de", lists.relayFor("live.ard.example")?.id)
        assertEquals("de", lists.relayFor("ard.example")?.id)
        // No override: the service default applies.
        assertEquals("de", lists.relayFor("service.example")?.id)
    }

    @Test
    fun anExplicitLocalOverrideBeatsTheServiceDefault() {
        val lists = RuleLists.from(
            RulesResponse(
                services = mapOf("tv" to "German TV"),
                domainServices = mapOf("ard.example" to "tv"),
                domainAgents = mapOf("ard.example" to "__local__"),
                serviceAgents = mapOf("tv" to "de"),
                relayAgents = mapOf("de" to RelayAgentDto(name = "Frankfurt", country = "DE")),
            ),
        )
        assertNull(lists.relayFor("ard.example"))
    }

    @Test
    fun serviceIdsAreStableSlugs() {
        assertEquals("bbc-iplayer", serviceIdFromName("BBC iPlayer"))
        assertEquals("ard-mediathek", serviceIdFromName("  ARD Mediathek  "))
        assertEquals("service", serviceIdFromName("***"))
    }

    @Test
    fun anIconIsEitherADomainOrAUrl() {
        assertEquals(
            "https://icons.duckduckgo.com/ip3/bbc.co.uk.ico",
            serviceIconUrl("bbc.co.uk"),
        )
        assertEquals("https://example.com/icon.png", serviceIconUrl("https://example.com/icon.png"))
        // Bare words are not a host: they would resolve to a 404 favicon and leave a hole.
        assertEquals("", serviceIconUrl("not a domain"))
        assertEquals("", serviceIconUrl(""))
    }

    @Test
    fun aStoredFaviconIsShownAsAPngAndroidCanDecode() {
        assertEquals(
            "https://www.google.com/s2/favicons?domain=bbc.co.uk&sz=64",
            serviceDisplayIcon("https://icons.duckduckgo.com/ip3/bbc.co.uk.ico"),
        )
        // Custom icons are already a format Coil renders; leave them alone.
        assertEquals("https://example.com/icon.png", serviceDisplayIcon("https://example.com/icon.png"))
    }

    @Test
    fun groupActionRoundTripsAndFallsBack() {
        assertEquals(GroupAction.Proxy, GroupAction.from("proxy"))
        assertEquals(GroupAction.Direct, GroupAction.from("direct"))
        assertEquals(GroupAction.Block, GroupAction.from("block"))
        // A future action the app does not know yet must not crash the list.
        assertEquals(GroupAction.Proxy, GroupAction.from("quarantine"))
    }

    @Test
    fun aBlankIdMeansUnsaved() {
        assertTrue(CustomGroup.from(CustomGroupDto()).isNew)
        assertFalse(CustomGroup.from(CustomGroupDto(id = "abc")).isNew)
    }
}
