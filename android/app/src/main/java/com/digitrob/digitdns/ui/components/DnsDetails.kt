package com.digitrob.digitdns.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.compositeOver
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.digitrob.digitdns.R
import com.digitrob.digitdns.core.notifications.DnsAnswer
import com.digitrob.digitdns.core.notifications.QueryEvent
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono
import com.digitrob.digitdns.ui.theme.protocolColor
import com.digitrob.digitdns.ui.theme.protocolLabel
import java.util.Locale

/**
 * Everything the shared DNS-response dialog shows, independent of which screen
 * produced it. The live log has a [QueryEvent]; history has a REST DTO. Mapping
 * both onto this model keeps the two dialogs byte-for-byte the same rather than
 * two copies that drift when one gains a field.
 */
data class DnsDetails(
    val domain: String = "",
    val latencyMs: Double = 0.0,
    val rcode: Int = 0,
    val action: String = "",
    val protocol: String = "",
    val upstreamProtocol: String = "",
    val upstream: String = "",
    val cached: Boolean = false,
    val qtype: Int = 0,
    val ruleMatched: String = "",
    val matchedPattern: String = "",
    val relayAgentName: String = "",
    val relayCountry: String = "",
    val answers: List<DnsAnswer> = emptyList(),
    val resolvedIPs: List<String> = emptyList(),
)

fun QueryEvent.toDnsDetails(): DnsDetails = DnsDetails(
    domain = domain,
    latencyMs = latencyMs,
    rcode = rcode,
    action = action,
    protocol = protocol,
    upstreamProtocol = upstreamProtocol,
    upstream = upstream,
    cached = cached,
    qtype = qtype,
    ruleMatched = ruleMatched,
    matchedPattern = matchedPattern,
    relayAgentName = relayAgentName,
    relayCountry = relayCountry,
    answers = answers,
    resolvedIPs = resolvedIPs,
)

/**
 * The read-only DNS response detail shared by the live log and query history.
 *
 * It is a compact custom dialog rather than [PanelDialog]: there is nothing to
 * confirm, so a button row would only add a way to dismiss something that already
 * closes on a tap outside or Back. The answer section is ONE Response field, one
 * record per line, so a CNAME chain reads top to bottom.
 */
@Composable
@Suppress("LongMethod", "CyclomaticComplexMethod")
fun DnsDetailsDialog(details: DnsDetails, onDismiss: () -> Unit) {
    val qtype = qtypeLabel(details.qtype)
    val rcode = rcodeLabel(details.rcode)
    val resolved = details.resolvedIPs.filter { it.isNotBlank() }
    val blockedSource = if (details.action.equals("BLOCK", ignoreCase = true)) {
        listOf(details.ruleMatched, details.matchedPattern)
            .map { it.trim() }
            .filter { it.isNotEmpty() }
            .joinToString(" · ")
    } else {
        ""
    }
    val relay = listOf(details.relayAgentName, details.relayCountry)
        .map { it.trim() }
        .filter { it.isNotEmpty() }
        .joinToString(" · ")

    val colors = DigitDnsTheme.colors
    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(usePlatformDefaultWidth = true),
    ) {
        Surface(
            modifier = Modifier
                .fillMaxWidth()
                .heightIn(max = 560.dp),
            shape = RoundedCornerShape(12.dp),
            color = colors.bgCard.compositeOver(colors.bgMain),
            contentColor = colors.textMain,
        ) {
            Column(
                modifier = Modifier
                    .padding(horizontal = 12.dp, vertical = 10.dp)
                    .verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(5.dp),
            ) {
                Text(
                    text = stringResource(R.string.live_log_info_title),
                    style = MaterialTheme.typography.titleSmall,
                    fontWeight = FontWeight.Bold,
                    color = colors.textMain,
                )
                Text(
                    text = details.domain.ifBlank { "—" },
                    style = MaterialTheme.typography.bodySmall,
                    color = colors.info,
                    fontFamily = PanelMono,
                )
                HorizontalDivider(modifier = Modifier.padding(vertical = 2.dp))
                // The time is the answer latency, not the response body: label it
                // Latency. A cache hit or block is often below 1 ms, so the value
                // keeps decimals rather than rounding to "0 ms".
                DnsDetailRow(stringResource(R.string.live_log_info_latency), formatLatencyMs(details.latencyMs), colors.textMain)
                DnsDetailRow(stringResource(R.string.live_log_info_response_code), rcode.ifBlank { "NOERROR" }, colors.textMain)
                val answers = details.answers.filter { it.value.isNotBlank() }
                val responseBlock = when {
                    answers.isNotEmpty() -> answers.joinToString("\n") { a ->
                        "${a.type.ifBlank { "?" }}: ${a.value} (ttl=${a.ttl})"
                    }
                    details.action.equals("BLOCK", ignoreCase = true) -> stringResource(R.string.live_log_info_blocked_answer)
                    resolved.isNotEmpty() -> resolved.joinToString("\n") { ip -> "A: $ip" }
                    else -> stringResource(R.string.live_log_info_no_answers)
                }
                val responseColor = when {
                    answers.isNotEmpty() -> colors.info
                    details.action.equals("BLOCK", ignoreCase = true) -> colors.danger
                    else -> colors.textMuted
                }
                DnsDetailRow(stringResource(R.string.live_log_info_response), responseBlock, responseColor)
                DnsDetailRow(stringResource(R.string.live_log_info_status), details.action, colors.textMain)
                val clientProto = protocolLabel(details.protocol)
                if (clientProto.isNotBlank()) {
                    DnsDetailRow(stringResource(R.string.live_log_info_client), clientProto, protocolColor(details.protocol))
                }
                val resolutionProto = protocolLabel(details.upstreamProtocol)
                if (resolutionProto.isNotBlank()) {
                    DnsDetailRow(
                        stringResource(R.string.live_log_info_resolved_via),
                        resolutionProto,
                        protocolColor(details.upstreamProtocol),
                    )
                }
                if (details.upstream.isNotBlank()) {
                    DnsDetailRow(stringResource(R.string.live_log_info_upstream), details.upstream, colors.textMain)
                }
                if (details.cached) {
                    DnsDetailRow(
                        stringResource(R.string.live_log_info_cache),
                        stringResource(R.string.live_log_info_cache_hit),
                        colors.textMain,
                    )
                }
                if (qtype.isNotBlank()) {
                    DnsDetailRow(stringResource(R.string.live_log_info_qtype), qtype, colors.textMain)
                }
                if (blockedSource.isNotBlank()) {
                    DnsDetailRow(stringResource(R.string.live_log_info_blocked_by), blockedSource, colors.textMain)
                }
                val category = blocklistCategory(details.ruleMatched)
                if (category.isNotBlank()) {
                    DnsDetailRow(stringResource(R.string.live_log_info_category), category, colors.textMain)
                }
                if (relay.isNotBlank()) {
                    DnsDetailRow(stringResource(R.string.live_log_info_relay), relay, colors.textMain)
                }
            }
        }
    }
}

@Composable
private fun DnsDetailRow(label: String, value: String, valueColor: Color) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(10.dp),
        verticalAlignment = Alignment.Top,
    ) {
        Text(
            text = label,
            modifier = Modifier.width(84.dp),
            style = MaterialTheme.typography.bodySmall,
            color = DigitDnsTheme.colors.textMuted,
        )
        Text(
            text = value,
            style = MaterialTheme.typography.bodySmall,
            color = valueColor,
            fontFamily = PanelMono,
        )
    }
}

private const val MS_IN_SECOND = 1000.0
private const val SUB_MILLISECOND = 1.0
private const val TEN_MILLISECONDS = 10.0

/**
 * Latency with sub-millisecond resolution. A cache hit, a block or a proxied name
 * is usually answered in well under a millisecond, and rounding those to "0 ms"
 * reads as a missing measurement. Below 1 ms this keeps three decimals.
 */
fun formatLatencyMs(ms: Double): String = when {
    ms <= 0.0 -> "0 ms"
    ms < SUB_MILLISECOND -> String.format(Locale.US, "%.3f ms", ms)
    ms < TEN_MILLISECONDS -> String.format(Locale.US, "%.2f ms", ms)
    ms < MS_IN_SECOND -> String.format(Locale.US, "%.1f ms", ms)
    else -> String.format(Locale.US, "%.1f s", ms / MS_IN_SECOND)
}

private val QTYPE_LABELS = mapOf(
    1 to "A", 2 to "NS", 5 to "CNAME", 6 to "SOA", 12 to "PTR", 15 to "MX", 16 to "TXT",
    28 to "AAAA", 33 to "SRV", 43 to "DS", 46 to "RRSIG", 48 to "DNSKEY", 64 to "SVCB",
    65 to "HTTPS", 255 to "ANY",
)

private val RCODE_LABELS = mapOf(
    0 to "NOERROR", 1 to "FORMERR", 2 to "SERVFAIL", 3 to "NXDOMAIN", 4 to "NOTIMP", 5 to "REFUSED",
)

/** A friendly category for a block, derived from the list name in rule_matched. */
fun blocklistCategory(rule: String): String {
    val r = rule.lowercase(Locale.US)
    if (r.isBlank()) return ""
    return when {
        Regex("malware|phish|threat|crypto|ransom|spam|botnet").containsMatchIn(r) -> "Malware"
        Regex("track|analytics|telemetry").containsMatchIn(r) -> "Tracker"
        Regex("\\bad|ads|advert|easylist|adblock|sponsor").containsMatchIn(r) -> "Ads"
        else -> ""
    }
}

fun qtypeLabel(value: Int): String = QTYPE_LABELS[value] ?: if (value > 0) "TYPE$value" else ""

fun rcodeLabel(value: Int): String = RCODE_LABELS[value] ?: if (value > 0) "RCODE$value" else ""
