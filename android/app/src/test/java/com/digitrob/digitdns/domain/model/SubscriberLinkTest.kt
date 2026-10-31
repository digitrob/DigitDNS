package com.digitrob.digitdns.domain.model

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** Parsing the pasted subscription link, including the forms people actually paste. */
class SubscriberLinkTest {

    @Test
    fun `parses a full portal link`() {
        val link = SubscriberLink.parse("https://norwich-vps.duckdns.org:27586/sub/7f37c16a4e4bbd41")!!
        assertEquals("https://norwich-vps.duckdns.org:27586", link.origin)
        assertEquals("7f37c16a4e4bbd41", link.token)
        assertEquals("norwich-vps.duckdns.org", link.host)
        assertEquals("https://norwich-vps.duckdns.org:27586/api/sub/7f37c16a4e4bbd41", link.statusUrl)
        assertEquals("https://norwich-vps.duckdns.org:27586/ip/7f37c16a4e4bbd41", link.registerUrl)
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
        val link = SubscriberLink.parse("https://dns.example.com/sub/7f37c16a4e4bbd41")!!
        assertEquals("7f37c1…bd41", link.maskedToken)
        assertEquals("https://dns.example.com/sub/7f37c1…bd41", link.maskedPortalUrl)
    }
}
