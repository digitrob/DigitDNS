package com.digitrob.digitdns.domain.model

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** Parsing the pasted subscription link, including the forms people actually paste. */
class SubscriberLinkTest {

    @Test
    fun `parses a full portal link`() {
        val link = SubscriberLink.parse("https://dns.example.com:27586/sub/abcdef0123456789")!!
        assertEquals("https://dns.example.com:27586", link.origin)
        assertEquals("abcdef0123456789", link.token)
        assertEquals("dns.example.com", link.host)
        assertEquals("https://dns.example.com:27586/api/sub/abcdef0123456789", link.statusUrl)
        assertEquals("https://dns.example.com:27586/ip/abcdef0123456789", link.registerUrl)
    }

    @Test
    fun `assumes https when the scheme is missing`() {
        val link = SubscriberLink.parse("dns.example.com/sub/abcdef")!!
        assertEquals("https://dns.example.com", link.origin)
    }

    @Test
    fun `ignores a trailing slash and a query string`() {
        val link = SubscriberLink.parse("https://dns.example.com/sub/abcdef/?theme=dark")!!
        assertEquals("abcdef", link.token)
        assertEquals("https://dns.example.com", link.origin)
    }

    @Test
    fun `rejects text that is not a subscription link`() {
        assertNull(SubscriberLink.parse(""))
        assertNull(SubscriberLink.parse("https://dns.example.com/"))
        assertNull(SubscriberLink.parse("https://dns.example.com/sub/"))
        assertNull(SubscriberLink.parse("ftp://dns.example.com/sub/abcdef"))
        assertNull(SubscriberLink.parse("dns.example.com"))
    }

    @Test
    fun `masks the token for display`() {
        val link = SubscriberLink.parse("https://dns.example.com/sub/abcdef0123456789")!!
        assertEquals("abcdef…6789", link.maskedToken)
        assertEquals("https://dns.example.com/sub/abcdef…6789", link.maskedPortalUrl)
    }
}
