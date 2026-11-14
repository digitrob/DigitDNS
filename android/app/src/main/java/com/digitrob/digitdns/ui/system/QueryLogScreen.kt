package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Check
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelToggle
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
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

@Composable
private fun ActionFilter(
    selected: Set<String>,
    onToggle: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    var expanded by remember { mutableStateOf(false) }
    androidx.compose.foundation.layout.Box(modifier = modifier) {
        OutlinedButton(onClick = { expanded = true }, modifier = Modifier.fillMaxWidth()) {
            Row(
                modifier = Modifier.weight(1f),
                horizontalArrangement = Arrangement.spacedBy(4.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                if (selected.isEmpty()) {
                    Text("Any action", maxLines = 1, overflow = TextOverflow.Ellipsis)
                } else {
                    val value = selected.first()
                    val label = QUERYLOG_ACTIONS.firstOrNull { it.first == value }?.second ?: value
                    // The badge yields width to the count, so "+N" is never clipped
                    // even when the first label is long.
                    StatusBadge(
                        label,
                        actionTone(value),
                        modifier = Modifier.weight(1f, fill = false),
                        uppercase = false,
                    )
                    if (selected.size > 1) {
                        Text(
                            "+${selected.size - 1}",
                            maxLines = 1,
                            modifier = Modifier.padding(start = 2.dp),
                        )
                    }
                }
            }
            androidx.compose.material3.Icon(
                Icons.Filled.ArrowDropDown,
                contentDescription = null,
            )
        }
        androidx.compose.material3.DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            QUERYLOG_ACTIONS.forEach { (value, label) ->
                androidx.compose.material3.DropdownMenuItem(
                    text = { StatusBadge(label, actionTone(value), uppercase = false) },
                    trailingIcon = {
                        if (selected.contains(value)) {
                            androidx.compose.material3.Icon(
                                Icons.Filled.Check,
                                contentDescription = null,
                            )
                        }
                    },
                    onClick = { onToggle(value) },
                )
            }
        }
    }
}

private val QUERYLOG_PROTOCOLS = listOf("UDP", "TCP", "DoT", "DoH", "DoQ", "DoH3", "DNSCrypt")

@Composable
private fun ProtoFilter(
    selected: Set<String>,
    onToggle: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    var expanded by remember { mutableStateOf(false) }
    androidx.compose.foundation.layout.Box(modifier = modifier) {
        OutlinedButton(onClick = { expanded = true }, modifier = Modifier.fillMaxWidth()) {
            Row(
                modifier = Modifier.weight(1f),
                horizontalArrangement = Arrangement.spacedBy(4.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                if (selected.isEmpty()) {
                    Text("Any proto", maxLines = 1, overflow = TextOverflow.Ellipsis)
                } else {
                    // One chip plus a count keeps the trigger a fixed height, so
                    // extra selections never stretch the card. The name yields
                    // width so the count stays visible.
                    val first = selected.first()
                    Text(
                        protocolLabel(first),
                        style = MaterialTheme.typography.labelMedium.copy(fontFamily = com.digitrob.digitdns.ui.theme.PanelMono),
                        color = protocolColor(first),
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f, fill = false),
                    )
                    if (selected.size > 1) {
                        Text(
                            "+${selected.size - 1}",
                            maxLines = 1,
                            modifier = Modifier.padding(start = 2.dp),
                        )
                    }
                }
            }
            androidx.compose.material3.Icon(
                Icons.Filled.ArrowDropDown,
                contentDescription = null,
            )
        }
        androidx.compose.material3.DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            QUERYLOG_PROTOCOLS.forEach { proto ->
                androidx.compose.material3.DropdownMenuItem(
                    text = {
                        Text(
                            protocolLabel(proto),
                            style = MaterialTheme.typography.labelMedium.copy(fontFamily = com.digitrob.digitdns.ui.theme.PanelMono),
                            color = protocolColor(proto),
                        )
                    },
                    trailingIcon = {
                        if (selected.contains(proto)) {
                            androidx.compose.material3.Icon(Icons.Filled.Check, contentDescription = null)
                        }
                    },
                    onClick = { onToggle(proto) },
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

@Suppress("LongMethod")
@Composable
fun QueryLogScreen(viewModel: QueryLogViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    // Portrait fits the cards without scrolling, but landscape is much shorter,
    // so the column must scroll whenever the content is taller than the viewport.
    Column(
        modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
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
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
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
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Button(onClick = viewModel::save, enabled = !state.isSaving) { Text("Save") }
                    OutlinedButton(onClick = viewModel::clear) { Text("Clear history") }
                }
                state.error?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = DigitDnsTheme.colors.danger) }
                state.message?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = DigitDnsTheme.colors.success) }
            }
        }

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
                    ActionFilter(
                        selected = state.actions,
                        onToggle = viewModel::toggleAction,
                        modifier = Modifier.weight(1f),
                    )
                    ProtoFilter(
                        selected = state.protos,
                        onToggle = viewModel::toggleProto,
                        modifier = Modifier.weight(1f),
                    )
                }
                Button(onClick = viewModel::search, enabled = !state.isLoading) {
                    Text(if (state.isLoading) "Searching…" else "Search")
                }
            }
        }

        PanelCard(modifier = Modifier.fillMaxWidth()) {
            if (state.items.isEmpty()) {
                Text(
                    "No matching queries.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            } else {
                Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    state.items.take(8).forEach { item ->
                        Column(modifier = Modifier.fillMaxWidth().padding(vertical = 3.dp)) {
                            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                                Text(item.action, style = MaterialTheme.typography.labelSmall, color = DigitDnsTheme.colors.info)
                                Text(item.domain, style = MaterialTheme.typography.bodySmall, modifier = Modifier.weight(1f))
                                Text("${item.latencyMs.toInt()} ms", style = MaterialTheme.typography.labelSmall)
                            }
                            Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                                Text(
                                    listOf(item.accountName, item.clientIp, item.ruleMatched)
                                        .filter { it.isNotBlank() }.joinToString(" · "),
                                    style = MaterialTheme.typography.labelSmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    modifier = Modifier.weight(1f),
                                    maxLines = 1,
                                )
                                Text(
                                    protocolLabel(item.protocol),
                                    style = MaterialTheme.typography.labelSmall.copy(fontFamily = com.digitrob.digitdns.ui.theme.PanelMono),
                                    color = protocolColor(item.protocol),
                                )
                            }
                        }
                    }
                    if (state.items.size > 8) {
                        Text(
                            "Showing first 8 of ${state.items.size}",
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }
        }
    }
}
