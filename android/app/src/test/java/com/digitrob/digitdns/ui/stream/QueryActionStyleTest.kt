package com.digitrob.digitdns.ui.stream

import org.junit.Assert.assertEquals
import org.junit.Test

class QueryActionStyleTest {

    @Test
    fun `a dns rewrite match is labelled Rewrite`() {
        val style = queryActionStyle("CUSTOM")
        assertEquals("Rewrite", style.label)
        assertEquals(QueryActionTone.Purple, style.tone)
    }
}
