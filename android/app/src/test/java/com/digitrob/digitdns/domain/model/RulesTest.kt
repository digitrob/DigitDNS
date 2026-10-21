package com.digitrob.digitdns.domain.model

import com.digitrob.digitdns.core.network.CustomGroupDto
import com.digitrob.digitdns.core.network.RulesResponse
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
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
