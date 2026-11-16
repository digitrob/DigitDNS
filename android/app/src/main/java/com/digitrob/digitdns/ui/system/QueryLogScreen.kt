package com.digitrob.digitdns.ui.system

import android.text.format.Formatter
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Check
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.core.network.QueryLogItemDto
import com.digitrob.digitdns.ui.components.DnsDetails
import com.digitrob.digitdns.ui.components.DnsDetailsDialog
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelToggle
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.components.formatLatencyMs
import com.digitrob.digitdns.ui.components.rcodeLabel
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono
import com.digitrob.digitdns.ui.theme.protocolColor
import com.digitrob.digitdns.ui.theme.protocolLabel

private val QUERYLOG_ACTIONS = listOf(
    "DIRECT" to "Direct",
    "PROXY" to "Proxy",
    "BLOCK" to "Block",
    "CUSTOM" to "Custom",
    "PROXY_FAILED" to "Proxy failed",
    "PROXY_IPV6_SINK" to "IPv6 sink",
    "PROXY_HTTPS_SINK" to "HTTPS sink",
    "QUOTA" to "Quota",
)

private val QUERYLOG_PROTOCOLS = listOf("UDP", "TCP", "DoT", "DoH", "DoQ", "DoH3", "DNSCrypt")

private val QTYPE_OPTIONS = listOf(
    "" to "Any type",
    "A" to "A",
    "AAAA" to "AAAA",
    "CNAME" to "CNAME",
    "MX" to "MX",
    "TXT" to "TXT",
    "HTTPS" to "HTTPS",
    "ANY" to "ANY",
)

private val RCODE_OPTIONS = listOf(
    "" to "Any rcode",
    "NOERROR" to "NOERROR",
    "NXDOMAIN" to "NXDOMAIN",
    "SERVFAIL" to "SERVFAIL",
    "REFUSED" to "REFUSED",
)

private val CACHE_OPTIONS = listOf(
    "" to "Any cache",
    "true" to "Cached",
    "false" to "Uncached",
)

private val RANGE_OPTIONS = listOf(
    0 to "Any time",
    1 to "Last hour",
    24 to "Last 24h",
    168 to "Last 7 days",
    720 to "Last 30 days",
)

/** A one-line, single-choice filter. The trigger is a bordered field above its menu. */
@Composable
private fun ChoiceFilter(
    selected: String,
    options: List<Pair<String, String>>,
    onSelect: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    var expanded by remember { mutableStateOf(false) }
    androidx.compose.foundation.layout.Box(modifier = modifier) {
        OutlinedButton(onClick = { expanded = true }, modifier = Modifier.fillMaxWidth()) {
            Text(
                options.firstOrNull { it.first == selected }?.second ?: options.first().second,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f),
            )
            Icon(Icons.Filled.ArrowDropDown, contentDescription = null)
        }
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            options.forEach { (value, label) ->
                DropdownMenuItem(
                    text = { Text(label) },
                    trailingIcon = {
                        if (selected == value) Icon(Icons.Filled.Check, contentDescription = null)
                    },
                    onClick = {
                        expanded = false
                        onSelect(value)
                    },
                )
            }
        }
    }
}

@Composable
private fun MultiChoiceFilter(
    trigger: String,
    options: List<Pair<String, String>>,
    selected: Set<String>,
    onToggle: (String) -> Unit,
    modifier: Modifier = Modifier,
    labelOf: (String) -> String = { it },
) {
    var expanded by remember { mutableStateOf(false) }
    androidx.compose.foundation.layout.Box(modifier = modifier) {
        OutlinedButton(onClick = { expanded = true }, modifier = Modifier.fillMaxWidth()) {
            if (selected.isEmpty()) {
                Text(trigger, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
            } else {
                val first = selected.first()
                Text(
                    labelOf(first) + if (selected.size > 1) " +${selected.size - 1}" else "",
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    color = protocolColor(first),
                    style = MaterialTheme.typography.labelMedium.copy(fontFamily = PanelMono),
                    modifier = Modifier.weight(1f),
                )
            }
            Icon(Icons.Filled.ArrowDropDown, contentDescription = null)
        }
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            options.forEach { (value, label) ->
                DropdownMenuItem(
                    text = { Text(label) },
                    trailingIcon = {
                        if (selected.contains(value)) Icon(Icons.Filled.Check, contentDescription = null)
                    },
                    onClick = { onToggle(value) },
                )
            }
        }
    }
}

@Composable
private fun actionTone(action: String) = when (action) {
    "DIRECT" -> DigitDnsTheme.colors.success
    "PROXY" -> DigitDnsTheme.colors.info
    "BLOCK", "PROXY_FAILED" -> DigitDnsTheme.colors.danger
    "CUSTOM", "QUOTA" -> DigitDnsTheme.colors.warning
    "PROXY_IPV6_SINK" -> DigitDnsTheme.colors.purple
    "PROXY_HTTPS_SINK" -> DigitDnsTheme.colors.blue
    else -> MaterialTheme.colorScheme.onSurfaceVariant
}

private fun QueryLogItemDto.toDnsDetails(): DnsDetails = DnsDetails(
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
    resolvedIPs = resolvedIps,
)

@Composable
private fun UsageLine(records: Long, bytes: Long, maxMb: Int) {
    val context = LocalContext.current
    val used = Formatter.formatShortFileSize(context, bytes)
    val limit = if (maxMb > 0) " of $maxMb MB" else ""
    Text(
        "$records record${if (records == 1L) "" else "s"} · $used$limit",
        style = MaterialTheme.typography.labelSmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        fontFamily = PanelMono,
    )
}

@Composable
private fun QueryLogSettingsLoading(state: QueryLogUiState) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text("Query search", style = MaterialTheme.typography.titleMedium)
            Text(
                text = state.error ?: "Loading query log settings…",
                style = MaterialTheme.typography.bodySmall,
                color = if (state.error != null) {
                    DigitDnsTheme.colors.danger
                } else {
                    MaterialTheme.colorScheme.onSurfaceVariant
                },
            )
        }
    }
}

@Composable
@Suppress("LongMethod")
private fun SettingsCard(state: QueryLogUiState, viewModel: QueryLogViewModel) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(modifier = Modifier.weight(1f)) {
                    Text("Query search", style = MaterialTheme.typography.titleMedium)
                    Text(
                        "Optional persistent history, off by default and stored separately from statistics.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                PanelToggle(checked = state.settings.enabled, onCheckedChange = viewModel::setEnabled)
            }
            Row(
                modifier = Modifier.horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                listOf(1, 7, 30).forEach { days ->
                    FilterChip(
                        selected = state.settings.retentionDays == days,
                        onClick = { viewModel.setRetention(days) },
                        label = { Text("${days}d") },
                    )
                }
                FilterChip(
                    selected = state.settings.anonymize,
                    onClick = { viewModel.setAnonymize(!state.settings.anonymize) },
                    label = { Text("Anonymize") },
                )
            }
            Row(
                modifier = Modifier.horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                listOf(64, 128, 256, 512, 1024).forEach { mb ->
                    FilterChip(
                        selected = state.settings.maxMb == mb,
                        onClick = { viewModel.setMaxMb(mb) },
                        label = { Text(if (mb >= 1024) "${mb / 1024} GB" else "$mb MB") },
                    )
                }
            }
            UsageLine(state.usage.records, state.usage.bytes, state.settings.maxMb)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = viewModel::save, enabled = !state.isSaving) { Text("Save") }
                OutlinedButton(onClick = viewModel::clear) { Text("Clear history") }
            }
            state.error?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = DigitDnsTheme.colors.danger) }
            state.message?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = DigitDnsTheme.colors.success) }
        }
    }
}

@Composable
@Suppress("LongMethod")
private fun FiltersCard(state: QueryLogUiState, viewModel: QueryLogViewModel) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedTextField(
                value = state.domain, onValueChange = viewModel::setDomain,
                label = { Text("Domain") }, singleLine = true, modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = state.client, onValueChange = viewModel::setClient,
                label = { Text("Client or account") }, singleLine = true, modifier = Modifier.fillMaxWidth(),
            )
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                ChoiceFilter(
                    selected = state.rangeHours.toString(),
                    options = RANGE_OPTIONS.map { it.first.toString() to it.second },
                    onSelect = { viewModel.setRangeHours(it.toIntOrNull() ?: 0) },
                    modifier = Modifier.weight(1f),
                )
                ChoiceFilter(
                    selected = state.qtype,
                    options = QTYPE_OPTIONS,
                    onSelect = viewModel::setQtype,
                    modifier = Modifier.weight(1f),
                )
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                ChoiceFilter(
                    selected = state.rcode,
                    options = RCODE_OPTIONS,
                    onSelect = viewModel::setRcode,
                    modifier = Modifier.weight(1f),
                )
                ChoiceFilter(
                    selected = state.cached,
                    options = CACHE_OPTIONS,
                    onSelect = viewModel::setCached,
                    modifier = Modifier.weight(1f),
                )
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                MultiChoiceFilter(
                    trigger = "Any action",
                    options = QUERYLOG_ACTIONS,
                    selected = state.actions,
                    onToggle = viewModel::toggleAction,
                    modifier = Modifier.weight(1f),
                )
                MultiChoiceFilter(
                    trigger = "Any proto",
                    options = QUERYLOG_PROTOCOLS.map { it to protocolLabel(it) },
                    selected = state.protos,
                    onToggle = viewModel::toggleProto,
                    modifier = Modifier.weight(1f),
                    labelOf = { protocolLabel(it) },
                )
            }
            Button(onClick = viewModel::search, enabled = !state.isLoading) {
                Text(if (state.isLoading) "Searching…" else "Search")
            }
        }
    }
}

@Composable
@Suppress("LongMethod")
private fun HistoryRow(item: QueryLogItemDto, onOpen: () -> Unit) {
    val rcode = rcodeLabel(item.rcode).ifBlank { "NOERROR" }
    val proto = item.upstreamProtocol.ifBlank { item.protocol }
    val context = listOf(item.accountName, item.clientIp, item.ruleMatched)
        .filter { it.isNotBlank() }
        .joinToString(" · ")
    PanelCard(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClickLabel = "Show DNS response details", onClick = onOpen),
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(3.dp)) {
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
                StatusBadge(item.action.ifBlank { "DIRECT" }, actionTone(item.action), uppercase = false)
                Text(
                    item.domain.ifBlank { "—" },
                    style = MaterialTheme.typography.bodyMedium,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f),
                )
                Text(formatLatencyMs(item.latencyMs), style = MaterialTheme.typography.labelSmall, fontFamily = PanelMono)
            }
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
                Text(
                    context,
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f),
                )
                if (item.cached) {
                    Text("RAM", style = MaterialTheme.typography.labelSmall, color = DigitDnsTheme.colors.success)
                }
                Text(rcode, style = MaterialTheme.typography.labelSmall, fontFamily = PanelMono)
                if (proto.isNotBlank()) {
                    Text(
                        protocolLabel(proto),
                        style = MaterialTheme.typography.labelSmall.copy(fontFamily = PanelMono),
                        color = protocolColor(proto),
                    )
                }
            }
            if (item.timestamp.isNotBlank()) {
                Text(
                    item.timestamp,
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

/**
 * Persistent query history with cursor paging. The list is a LazyColumn rather than
 * a Column inside verticalScroll: the old screen showed the first eight rows and a
 * "Showing first 8 of N" caption, which silently hid the rest of the answer. Load
 * older appends the next page from the server cursor.
 */
@Composable
fun QueryLogScreen(viewModel: QueryLogViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    var detail by remember { mutableStateOf<DnsDetails?>(null) }

    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = androidx.compose.foundation.layout.PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        item {
            if (state.settingsLoaded) {
                SettingsCard(state, viewModel)
            } else {
                QueryLogSettingsLoading(state)
            }
        }
        item { FiltersCard(state, viewModel) }
        if (state.items.isEmpty()) {
            item {
                PanelCard(modifier = Modifier.fillMaxWidth()) {
                    Text(
                        if (state.isLoading) "Searching…" else "No matching queries.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        } else {
            item {
                Text(
                    "${state.items.size} loaded",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            items(state.items) { item ->
                HistoryRow(item) { detail = item.toDnsDetails() }
            }
            if (state.hasMore) {
                item {
                    OutlinedButton(
                        onClick = viewModel::loadMore,
                        enabled = !state.isLoadingMore,
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(if (state.isLoadingMore) "Loading…" else "Load older")
                    }
                }
            }
        }
    }

    detail?.let { DnsDetailsDialog(it, onDismiss = { detail = null }) }
}
