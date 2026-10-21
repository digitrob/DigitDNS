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
}
