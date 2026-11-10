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
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.R
import com.digitrob.digitdns.domain.model.DiagnosticReport
import com.digitrob.digitdns.domain.model.DiagnosticResult
import com.digitrob.digitdns.domain.model.DiagnosticTarget
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelRowDefaults
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.SwipeToDeleteRow
import com.digitrob.digitdns.ui.formatLatency
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono
import androidx.compose.foundation.shape.CircleShape

/**
 * Reachability diagnostics: a target list plus an on-demand run. The run probes every
 * target once and grades the result; the report is shown inline with per-target
 * latency and pass/fail.
 */
@Composable
fun DiagnosticsScreen(viewModel: DiagnosticsViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()

    PanelRefreshBox(
        isRefreshing = false,
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
    val latencies = state.report?.results?.associateBy { it.target }.orEmpty()
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("Servers", style = MaterialTheme.typography.titleMedium)
            if (state.isLoading) {
                Text(
                    text = stringResource(R.string.settings_loading),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            } else if (state.targets.isEmpty()) {
                Text(
                    text = stringResource(R.string.diag_no_servers),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            } else {
                state.targets.forEach { target ->
                    SwipeToDeleteRow(
                        onDelete = { viewModel.deleteTarget(target.host) },
                        confirmTitle = "Remove ${target.name}?",
                        confirmText = "It is no longer checked for reachability and latency.",
                        confirmLabel = "Remove",
                    ) {
                        DiagnosticTargetRow(target = target, result = latencies[target.host])
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
// One configured server and the result of the last run against it. Kept as its own
// composable because the row carries a status dot, two lines of text and a result,
// and inlining that pushed DiagnosticsBody past detekt's length and complexity
// limits - which is a fair signal that it was two things.
@Composable
private fun DiagnosticTargetRow(target: DiagnosticTarget, result: DiagnosticResult?) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .heightIn(min = PanelRowDefaults.height)
            .background(MaterialTheme.colorScheme.surface)
            .padding(horizontal = 4.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        // The result belongs to the row it measured, not to a second list below:
        // the same server was printed twice, and matching the two by eye was the
        // only way to read which one failed. Keyed on the host because that is what
        // was measured; two targets sharing a display name would otherwise collide.
        Box(
            modifier = Modifier
                .size(8.dp)
                .background(
                    color = when {
                        result == null -> MaterialTheme.colorScheme.onSurfaceVariant
                        result.success -> DigitDnsTheme.colors.success
                        else -> DigitDnsTheme.colors.danger
                    },
                    shape = CircleShape,
                ),
        )
        Spacer(Modifier.width(8.dp))
        Column(modifier = Modifier.weight(1f)) {
            Text(text = target.name, style = MaterialTheme.typography.bodyMedium)
            Text(
                text = target.host,
                style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        Text(
            text = when {
                result == null -> stringResource(R.string.diag_not_tested)
                result.success -> formatLatency(result.latencyMs)
                else -> stringResource(R.string.diag_unreachable)
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
                            text = stringResource(R.string.diag_score, report.overallScore),
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
                    text = stringResource(
                        R.string.diag_reachable,
                        report.reachable,
                        report.total,
                        formatLatency(report.avgLatencyMs),
                    ),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            } else {
                Text(
                    text = stringResource(R.string.diag_run_hint),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}
