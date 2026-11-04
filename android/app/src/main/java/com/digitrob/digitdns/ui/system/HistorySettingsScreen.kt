package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.R
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

/**
 * The aggregate-history policy: whether the server keeps history, for how long,
 * how much top-N it retains, and which clients must never be identified. The
 * server opens its store once at boot, so saving deliberately ends in a
 * restart-required notice rather than pretending the running daemon changed.
 */
@Composable
fun HistorySettingsScreen(viewModel: HistorySettingsViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        if (state.unsupported) {
            UnsupportedHistorySettings()
        } else {
            HistoryPolicyCard(state, viewModel)
        }
    }
}

@Composable
private fun UnsupportedHistorySettings() {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(
                text = stringResource(R.string.history_settings_title),
                style = MaterialTheme.typography.titleMedium,
            )
            Text(
                text = stringResource(R.string.history_settings_unsupported),
                style = MaterialTheme.typography.bodyMedium,
                color = DigitDnsTheme.colors.warning,
            )
        }
    }
}

@Composable
private fun HistoryPolicyCard(
    state: HistorySettingsUiState,
    viewModel: HistorySettingsViewModel,
) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            HistoryPolicyHeader(state)
            HistoryPolicyFields(state, viewModel)
            HistoryPolicyFooter(state, viewModel::save)
        }
    }
}

@Composable
private fun HistoryPolicyHeader(state: HistorySettingsUiState) {
    Text(
        text = stringResource(R.string.history_settings_title),
        style = MaterialTheme.typography.titleMedium,
        color = MaterialTheme.colorScheme.onSurface,
    )
    Text(
        text = stringResource(R.string.history_settings_help),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    if (state.isLoading && !state.isSaving) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            CircularProgressIndicator(modifier = Modifier.height(18.dp).width(18.dp), strokeWidth = 2.dp)
            Spacer(Modifier.width(8.dp))
            Text(
                text = stringResource(R.string.history_settings_loading),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun HistoryPolicyFields(
    state: HistorySettingsUiState,
    viewModel: HistorySettingsViewModel,
) {
    SettingSwitch(
        label = stringResource(R.string.history_settings_enabled),
        checked = state.enabled,
        onCheckedChange = viewModel::onEnabledChange,
    )
    SettingSwitch(
        label = stringResource(R.string.history_settings_anonymize),
        checked = state.anonymizeClients,
        onCheckedChange = viewModel::onAnonymizeChange,
    )
    OutlinedTextField(
        value = state.retentionDays,
        onValueChange = viewModel::onRetentionChange,
        label = { Text(stringResource(R.string.history_settings_retention)) },
        singleLine = true,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
        modifier = Modifier.fillMaxWidth(),
    )
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        OutlinedTextField(
            value = state.topDomains,
            onValueChange = viewModel::onTopDomainsChange,
            label = { Text(stringResource(R.string.history_settings_top_domains)) },
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
            modifier = Modifier.weight(1f),
        )
        OutlinedTextField(
            value = state.topClients,
            onValueChange = viewModel::onTopClientsChange,
            label = { Text(stringResource(R.string.history_settings_top_clients)) },
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
            modifier = Modifier.weight(1f),
        )
    }
    OutlinedTextField(
        value = state.excludeClients,
        onValueChange = viewModel::onExcludeClientsChange,
        label = { Text(stringResource(R.string.history_settings_exclude)) },
        minLines = 3,
        maxLines = 6,
        modifier = Modifier.fillMaxWidth(),
    )
    Text(
        text = stringResource(R.string.history_settings_exclude_help),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

@Composable
private fun HistoryPolicyFooter(state: HistorySettingsUiState, onSave: () -> Unit) {
    Text(
        text = stringResource(R.string.history_settings_restart_note),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    state.error?.let { error ->
        Text(
            text = error,
            style = MaterialTheme.typography.bodySmall,
            color = DigitDnsTheme.colors.danger,
        )
    }
    if (state.saved) {
        Text(
            text = stringResource(R.string.history_settings_saved),
            style = MaterialTheme.typography.bodySmall,
            color = DigitDnsTheme.colors.success,
        )
    }
    Button(
        onClick = onSave,
        enabled = !state.isSaving && !state.isLoading,
        modifier = Modifier.fillMaxWidth(),
    ) {
        Text(stringResource(R.string.history_settings_save))
    }
}

@Composable
private fun SettingSwitch(
    label: String,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = label,
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurface,
            modifier = Modifier.weight(1f),
        )
        Switch(checked = checked, onCheckedChange = onCheckedChange)
    }
}
