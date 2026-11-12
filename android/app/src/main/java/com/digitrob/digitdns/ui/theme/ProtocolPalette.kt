package com.digitrob.digitdns.ui.theme

import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import java.util.Locale

/**
 * One colour per resolution transport, shared by the live log and the history
 * Protocols card — and deliberately the same seven roles as the `.proto-*` table in
 * web/css/style.css, so a protocol means one colour on both surfaces:
 *
 *   UDP      cyan      (the panel's --accent-cyan / info tier)
 *   TCP      blue
 *   DoT      green
 *   DoH      purple
 *   DoQ      amber
 *   DoH3     orange
 *   DNSCrypt fuchsia
 *
 * This mapping used to read UDP as [ExtendedColors.discord] (blurple) and TCP as
 * [ExtendedColors.info] (cyan) while its own comment said "UDP cyan, TCP blue" — the
 * two transports were swapped, and the live log disagreed with the resolver card
 * beside it. [ExtendedColors.info] is the accent cyan and is used for Proxy
 * everywhere else, so it could not also stand for TCP.
 */
@Composable
fun protocolColor(protocol: String): Color {
    val colors = DigitDnsTheme.colors
    return when (protocol.uppercase(Locale.ROOT)) {
        "UDP" -> colors.info
        "TCP" -> colors.blue
        "DOT" -> colors.success
        "DOH" -> colors.purple
        "DOQ" -> colors.warning
        "DOH3" -> colors.orange
        "DNSCRYPT" -> colors.fuchsia
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
