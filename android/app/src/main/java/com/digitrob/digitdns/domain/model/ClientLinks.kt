package com.digitrob.digitdns.domain.model

import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/** The wire value that means "no expiry" (Go's zero time), shared by create and patch. */
object ClientExpiry {
    const val NEVER = "0001-01-01T00:00:00Z"
}

/**
 * The links and messages the panel builds for one subscriber.
 *
 * The browser reads these straight out of the client card (`/sub/{token}`,
 * `/ip/{token}`, and the Telegram provisioning text); the app has to produce the
 * same strings from the same fields, or an operator copying from the phone sends
 * a customer something different from what the panel sends. Pure and tested so
 * the template cannot drift when the card is restyled.
 */
object ClientLinks {

    /** The read-only subscription page, e.g. `https://dns.example.com:37586/sub/TOKEN`. */
    fun subscriptionLink(origin: String, token: String): String = "${trimOrigin(origin)}/sub/$token"

    /** The scripted IP-registration endpoint, e.g. `…/ip/TOKEN`. */
    fun registrationEndpoint(origin: String, token: String): String = "${trimOrigin(origin)}/ip/$token"

    fun subscriptionLink(origin: String, client: Client): String = subscriptionLink(origin, client.token)

    fun registrationEndpoint(origin: String, client: Client): String = registrationEndpoint(origin, client.token)

    /**
     * The Telegram/bot provisioning card, byte-for-byte the panel's message
     * (`web/js/app.js`, `generateClientTelegramMessage`).
     *
     * [expiresLabel] is passed in rather than formatted here because it is the
     * card's most locale-sensitive line; the caller formats it once, the same way
     * the card and the detail sheet show it.
     */
    fun telegramBotCard(
        client: Client,
        origin: String,
        dnsAddress: String,
        expiresLabel: String,
    ): String {
        val serverLine = dnsAddress.ifBlank { "(server address not configured)" }
        val secretLine = client.registerSecret.ifBlank {
            "(no secret on file — regenerate it in the panel)"
        }
        return """🎮 Your SmartDNS account is ready

🔹 Client ID : ${client.id}
🔹 Plan expires : $expiresLabel

📄 Your subscription panel (status, usage and renewal):
${subscriptionLink(origin, client)}

📶 Your dedicated DNS :
🔹 Primary : $serverLine
🔹 Secondary : 1.1.1.1

Steps to register your IP :
1️⃣ Connect your phone and game console to the same internet connection .
2️⃣ Open the subscription panel link above (without a VPN/proxy) .
3️⃣ Enter the registration secret below and press "Register my IP" .
❌ Without registering your IP, DNS will not be enabled for you ❌

🔑 Registration Secret :
$secretLine

🔗 IP registration API endpoint (for automation):
${registrationEndpoint(origin, client)}"""
    }

    /** The full expiry line the card shows, or the panel's lifetime wording. */
    fun expiryLabel(expiresAt: Instant?, zone: ZoneId = ZoneId.systemDefault()): String {
        if (expiresAt == null) return "Lifetime (No Expiry)"
        val formatter = DateTimeFormatter
            .ofLocalizedDateTime(FormatStyle.MEDIUM, FormatStyle.SHORT)
            .withZone(zone)
        return formatter.format(expiresAt)
    }

    /** `yyyy-MM-dd HH:mm` in the operator's zone, for editable fields. */
    fun expiryFieldValue(expiresAt: Instant?, zone: ZoneId = ZoneId.systemDefault()): String {
        if (expiresAt == null) return ""
        return DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm").withZone(zone).format(expiresAt)
    }

    /** Parse the editable `yyyy-MM-dd HH:mm` value; null means lifetime/unparseable. */
    fun parseExpiryField(value: String, zone: ZoneId = ZoneId.systemDefault()): Instant? {
        val trimmed = value.trim()
        if (trimmed.isEmpty()) return null
        return runCatching {
            java.time.LocalDateTime
                .parse(trimmed, DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm"))
                .atZone(zone)
                .toInstant()
        }.getOrNull()
    }

    /** Trim the trailing slash in `origin` so the joined path never doubles it. */
    private fun trimOrigin(origin: String): String = origin.trimEnd('/')
}
