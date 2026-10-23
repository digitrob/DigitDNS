package com.digitrob.digitdns.domain.model

import java.time.Instant
import java.time.ZoneOffset
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The link and bot-card templates are copied from the web panel; these tests pin the
 * exact strings so a restyle on one side cannot silently diverge from the other.
 */
class ClientLinksTest {

    private val client = Client(
        id = "client-1",
        name = "Living room",
        uuid = "uuid-1",
        token = "token-1",
        allowedIps = emptyList(),
        trafficLimitGb = 0.0,
        trafficUsedBytes = 0,
        trafficResetCycle = "",
        quotaExceeded = false,
        expiresAt = null,
        enabled = true,
        note = "",
        customPolicies = emptyList(),
        maxDevices = 0,
        registerSecret = "secret-1",
    )

    @Test
    fun subscriptionOriginTrailingSlashDoesNotDoubleThePath() {
        assertEquals(
            "https://dns.example.com:37586/sub/token-1",
            ClientLinks.subscriptionLink("https://dns.example.com:37586/", client),
        )
        assertEquals(
            "https://dns.example.com:37586/ip/token-1",
            ClientLinks.registrationEndpoint("https://dns.example.com:37586", client),
        )
    }

    @Test
    fun lifetimeExpiryUsesThePanelWording() {
        assertEquals("Lifetime (No Expiry)", ClientLinks.expiryLabel(null))
        assertEquals("", ClientLinks.expiryFieldValue(null))
    }

    @Test
    fun expiryFieldRoundTripsInUtc() {
        val value = Instant.parse("2026-12-01T12:30:00Z")
        val text = ClientLinks.expiryFieldValue(value, ZoneOffset.UTC)

        assertEquals("2026-12-01 12:30", text)
        assertEquals(value, ClientLinks.parseExpiryField(text, ZoneOffset.UTC))
        assertNull(ClientLinks.parseExpiryField("", ZoneOffset.UTC))
        assertNull(ClientLinks.parseExpiryField("not a date", ZoneOffset.UTC))
    }

    @Test
    fun botCardCarriesTheFieldsTheSubscriberActsOn() {
        val message = ClientLinks.telegramBotCard(
            client = client,
            origin = "https://dns.example.com:37586",
            dnsAddress = "77.90.63.9",
            expiresLabel = "Lifetime (No Expiry)",
        )

        assertTrue(message.contains("Client ID : client-1"))
        assertTrue(message.contains("Plan expires : Lifetime (No Expiry)"))
        assertTrue(message.contains("https://dns.example.com:37586/sub/token-1"))
        assertTrue(message.contains("Primary : 77.90.63.9"))
        assertTrue(message.contains("Secondary : 1.1.1.1"))
        assertTrue(message.contains("secret-1"))
        assertTrue(message.contains("https://dns.example.com:37586/ip/token-1"))
    }

    @Test
    fun botCardExplainsMissingSecretInsteadOfPrintingBlank() {
        val message = ClientLinks.telegramBotCard(
            client = client.copy(registerSecret = ""),
            origin = "https://dns.example.com:37586",
            dnsAddress = "",
            expiresLabel = "Lifetime (No Expiry)",
        )

        assertTrue(message.contains("(server address not configured)"))
        assertTrue(message.contains("(no secret on file"))
        assertFalse(message.contains("Registration Secret :\n\n"))
    }
}
