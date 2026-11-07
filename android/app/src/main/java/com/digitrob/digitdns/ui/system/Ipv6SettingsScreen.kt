package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.R
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

/**
 * The live proxy-IPv6 policy. Off is the hard leak state: proxied AAAA and
 * HTTPS/SVCB hints are withheld, and agents switch to IPv4-only listeners on
 * their next heartbeat.
 */
@Composable
fun Ipv6SettingsScreen(viewModel: Ipv6SettingsViewModel = hiltViewModel()) {
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
                .padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            when {
                state.unsupported -> UnsupportedIpv6Card()
                !state.loaded -> Ipv6LoadingCard(state, viewModel::load)
                else -> Ipv6PolicyCard(state, viewModel)
            }
        }
    }
}

@Composable
private fun Ipv6LoadingCard(state: Ipv6SettingsUiState, onRetry: () -> Unit) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(
                text = stringResource(R.string.ipv6_settings_title),
                style = MaterialTheme.typography.titleMedium,
            )
            if (state.error == null) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    CircularProgressIndicator(
                        modifier = Modifier.height(18.dp).width(18.dp),
                        strokeWidth = 2.dp,
                    )
                    Spacer(Modifier.width(8.dp))
                    Text(
                        text = stringResource(R.string.ipv6_settings_loading),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            } else {
                Text(
                    text = state.error,
                    style = MaterialTheme.typography.bodySmall,
                    color = DigitDnsTheme.colors.danger,
                )
                TextButton(
                    onClick = onRetry,
                    contentPadding = PaddingValues(0.dp),
                    modifier = Modifier.height(32.dp),
                ) {
                    Text(stringResource(R.string.action_retry))
                }
            }
        }
    }
}

@Composable
private fun UnsupportedIpv6Card() {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(
                text = stringResource(R.string.ipv6_settings_title),
                style = MaterialTheme.typography.titleMedium,
            )
            Text(
                text = stringResource(R.string.ipv6_settings_unsupported),
                style = MaterialTheme.typography.bodyMedium,
                color = DigitDnsTheme.colors.warning,
            )
        }
    }
}

@Composable
private fun Ipv6PolicyCard(state: Ipv6SettingsUiState, viewModel: Ipv6SettingsViewModel) {
    var confirmDisable by rememberSaveable { mutableStateOf(false) }
    if (confirmDisable) {
        Ipv6DisableDialog(
            onDismiss = { confirmDisable = false },
            onConfirm = {
                confirmDisable = false
                viewModel.onEnabledChange(false)
            },
        )
    }

    PanelCard(
        modifier = Modifier.fillMaxWidth(),
        contentPadding = 12.dp,
        verticalPadding = 12.dp,
        bottomPadding = 12.dp,
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Ipv6Header(state)
            Ipv6SwitchRow(state) { want ->
                if (!want && state.enabled) confirmDisable = true else viewModel.onEnabledChange(want)
            }
            Ipv6Address(state, viewModel)
            Ipv6Note(state)
            Ipv6Status(state)
            Ipv6SaveButton(state, viewModel::save)
        }
    }
}

@Composable
private fun Ipv6DisableDialog(onDismiss: () -> Unit, onConfirm: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.ipv6_settings_disable_title)) },
        text = { Text(stringResource(R.string.ipv6_settings_disable_message)) },
        confirmButton = {
            TextButton(onClick = onConfirm) {
                Text(
                    text = stringResource(R.string.ipv6_settings_disable_confirm),
                    color = DigitDnsTheme.colors.danger,
                    fontWeight = FontWeight.Bold,
                )
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) {
                Text(stringResource(R.string.action_cancel))
            }
        },
    )
}

@Composable
private fun Ipv6Header(state: Ipv6SettingsUiState) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = stringResource(R.string.ipv6_settings_title),
                style = MaterialTheme.typography.titleMedium,
                color = MaterialTheme.colorScheme.onSurface,
            )
            Text(
                text = stringResource(R.string.ipv6_settings_help),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        Spacer(Modifier.width(8.dp))
        StatusBadge(
            text = stringResource(
                if (state.enabled) R.string.ipv6_settings_status_on else R.string.ipv6_settings_status_off,
            ),
            color = if (state.enabled) DigitDnsTheme.colors.success else DigitDnsTheme.colors.textMuted,
        )
    }
}

@Composable
private fun Ipv6SwitchRow(state: Ipv6SettingsUiState, onCheckedChange: (Boolean) -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = stringResource(R.string.ipv6_settings_enabled),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurface,
            modifier = Modifier.weight(1f),
        )
        Switch(
            checked = state.enabled,
            onCheckedChange = onCheckedChange,
            enabled = !state.isSaving,
        )
    }
}

@Composable
private fun Ipv6Address(state: Ipv6SettingsUiState, viewModel: Ipv6SettingsViewModel) {
    OutlinedTextField(
        value = state.address,
        onValueChange = viewModel::onAddressChange,
        label = { Text(stringResource(R.string.ipv6_settings_address)) },
        supportingText = { Text(stringResource(R.string.ipv6_settings_address_help)) },
        singleLine = true,
        textStyle = MaterialTheme.typography.bodySmall,
        modifier = Modifier.fillMaxWidth(),
    )
}

@Composable
private fun Ipv6Note(state: Ipv6SettingsUiState) {
    val color = if (state.enabled) DigitDnsTheme.colors.success else DigitDnsTheme.colors.warning
    Surface(
        color = color.copy(alpha = if (state.enabled) 0.10f else 0.12f),
        shape = MaterialTheme.shapes.small,
        modifier = Modifier.fillMaxWidth(),
    ) {
        Text(
            text = stringResource(
                if (state.enabled) R.string.ipv6_settings_on_note else R.string.ipv6_settings_off_note,
            ),
            style = MaterialTheme.typography.bodySmall,
            color = color,
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 6.dp),
        )
    }
}

@Composable
private fun Ipv6Status(state: Ipv6SettingsUiState) {
    state.error?.let { error ->
        Text(
            text = error,
            style = MaterialTheme.typography.bodySmall,
            color = DigitDnsTheme.colors.danger,
        )
    }
    if (state.saved) {
        Text(
            text = stringResource(R.string.ipv6_settings_saved),
            style = MaterialTheme.typography.bodySmall,
            color = DigitDnsTheme.colors.success,
        )
    }
}

@Composable
private fun Ipv6SaveButton(state: Ipv6SettingsUiState, onSave: () -> Unit) {
    Button(
        onClick = onSave,
        enabled = !state.isSaving,
        modifier = Modifier.fillMaxWidth(),
    ) {
        if (state.isSaving) {
            CircularProgressIndicator(
                modifier = Modifier.padding(end = 8.dp).height(18.dp).width(18.dp),
                strokeWidth = 2.dp,
                color = MaterialTheme.colorScheme.onPrimary,
            )
        }
        Text(stringResource(R.string.ipv6_settings_save))
    }
}
