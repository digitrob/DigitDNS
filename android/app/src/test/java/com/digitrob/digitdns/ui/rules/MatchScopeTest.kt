package com.digitrob.digitdns.ui.rules

import com.digitrob.digitdns.domain.model.RuleLists
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** The quick-action match scopes, including wildcard rebasing and preselection. */
class MatchScopeTest {

    @Test
    fun `plain host resolves every scope`() {
        assertEquals("s-imp.rmp.rakuten.com", MatchScope.Exact.resolve("s-imp.rmp.rakuten.com"))
        assertEquals("*.s-imp.rmp.rakuten.com", MatchScope.Host.resolve("s-imp.rmp.rakuten.com"))
        assertEquals("*.rmp.rakuten.com", MatchScope.Parent.resolve("s-imp.rmp.rakuten.com"))
    }

    @Test
    fun `parent wildcard never names a TLD`() {
        assertNull(MatchScope.Parent.resolve("example.com"))
        assertNull(MatchScope.Exact.resolve(""))
    }

    @Test
    fun `a wildcard row rebases instead of refusing`() {
        // *.host on a wildcard row is the row itself, never *.*.base.
        // Exact drops the wildcard and keeps the host.
        assertEquals("a.b.c", MatchScope.Exact.resolve("*.a.b.c"))
        assertEquals("*.a.b.c", MatchScope.Host.resolve("*.a.b.c"))
        // Parent drops one label from the base.
        assertEquals("*.b.c", MatchScope.Parent.resolve("*.a.b.c"))
        assertNull(MatchScope.Parent.resolve("*.b.c"))
        // A wildcard for a bare TLD is never offered.
        assertNull(MatchScope.Host.resolve("*.com"))
    }

    @Test
    fun `opens on a scope that covers the clicked host`() {
        assertEquals(MatchScope.Exact, MatchScope.initialScope("a.b.c", setOf("a.b.c")))
        assertEquals(MatchScope.Parent, MatchScope.initialScope("a.b.c", setOf("*.b.c")))
        assertEquals(MatchScope.Exact, MatchScope.initialScope("a.b.c", emptySet()))
        // The plain rule already covers subdomains, so it wins over a redundant wildcard.
        assertEquals(MatchScope.Exact, MatchScope.initialScope("a.b.c", setOf("a.b.c", "*.a.b.c")))
        // A wildcard-only rule leaves the host itself direct. Opening on *.host
        // would write the same wildcard again, so open on the plain rule, which
        // the matcher treats as host + subdomains.
        assertEquals(MatchScope.Exact, MatchScope.initialScope("reflection.ai", setOf("*.reflection.ai")))
    }

    @Test
    fun `a scope says whether it still covers the clicked row`() {
        assertTrue(MatchScope.covers("reflection.ai", listOf("reflection.ai")))
        // A plain rule covers every subdomain.
        assertTrue(MatchScope.covers("www.reflection.ai", listOf("reflection.ai")))
        // A wildcard rule covers strict subdomains only.
        assertTrue(MatchScope.covers("www.reflection.ai", listOf("*.reflection.ai")))
        assertFalse(MatchScope.covers("reflection.ai", listOf("*.reflection.ai")))
        // A wildcard row stands for its own subtree.
        assertTrue(MatchScope.covers("*.reflection.ai", listOf("*.reflection.ai")))
        assertTrue(MatchScope.covers("*.reflection.ai", listOf("*.ai")))
        assertFalse(MatchScope.covers("reflection.ai", emptyList()))
    }

    @Test
    fun `a wildcard row opens on its own host wildcard`() {
        assertEquals(MatchScope.Host, MatchScope.initialScope("*.a.b.c", setOf("*.a.b.c")))
        // Its own rule still wins when a broader parent rule also exists.
        assertEquals(MatchScope.Host, MatchScope.initialScope("*.a.b.c", setOf("*.a.b.c", "*.b.c")))
        // With no rule of its own, the parent wildcard is the match.
        assertEquals(MatchScope.Parent, MatchScope.initialScope("*.a.b.c", setOf("*.b.c")))
        // Only the host entry exists: Exact is the base.
        assertEquals(MatchScope.Exact, MatchScope.initialScope("*.a.b.c", setOf("a.b.c")))
    }

    @Test
    fun `rule lists expose every managed pattern`() {
        val lists = RuleLists(
            proxied = listOf("one.example"),
            proxiedDisabled = listOf("two.example"),
            blocked = listOf("*.three.example"),
            direct = listOf("four.example"),
        )
        assertEquals(
            setOf("one.example", "two.example", "*.three.example", "four.example"),
            lists.managedPatterns,
        )
    }
}
