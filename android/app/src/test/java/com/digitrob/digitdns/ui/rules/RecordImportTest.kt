package com.digitrob.digitdns.ui.rules

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class RecordImportTest {

    @Test
    fun `parses hosts and addresses with every supported separator`() {
        val result = parseRecordImport(
            """
            # comment
            a.example.com 203.0.113.9
            b.example.com, 203.0.113.10
            c.example.com; 2001:db8::1
            d.example.com=203.0.113.11
            """.trimIndent(),
            emptyMap(),
        )

        assertEquals(4, result.added)
        assertEquals("203.0.113.9", result.merged["a.example.com"])
        assertEquals("203.0.113.10", result.merged["b.example.com"])
        assertEquals("2001:db8::1", result.merged["c.example.com"])
        assertEquals("203.0.113.11", result.merged["d.example.com"])
        assertTrue(result.invalid.isEmpty())
    }

    @Test
    fun `updates changed addresses and counts unchanged ones`() {
        val result = parseRecordImport(
            """
            a.example.com 203.0.113.20
            b.example.com 203.0.113.10
            """.trimIndent(),
            mapOf("a.example.com" to "203.0.113.9", "b.example.com" to "203.0.113.10"),
        )

        assertEquals(0, result.added)
        assertEquals(1, result.updated)
        assertEquals(1, result.unchanged)
        assertEquals("203.0.113.20", result.merged["a.example.com"])
    }

    @Test
    fun `rejects wildcard hosts, bad addresses and duplicate hosts`() {
        val result = parseRecordImport(
            """
            *.example.com 203.0.113.9
            a.example.com 999.1.1.1
            b.example.com not-an-ip
            c.example.com 203.0.113.9
            c.example.com 203.0.113.10
            """.trimIndent(),
            emptyMap(),
        )

        assertEquals(1, result.added)
        assertEquals(4, result.invalid.size)
        assertFalse(result.merged.containsKey("*.example.com"))
        assertFalse(result.merged.containsKey("a.example.com"))
        assertEquals("203.0.113.9", result.merged["c.example.com"])
    }

    @Test
    fun `rejects malformed lines and accepts a trailing dot`() {
        val result = parseRecordImport(
            """
            just-a-host
            a.example.com. 203.0.113.9 extra
            b.example.com. 203.0.113.10
            """.trimIndent(),
            emptyMap(),
        )

        assertEquals(1, result.added)
        assertEquals(2, result.invalid.size)
        assertEquals("203.0.113.10", result.merged["b.example.com"])
    }

    @Test
    fun `summary reports only the non-zero buckets`() {
        val result = RecordImportResult(
            merged = emptyMap(),
            added = 2,
            updated = 1,
            unchanged = 0,
            invalid = listOf("bad"),
        )
        assertEquals("DNS rewrites: 2 added, 1 updated, 1 invalid.", result.summary())
    }
}
