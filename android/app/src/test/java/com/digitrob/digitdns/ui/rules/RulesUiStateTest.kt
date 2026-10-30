package com.digitrob.digitdns.ui.rules

import com.digitrob.digitdns.domain.model.RuleLists
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** The status-filtered Domains projection, its search/sort, and the scope parsers. */
class RulesUiStateTest {

    private val lists = RuleLists(
        proxied = listOf("a.com", "b.com", "c.com"),
        proxiedDisabled = listOf("d.com"),
        blocked = listOf("ads.com"),
        direct = listOf("lan.local"),
    )

    @Test
    fun `the proxy chip shows only the active proxied set`() {
        val state = RulesUiState(lists = lists, filter = DomainFilter.PROXY, sort = DomainSort.AZ)
        assertEquals(listOf("a.com", "b.com", "c.com"), state.rows.map { it.domain })
    }

    @Test
    fun `the paused chip shows the disabled proxied set`() {
        val state = RulesUiState(lists = lists, filter = DomainFilter.PAUSED)
        assertEquals(listOf("d.com"), state.rows.map { it.domain })
    }

    @Test
    fun `the blocked chip shows the blocked set`() {
        val state = RulesUiState(lists = lists, filter = DomainFilter.BLOCKED)
        assertEquals(listOf("ads.com"), state.rows.map { it.domain })
    }

    @Test
    fun `the direct chip shows the direct set`() {
        val state = RulesUiState(lists = lists, filter = DomainFilter.DIRECT)
        assertEquals(listOf("lan.local"), state.rows.map { it.domain })
    }

    @Test
    fun `no chip selected projects every status, direct included`() {
        val state = RulesUiState(lists = lists, sort = DomainSort.AZ)
        assertEquals(listOf("a.com", "ads.com", "b.com", "c.com", "d.com", "lan.local"), state.rows.map { it.domain })
    }

    @Test
    fun `az sort orders case-insensitively`() {
        val state = RulesUiState(
            lists = RuleLists(proxied = listOf("Zeta.com", "alpha.com")),
            sort = DomainSort.AZ,
        )
        assertEquals(listOf("alpha.com", "Zeta.com"), state.rows.map { it.domain })
    }

    @Test
    fun `za sort is the reverse of az`() {
        val state = RulesUiState(
            lists = RuleLists(proxied = listOf("alpha.com", "Zeta.com")),
            sort = DomainSort.ZA,
        )
        assertEquals(listOf("Zeta.com", "alpha.com"), state.rows.map { it.domain })
    }

    @Test
    fun `added-desc shows newest first`() {
        val state = RulesUiState(lists = lists, filter = DomainFilter.PROXY, sort = DomainSort.ADDED_DESC)
        assertEquals(listOf("c.com", "b.com", "a.com"), state.rows.map { it.domain })
    }

    @Test
    fun `search filters the rows case-insensitively`() {
        val state = RulesUiState(lists = lists, search = "B.C", sort = DomainSort.AZ)
        assertEquals(listOf("b.com"), state.rows.map { it.domain })
    }

    @Test
    fun `paused rows report disabled`() {
        val state = RulesUiState(lists = lists, filter = DomainFilter.PAUSED, search = "d.com")
        assertFalse(state.rows.first().enabled)
    }

    @Test
    fun `active rows report enabled`() {
        val state = RulesUiState(lists = lists, search = "a.com")
        assertTrue(state.rows.first().enabled)
    }

    @Test
    fun `count label reflects the view`() {
        val state = RulesUiState(lists = lists, search = "a.com")
        assertEquals("1 domains", state.countLabel)
    }

    @Test
    fun `parseDomains splits separators, drops comments and blanks, dedupes`() {
        val raw = """
            # comment line
            example.com, *.sub.example.com
            another.example.net; third.example.org

            example.com
        """.trimIndent()
        assertEquals(
            listOf("example.com", "*.sub.example.com", "another.example.net", "third.example.org"),
            RulesViewModel.parseDomains(raw),
        )
    }

    @Test
    fun `parseDomains strips trailing dots and lowercases`() {
        assertEquals(listOf("example.com"), RulesViewModel.parseDomains("  Example.com.  "))
    }

    @Test
    fun `exact scope resolves to the host`() {
        assertEquals("api.target.com", MatchScope.Exact.resolve("API.Target.com"))
    }

    @Test
    fun `host scope resolves to the own wildcard`() {
        assertEquals("*.api.target.com", MatchScope.Host.resolve("api.target.com"))
    }

    @Test
    fun `host scope refuses a bare label`() {
        assertNull(MatchScope.Host.resolve("localhost"))
    }

    @Test
    fun `parent scope resolves to the parent wildcard`() {
        assertEquals("*.target.com", MatchScope.Parent.resolve("api.target.com"))
    }

    @Test
    fun `parent scope refuses when the parent is a TLD`() {
        assertNull(MatchScope.Parent.resolve("target.com"))
    }

    @Test
    fun `changing scope rewrites the rule in the same list`() {
        val lists = RuleLists(blocked = listOf("a.b.c", "other.example"))
        val next = lists.withScope("a.b.c", MatchScope.Host)!!
        // The new pattern takes the old one's slot, so the row does not move.
        assertEquals(listOf("*.a.b.c", "other.example"), next.blocked)
        // The exact entry is gone; the status is preserved by staying in blocked.
        assertTrue(next.proxied.isEmpty() && next.direct.isEmpty())
    }

    @Test
    fun `parent then exact restores the origin from the session pattern`() {
        val lists = RuleLists(blocked = listOf("x.example", "a.b.c", "y.example"))
        val parent = lists.withScope("a.b.c", MatchScope.Parent)!!
        assertEquals(listOf("x.example", "*.b.c", "y.example"), parent.blocked)
        // The origin is no longer in the list; the previous pattern carries the status.
        val back = parent.withScope("a.b.c", MatchScope.Exact, previous = listOf("*.b.c"))!!
        assertEquals(listOf("x.example", "a.b.c", "y.example"), back.blocked)
    }

    @Test
    fun `changing scope replaces the companion wildcard too`() {
        val lists = RuleLists(proxied = listOf("a.b.c", "*.a.b.c"))
        val next = lists.withScope("a.b.c", MatchScope.Parent)!!
        assertEquals(listOf("*.b.c"), next.proxied)
    }

    @Test
    fun `exact on a wildcard row drops the wildcard`() {
        val lists = RuleLists(proxiedDisabled = listOf("*.a.b.c"))
        val next = lists.withScope("*.a.b.c", MatchScope.Exact)!!
        assertEquals(listOf("a.b.c"), next.proxiedDisabled)
    }

    @Test
    fun `a name with no rule keeps the scope as a modifier`() {
        assertNull(RuleLists().withScope("a.b.c", MatchScope.Host))
        // Parent of example.com is a TLD, so there is nothing to write.
        assertNull(RuleLists(proxied = listOf("example.com")).withScope("example.com", MatchScope.Parent))
    }

    @Test
    fun `a scope change keeps the row in the middle of the list`() {
        val lists = RuleLists(proxied = listOf("x.example", "a.b.c", "y.example"))
        val next = lists.withScope("a.b.c", MatchScope.Host)!!
        assertEquals(listOf("x.example", "*.a.b.c", "y.example"), next.proxied)
        val back = next.withScope("*.a.b.c", MatchScope.Exact)!!
        assertEquals(listOf("x.example", "a.b.c", "y.example"), back.proxied)
    }

    @Test
    fun `A-Z sorting uses the base name for wildcard rows`() {
        val state = RulesUiState(
            lists = RuleLists(proxied = listOf("b.example", "*.a.example")),
            sort = DomainSort.AZ,
        )
        assertEquals(listOf("*.a.example", "b.example"), state.rows.map { it.domain })
    }

    @Test
    fun `a redundant wildcard row is hidden next to its plain rule`() {
        val state = RulesUiState(
            lists = RuleLists(blocked = listOf("a.b.c", "*.a.b.c", "*.b.c")),
        )
        // The plain rule covers every subdomain, so *.a.b.c is not a second row.
        // The parent wildcard is a separate rule for the whole parent subtree.
        assertEquals(setOf("a.b.c", "*.b.c"), state.rows.map { it.domain }.toSet())
        val row = state.rows.first { it.domain == "a.b.c" }
        // *.a.b.c adds nothing, but the parent wildcard still applies.
        assertEquals(listOf("*.parent"), row.wildcards.map { it.type })
    }

    @Test
    fun `a wildcard in a different list stays visible`() {
        val state = RulesUiState(
            lists = RuleLists(proxied = listOf("a.b.c"), blocked = listOf("*.a.b.c")),
        )
        assertEquals(setOf("a.b.c", "*.a.b.c"), state.rows.map { it.domain }.toSet())
        val row = state.rows.first { it.domain == "a.b.c" }
        assertEquals(listOf("*.host"), row.wildcards.map { it.type })
    }

    @Test
    fun `rows name the host and parent wildcards that apply`() {
        val state = RulesUiState(
            lists = RuleLists(
                proxied = listOf("a.b.c"),
                blocked = listOf("*.a.b.c", "*.b.c"),
            ),
        )
        val row = state.rows.first { it.domain == "a.b.c" }
        assertEquals(listOf("*.host", "*.parent"), row.wildcards.map { it.type })
        assertEquals(listOf("*.a.b.c", "*.b.c"), row.wildcards.map { it.pattern })
        // The winning list is reported with each type.
        assertEquals(DomainStatus.BLOCKED, row.wildcards.first().status)
    }

    @Test
    fun `wildcard rows report their own host type`() {
        val state = RulesUiState(lists = RuleLists(proxied = listOf("*.a.b.c")))
        val row = state.rows.first { it.domain == "*.a.b.c" }
        assertEquals(listOf("*.host"), row.wildcards.map { it.type })
        assertEquals(DomainStatus.PROXY, row.wildcards.first().status)
    }

    @Test
    fun `a plain domain with no companions has no wildcard labels`() {
        val state = RulesUiState(lists = RuleLists(proxied = listOf("plain.example")))
        assertTrue(state.rows.first { it.domain == "plain.example" }.wildcards.isEmpty())
    }

    @Test
    fun wildcardRowKeepsAllItsScopesUsable() {
        assertEquals("example.com", MatchScope.Exact.resolve("*.example.com"))
        // Its own wildcard, not *.*.example.com.
        assertEquals("*.example.com", MatchScope.Host.resolve("*.example.com"))
        // The parent of example.com is a TLD, so there is no parent wildcard.
        assertNull(MatchScope.Parent.resolve("*.example.com"))
    }
}
