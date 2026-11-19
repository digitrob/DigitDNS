package com.digitrob.digitdns.ui.rules

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.core.network.MigrateApplyResultDto
import com.digitrob.digitdns.core.network.MigrateIssueDto
import com.digitrob.digitdns.core.network.MigratePreviewDto
import com.digitrob.digitdns.core.network.MigrateStatsDto
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelScreenHeader
import com.digitrob.digitdns.ui.components.PanelSectionTitle
import com.digitrob.digitdns.ui.components.PanelSnackbarHost
import com.digitrob.digitdns.ui.components.PanelSwitch
import com.digitrob.digitdns.ui.components.showPanelToast
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

/**
 * The Android importer, mirroring the web panel's two-step flow: analyze a
 * Pi-hole or AdGuard Home export, review the normalized counts and skipped
 * items, then apply. A v6 gravity.db is read locally; nothing from the source is
 * applied until the operator presses Apply.
 */
@Suppress("LongMethod", "CyclomaticComplexMethod")
@Composable
fun MigrateScreen(viewModel: MigrateViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }
    val picker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri != null) {
            viewModel.analyze(uri, uri.lastPathSegment?.substringAfterLast('/') ?: "import")
        }
    }
    val reportSaver = rememberLauncherForActivityResult(
        ActivityResultContracts.CreateDocument("text/plain"),
    ) { uri ->
        if (uri != null) viewModel.saveReport(uri)
    }

    LaunchedEffect(state.message) {
        val message = state.message ?: return@LaunchedEffect
        snackbar.showPanelToast(message)
        viewModel.dismissMessage()
    }

    Scaffold(snackbarHost = { PanelSnackbarHost(snackbar) }) { padding ->
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            PanelScreenHeader(showBadge = false)
            LazyColumn(
                contentPadding = PaddingValues(horizontal = 12.dp, vertical = 4.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                item {
                    Text(
                        text = "Import from Pi-hole / AdGuard Home",
                        style = MaterialTheme.typography.titleMedium,
                    )
                    Text(
                        text = "Analyze first, then apply. Rules, clients and lists are added; " +
                            "passwords, API keys, TLS keys, DHCP state and query history are never imported.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }

                item {
                    PanelCard(modifier = Modifier.fillMaxWidth()) {
                        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            OutlinedButton(
                                onClick = {
                                    picker.launch(
                                        arrayOf(
                                            "application/zip",
                                            "application/x-yaml",
                                            "text/yaml",
                                            "text/plain",
                                            "application/octet-stream",
                                        ),
                                    )
                                },
                                enabled = !state.analyzing,
                            ) {
                                Text(if (state.analyzing) "Analyzing…" else "Choose export")
                            }
                            if (state.fileName.isNotBlank()) {
                                Text(
                                    text = state.fileName,
                                    style = MaterialTheme.typography.bodySmall,
                                    fontFamily = com.digitrob.digitdns.ui.theme.PanelMono,
                                )
                            }
                        }
                    }
                }

                state.error?.let { error ->
                    item {
                        Text(
                            text = error,
                            color = DigitDnsTheme.colors.danger,
                            style = MaterialTheme.typography.bodySmall,
                        )
                    }
                }

                state.preview?.let { preview ->
                    item { PanelSectionTitle("Preview") }
                    item { PreviewCard(preview) }
                    if (preview.stats.upstreams > 0) {
                        item {
                            PanelCard(modifier = Modifier.fillMaxWidth()) {
                                PanelSwitch(
                                    label = "Import upstream resolvers",
                                    checked = state.upstreamsOptIn,
                                    onCheckedChange = viewModel::setUpstreamsOptIn,
                                    supporting = "Off by default; turning this on adds the source's " +
                                        "resolver addresses to this server's pool.",
                                )
                            }
                        }
                    }
                    if (preview.warnings.isNotEmpty() || preview.skipped.isNotEmpty()) {
                        item { IssuesCard("Warnings and skipped items", preview.warnings + preview.skipped) }
                    }
                    if (preview.reportAvailable) {
                        item {
                            OutlinedButton(
                                onClick = { reportSaver.launch("digitdns-import-skipped-regex.txt") },
                                enabled = !state.savingReport && !state.applying,
                                modifier = Modifier.fillMaxWidth(),
                            ) {
                                Text(if (state.savingReport) "Saving report…" else "Save skipped report")
                            }
                        }
                    }
                    item {
                        Button(
                            onClick = viewModel::apply,
                            enabled = !state.applying,
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Text(if (state.applying) "Applying…" else "Apply import")
                        }
                    }
                }

                state.result?.let { result ->
                    item { PanelSectionTitle("Result") }
                    item { ResultCard(result, state.undoToken.isNotEmpty(), state.undoing, viewModel::undo) }
                }

                item {
                    OutlinedButton(
                        onClick = viewModel::reset,
                        enabled = !state.analyzing && !state.applying && !state.undoing && !state.savingReport,
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text("Start over")
                    }
                }
            }
        }
    }
}

@Composable
private fun PreviewCard(preview: MigratePreviewDto) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(
                text = preview.source.fileName.ifBlank { preview.source.kind.ifBlank { "Import" } },
                style = MaterialTheme.typography.bodyMedium,
                fontWeight = FontWeight.SemiBold,
            )
            StatsRows(preview.stats)
            val customRules = preview.filtering?.customRules.orEmpty().lineSequence().count { it.isNotBlank() }
            if (customRules > 0) StatRow("Custom rules", customRules)
            if (preview.sample.blocked.isNotEmpty()) SampleLine("Blocked", preview.sample.blocked)
            if (preview.sample.allowlist.isNotEmpty()) SampleLine("Allowlist", preview.sample.allowlist)
            if (preview.sample.clients.isNotEmpty()) SampleLine("Clients", preview.sample.clients)
            if (preview.sample.filterLists.isNotEmpty()) SampleLine("Lists", preview.sample.filterLists)
            if (preview.sample.records.isNotEmpty()) SampleLine("Records", preview.sample.records)
        }
    }
}

@Composable
private fun ResultCard(
    result: MigrateApplyResultDto,
    canUndo: Boolean,
    undoing: Boolean,
    onUndo: () -> Unit,
) {
    PanelCard(modifier = Modifier.fillMaxWidth(), highlighted = true) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(
                text = if (result.success) "Import applied" else "Import did not report success",
                style = MaterialTheme.typography.bodyMedium,
                fontWeight = FontWeight.SemiBold,
            )
            StatsRows(result.added)
            if (result.skipped.isNotEmpty()) {
                Spacer(Modifier.width(4.dp))
                Text(
                    text = "Skipped: ${result.skipped.size}",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                result.skipped.take(6).forEach { issue ->
                    Text(
                        text = issueLine(issue),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            if (canUndo) {
                OutlinedButton(onClick = onUndo, enabled = !undoing, modifier = Modifier.fillMaxWidth()) {
                    Text(if (undoing) "Undoing…" else "Undo import")
                }
            }
        }
    }
}

@Composable
private fun IssuesCard(title: String, issues: List<MigrateIssueDto>) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(3.dp)) {
            Text(title, style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.SemiBold)
            issues.take(10).forEach { issue ->
                Text(
                    text = issueLine(issue),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (issues.size > 10) {
                Text(
                    text = "…and ${issues.size - 10} more",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun StatsRows(stats: MigrateStatsDto) {
    if (stats.records > 0) StatRow("Records", stats.records)
    if (stats.clients > 0) StatRow("Clients", stats.clients)
    if (stats.proxied > 0) StatRow("Proxied", stats.proxied)
    if (stats.blocked > 0) StatRow("Blocked", stats.blocked)
    if (stats.direct > 0) StatRow("Direct", stats.direct)
    if (stats.allowlist > 0) StatRow("Allowlist", stats.allowlist)
    if (stats.filterLists > 0) StatRow("Filter lists", stats.filterLists)
    if (stats.filterRules > 0) StatRow("Filter rules", stats.filterRules)
    if (stats.rewrites > 0) StatRow("Rewrites", stats.rewrites)
    if (stats.upstreams > 0) StatRow("Upstreams", stats.upstreams)
    if (stats.warnings > 0) StatRow("Warnings", stats.warnings)
    if (stats.skipped > 0) StatRow("Skipped", stats.skipped)
}

@Composable
private fun StatRow(label: String, value: Int) {
    Row(modifier = Modifier.fillMaxWidth()) {
        Text(
            text = label,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.weight(1f),
        )
        Text(
            text = value.toString(),
            style = MaterialTheme.typography.bodySmall,
            fontFamily = com.digitrob.digitdns.ui.theme.PanelMono,
        )
    }
}

@Composable
private fun SampleLine(label: String, values: List<String>) {
    Text(
        text = "$label: ${values.take(3).joinToString(", ")}${if (values.size > 3) "…" else ""}",
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

private fun issueLine(issue: MigrateIssueDto): String {
    val item = issue.item.takeIf { it.isNotBlank() }?.let { ": $it" }.orEmpty()
    val reason = issue.reason.takeIf { it.isNotBlank() }?.let { " — $it" }.orEmpty()
    return "${issue.code}$item$reason"
}
