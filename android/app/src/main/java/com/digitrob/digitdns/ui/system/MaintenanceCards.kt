package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.Button
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.digitrob.digitdns.data.SystemSummary
import com.digitrob.digitdns.domain.model.UpdateProgressView
import com.digitrob.digitdns.domain.model.UpdateStatus
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

/**
 * The System and Update cards, relocated from the retired Updates page onto About &
 * Legal. They read the same [MaintenanceViewModel], so the version shown here and the
 * update check never disagree.
 */
@Composable
internal fun SystemCard(summary: SystemSummary) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("System", style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
                StatusBadge(
                    text = summary.status.ifBlank { "unknown" },
                    color = healthColor(summary.status),
                )
            }
            InfoLine("Version", summary.display.ifBlank { summary.version })
            InfoLine("Channel", summary.channel)
            InfoLine("Commit", summary.commit)
            InfoLine("Public IP", summary.publicIp)
            InfoLine("API exposure", summary.apiExposure)
        }
    }
}

@Composable
internal fun UpdateCard(state: MaintenanceUiState, viewModel: MaintenanceViewModel) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("Update", style = MaterialTheme.typography.titleMedium)
            state.update?.let { UpdateInfo(it) }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedButton(onClick = viewModel::checkForUpdate, enabled = !state.checking) {
                    Text(if (state.checking) "Checking…" else "Check for updates")
                }
                if (state.update?.canApply == true) {
                    Button(onClick = viewModel::applyUpdate, enabled = !state.applying) {
                        Text(if (state.applying) "Starting…" else "Apply update")
                    }
                }
            }
            state.progress?.let { UpdateProgress(it) }
        }
    }
}

/** The cache-flush card, relocated onto the REST API Gateway page. */
@Composable
internal fun CacheCard(state: MaintenanceUiState, viewModel: MaintenanceViewModel) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("Cache", style = MaterialTheme.typography.titleMedium)
            Text(
                text = "Clears the resolver's cached answers. Clients re-resolve on their next query.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            OutlinedButton(onClick = viewModel::flushCache, enabled = !state.flushing) {
                Text(if (state.flushing) "Flushing…" else "Flush resolver cache")
            }
        }
    }
}

@Composable
internal fun InfoLine(label: String, value: String) {
    Row(modifier = Modifier.fillMaxWidth()) {
        Text(
            text = label,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.weight(1f),
        )
        Text(value.ifBlank { "—" }, style = MaterialTheme.typography.bodySmall)
    }
}

@Composable
private fun UpdateInfo(update: UpdateStatus) {
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        when {
            update.error.isNotBlank() -> Text(
                text = "Could not check: ${update.error}",
                style = MaterialTheme.typography.bodySmall,
                color = DigitDnsTheme.colors.warning,
            )
            update.updateAvailable -> Text(
                text = "Update available: ${update.current} → ${update.latest}",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.primary,
            )
            else -> Text(
                text = "Up to date (${update.current}).",
                style = MaterialTheme.typography.bodySmall,
                color = DigitDnsTheme.colors.success,
            )
        }
        if (update.note.isNotBlank()) {
            Text(update.note, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        if (update.updateAvailable && !update.supported) {
            Text(
                text = "This update cannot be applied by the app on this install.",
                style = MaterialTheme.typography.bodySmall,
                color = DigitDnsTheme.colors.warning,
            )
        }
    }
}

/**
 * The badge colour for the server's health word. The v3 endpoint answers "healthy";
 * older builds answered "ok". Both are the green state, everything else is a warning.
 */
@Composable
private fun healthColor(status: String): androidx.compose.ui.graphics.Color =
    if (status.equals("healthy", ignoreCase = true) || status.equals("ok", ignoreCase = true)) {
        DigitDnsTheme.colors.success
    } else {
        DigitDnsTheme.colors.warning
    }

@Composable
private fun UpdateProgress(progress: UpdateProgressView) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp), modifier = Modifier.fillMaxWidth()) {
        if (progress.running) {
            LinearProgressIndicator(
                progress = { (progress.percent.coerceIn(0, 100)) / 100f },
                modifier = Modifier.fillMaxWidth(),
            )
        }
        Text(
            text = "${progress.phase} · ${progress.percent}%" + if (progress.restarting) " · restarting" else "",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        if (progress.message.isNotBlank()) {
            Text(progress.message, style = MaterialTheme.typography.bodySmall)
        }
        if (progress.error.isNotBlank()) {
            Text(progress.error, style = MaterialTheme.typography.bodySmall, color = DigitDnsTheme.colors.danger)
        }
    }
}
