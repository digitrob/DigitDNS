package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material3.Icon
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.domain.model.DiagnosticReport
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelRowDefaults
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.SwipeToDeleteRow
import com.digitrob.digitdns.ui.formatLatency
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono

/**
 * Reachability diagnostics: a target list plus an on-demand run. The run probes every
 * target once and grades the result; the report is shown inline with per-target
 * latency and pass/fail.
 */
@Composable
fun DiagnosticsScreen(viewModel: DiagnosticsViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()

    PanelRefreshBox(
        isRefreshing = state.isLoading,
        onRefresh = viewModel::load,
        modifier = Modifier.fillMaxSize(),
    ) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        state.error?.let {
            Text(it, color = DigitDnsTheme.colors.danger, style = MaterialTheme.typography.bodySmall)
        }

        ReportCard(report = state.report, isRunning = state.isRunning, onRun = viewModel::run)

        DiagnosticsBody(state, viewModel)
    }
    }
}

@Composable
private fun DiagnosticsBody(state: DiagnosticsUiState, viewModel: DiagnosticsViewModel) {
    val latencies = state.report?.results?.associateBy { it.name }.orEmpty()
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("Servers", style = MaterialTheme.typography.titleMedium)
            if (state.isLoading) {
                CircularProgressIndicator(color = MaterialTheme.colorScheme.primary)
            } else if (state.targets.isEmpty()) {
                Text(
                    text = "No servers yet.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            } else {
                state.targets.forEach { target ->
                    SwipeToDeleteRow(onDelete = { viewModel.deleteTarget(target.host) }) {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .heightIn(min = PanelRowDefaults.height)
                                .background(MaterialTheme.colorScheme.surface)
                                .padding(horizontal = 4.dp, vertical = 8.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Text(
                                text = target.name,
                                style = MaterialTheme.typography.bodyMedium,
                                modifier = Modifier.weight(1f),
                            )
                            val result = latencies[target.name]
                            Text(
                                text = when {
                                    result == null -> ""
                                    result.success -> formatLatency(result.latencyMs)
                                    else -> "unreachable"
                                },
                                style = MaterialTheme.typography.bodyMedium.copy(fontFamily = PanelMono),
                                color = when {
                                    result == null -> MaterialTheme.colorScheme.onSurfaceVariant
                                    result.success -> DigitDnsTheme.colors.success
                                    else -> DigitDnsTheme.colors.danger
                                },
                            )
                        }
                    }
                }
            }
        }
    }

    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("Add a server", style = MaterialTheme.typography.titleMedium)
            OutlinedTextField(
                value = state.newName,
                onValueChange = viewModel::onNewNameChange,
                label = { Text("Name") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = state.newHost,
                onValueChange = viewModel::onNewHostChange,
                label = { Text("Host") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            Button(
                onClick = viewModel::addTarget,
                enabled = !state.adding && state.newName.isNotBlank() && state.newHost.isNotBlank(),
            ) { Text("Add") }
        }
    }
}

/**
 * The diagnostic summary, with the Run action folded into its header row so the screen
 * does not spend a whole row on a single button. Before the first run there is no
 * report, so the card shows a neutral placeholder — keeping Run visible without a
 * second layout for the "never run" case.
 */
@Composable
private fun ReportCard(
    report: DiagnosticReport?,
    isRunning: Boolean,
    onRun: () -> Unit,
) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.weight(1f)) {
                    Text(
                        text = report?.overallQuality?.ifBlank { "Result" } ?: "Ready to test",
                        style = MaterialTheme.typography.titleMedium,
                    )
                    if (report != null) {
                        Spacer(Modifier.width(8.dp))
                        Text(
                            text = "${report.overallScore}%",
                            style = MaterialTheme.typography.headlineSmall,
                            color = DigitDnsTheme.colors.success,
                        )
                    }
                }
                // Fixed-height slot so the Run button and its spinner take the same space
                // and the card does not jump while a run is in flight.
                Box(
                    modifier = Modifier.height(40.dp),
                    contentAlignment = Alignment.Center,
                ) {
                    if (isRunning) {
                        CircularProgressIndicator(
                            modifier = Modifier.size(20.dp),
                            strokeWidth = 2.dp,
                            color = MaterialTheme.colorScheme.primary,
                        )
                    } else {
                        TextButton(onClick = onRun) {
                            Icon(Icons.Filled.PlayArrow, contentDescription = null, modifier = Modifier.size(16.dp))
                            Spacer(Modifier.width(4.dp))
                            Text("Run")
                        }
                    }
                }
            }
            if (report != null) {
                Text(
                    text = "${report.reachable}/${report.total} reachable · ${formatLatency(report.avgLatencyMs)} avg",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            } else {
                Text(
                    text = "Run the test to measure reachability and latency.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}
