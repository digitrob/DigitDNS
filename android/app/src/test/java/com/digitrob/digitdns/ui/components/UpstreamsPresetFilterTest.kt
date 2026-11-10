package com.digitrob.digitdns.ui.components

import com.digitrob.digitdns.core.network.UpstreamPresetProvider
import com.digitrob.digitdns.core.network.UpstreamPresetServer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** Filtering for the provider browser: provider-name matches keep all servers. */
class UpstreamsPresetFilterTest {

    private val providers = listOf(
        UpstreamPresetProvider(
            id = "cloudflare",
            name = "Cloudflare",
            servers = listOf(
                UpstreamPresetServer("1.1.1.1:53", "1.1.1.1", "UDP"),
                UpstreamPresetServer("tls://one.one.one.one:853", "one.one.one.one", "DoT"),
            ),
        ),
        UpstreamPresetProvider(
            id = "google",
            name = "Google Public DNS",
            servers = listOf(
                UpstreamPresetServer("8.8.8.8:53", "8.8.8.8", "UDP"),
                UpstreamPresetServer("https://dns.google/dns-query", "dns.google", "DoH"),
            ),
        ),
        UpstreamPresetProvider(
            id = "nextdns",
            name = "NextDNS",
            servers = listOf(
                UpstreamPresetServer("45.90.28.0:53", "45.90.28.0", "UDP"),
            ),
        ),
    )

    @Test
    fun `blank query keeps every provider and server`() {
        val result = filterPresetProviders(providers, "  ")
        assertEquals(3, result.size)
        assertEquals(2, result.first().servers.size)
    }

    @Test
    fun `provider name keeps all of its servers`() {
        val result = filterPresetProviders(providers, "google")
        assertEquals(listOf("google"), result.map { it.id })
        assertEquals(listOf("8.8.8.8:53", "https://dns.google/dns-query"), result.single().servers.map { it.address })
    }

    @Test
    fun `address match narrows to the matching server`() {
        val result = filterPresetProviders(providers, "one.one")
        assertEquals(listOf("cloudflare"), result.map { it.id })
        assertEquals(listOf("tls://one.one.one.one:853"), result.single().servers.map { it.address })
    }

    @Test
    fun `protocol match finds transports across providers`() {
        val result = filterPresetProviders(providers, "doh")
        assertEquals(listOf("google"), result.map { it.id })
        assertEquals(listOf("https://dns.google/dns-query"), result.single().servers.map { it.address })
    }

    @Test
    fun `no match returns an empty list`() {
        assertTrue(filterPresetProviders(providers, "does-not-exist").isEmpty())
    }
}
