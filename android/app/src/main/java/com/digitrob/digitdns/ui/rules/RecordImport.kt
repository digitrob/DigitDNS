@file:Suppress("MatchingDeclarationName")

package com.digitrob.digitdns.ui.rules

import java.net.Inet6Address
import java.net.InetAddress
import java.util.Locale

/** The three outcomes of importing one DNS rewrite table. */
data class RecordImportResult(
    val merged: Map<String, String>,
    val added: Int,
    val updated: Int,
    val unchanged: Int,
    val invalid: List<String>,
) {
    val changed: Int get() = added + updated

    fun summary(): String {
        val parts = mutableListOf("$added added")
        if (updated > 0) parts += "$updated updated"
        if (unchanged > 0) parts += "$unchanged unchanged"
        if (invalid.isNotEmpty()) parts += "${invalid.size} invalid"
        return "DNS rewrites: ${parts.joinToString(", ")}."
    }
}

/**
 * Parse a pasted or uploaded `hostname address` table and merge it into the
 * existing DNS rewrites.
 *
 * The format matches the web panel: one pair per line, separated by whitespace,
 * commas, semicolons or `=`; `#` starts a comment; exact hostnames only. Existing
 * hosts are updated in place, identical hosts count as unchanged, and duplicate
 * hosts inside the import are rejected rather than letting the last line win.
 */
fun parseRecordImport(text: String, existing: Map<String, String>): RecordImportResult {
    val merged = LinkedHashMap(existing)
    val seen = HashSet<String>()
    var added = 0
    var updated = 0
    var unchanged = 0
    val invalid = mutableListOf<String>()

    text.lineSequence().forEach { raw ->
        val line = raw.substringBefore('#').trim()
        if (line.isEmpty()) return@forEach
        val parts = line.split(Regex("[\\s,;=]+")).filter { it.isNotEmpty() }
        if (parts.size != 2) {
            invalid += line
            return@forEach
        }
        val domain = parts[0].trim().lowercase(Locale.ROOT).trimEnd('.')
        val address = parts[1].trim()
        if (!RECORD_HOST.matches(domain) || !isIPAddress(address) || !seen.add(domain)) {
            invalid += line
            return@forEach
        }
        val previous = merged[domain]
        when {
            previous == null -> {
                merged[domain] = address
                added++
            }
            previous == address -> unchanged++
            else -> {
                merged[domain] = address
                updated++
            }
        }
    }
    return RecordImportResult(merged, added, updated, unchanged, invalid)
}

private val RECORD_HOST = Regex(
    "^([a-z0-9](?:[a-z0-9-]*[a-z0-9])?)(?:\\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+$",
)

private fun isIPAddress(value: String): Boolean =
    isIPv4(value) || isIPv6(value)

private fun isIPv4(value: String): Boolean {
    val parts = value.split('.')
    if (parts.size != IPV4_PARTS) return false
    return parts.all { part ->
        part.isNotEmpty() && part.all(Char::isDigit) &&
            (part == "0" || !part.startsWith('0')) &&
            part.toIntOrNull()?.let { it in 0..MAX_OCTET } == true
    }
}

private fun isIPv6(value: String): Boolean {
    val legal = value.contains(':') && !value.contains('%') && value.all { c ->
        c.isDigit() || c == ':' || c == '.' || c.lowercaseChar() in 'a'..'f'
    }
    return legal && runCatching { InetAddress.getByName(value) is Inet6Address }.getOrDefault(false)
}

private const val IPV4_PARTS = 4
private const val MAX_OCTET = 255
