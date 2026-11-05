package com.digitrob.digitdns.ui.theme

import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import java.util.Locale

/**
 * One colour per resolution transport, shared by the live log and the history
 * Protocols card: UDP cyan, TCP blue, DoT green, DoH purple, DoQ amber,
 * DoH3/orange-soft, DNSCrypt blurple. Unknown or blank protocols stay muted.
 */
@Composable
fun protocolColor(protocol: String): Color {
    val colors = DigitDnsTheme.colors
    return when (protocol.uppercase(Locale.ROOT)) {
        "UDP" -> colors.discord
        "TCP" -> colors.info
        "DOT" -> colors.success
        "DOH" -> colors.purple
        "DOQ" -> colors.warning
        "DOH3" -> colors.warning.copy(alpha = 0.75f)
        "DNSCRYPT" -> colors.discord.copy(alpha = 0.75f)
        else -> colors.textMuted
    }
}

/** The human display form of a protocol wire value. */
fun protocolLabel(protocol: String): String = when (protocol.uppercase(Locale.ROOT)) {
    "DOT" -> "DoT"
    "DOH" -> "DoH"
    "DOQ" -> "DoQ"
    "DOH3" -> "DoH3"
    "DNSCRYPT" -> "DNSCrypt"
    else -> protocol.trim()
}
