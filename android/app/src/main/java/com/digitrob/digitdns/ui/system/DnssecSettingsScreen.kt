package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.BorderStroke
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
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
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
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.R
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono

/**
 * The server-wide DNSSEC policy. Like Traffic History it is startup-scoped:
 * the daemon installs its validator before listeners start, so the screen saves
 * the record and tells the operator when a restart is still pending.
 */
@Composable
fun DnssecSettingsScreen(viewModel: DnssecSettingsViewModel = hiltViewModel()) {
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
                state.unsupported -> UnsupportedDnssecSettings()
                !state.loaded -> DnssecLoadingCard(state, viewModel::load)
                else -> DnssecPolicyCard(state, viewModel)
            }
        }
    }
}

/**
 * First-paint state. The card is deliberately not drawn with the state defaults:
 * doing so made the switch flip and the badge change from OFF to RUNNING when the
 * server's saved policy arrived. A short loading card appears once, then the real
 * card is composed with its final values.
 */
@Composable
private fun DnssecLoadingCard(state: DnssecSettingsUiState, onRetry: () -> Unit) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(
                text = stringResource(R.string.dnssec_settings_title),
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
                        text = stringResource(R.string.dnssec_settings_loading),
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
private fun UnsupportedDnssecSettings() {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(
                text = stringResource(R.string.dnssec_settings_title),
                style = MaterialTheme.typography.titleMedium,
            )
            Text(
                text = stringResource(R.string.dnssec_settings_unsupported),
                style = MaterialTheme.typography.bodyMedium,
                color = DigitDnsTheme.colors.warning,
            )
        }
    }
}

@Composable
private fun DnssecPolicyCard(
    state: DnssecSettingsUiState,
    viewModel: DnssecSettingsViewModel,
) {
    PanelCard(
        modifier = Modifier.fillMaxWidth(),
        contentPadding = 12.dp,
        verticalPadding = 12.dp,
        bottomPadding = 12.dp,
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            DnssecHeader(state.running)
            DnssecSwitch(state, viewModel)
            DnssecAnchors(state, viewModel)
            DnssecFooter(state, viewModel::save)
        }
    }
}

@Composable
private fun DnssecHeader(running: Boolean) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = stringResource(R.string.dnssec_settings_title),
                style = MaterialTheme.typography.titleMedium,
                color = MaterialTheme.colorScheme.onSurface,
            )
            Text(
                text = stringResource(R.string.dnssec_settings_help),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        Spacer(Modifier.width(8.dp))
        RunningBadge(running)
    }
}

@Composable
private fun RunningBadge(running: Boolean) {
    StatusBadge(
        text = stringResource(
            if (running) R.string.dnssec_settings_status_running else R.string.dnssec_settings_status_off,
        ),
        color = if (running) DigitDnsTheme.colors.success else DigitDnsTheme.colors.textMuted,
    )
}

@Composable
private fun DnssecSwitch(state: DnssecSettingsUiState, viewModel: DnssecSettingsViewModel) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = stringResource(R.string.dnssec_settings_enabled),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurface,
            modifier = Modifier.weight(1f),
        )
        Switch(
            checked = state.enabled,
            onCheckedChange = viewModel::onEnabledChange,
        )
    }
}

@Composable
private fun DnssecAnchors(state: DnssecSettingsUiState, viewModel: DnssecSettingsViewModel) {
    var showDefaults by rememberSaveable { mutableStateOf(false) }
    OutlinedTextField(
        value = state.anchors,
        onValueChange = viewModel::onAnchorsChange,
        label = { Text(stringResource(R.string.dnssec_settings_anchors)) },
        supportingText = { Text(stringResource(R.string.dnssec_settings_anchors_help)) },
        minLines = 2,
        maxLines = 4,
        textStyle = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
        modifier = Modifier.fillMaxWidth(),
    )
    TextButton(
        onClick = { showDefaults = !showDefaults },
        contentPadding = PaddingValues(0.dp),
        modifier = Modifier.height(32.dp),
    ) {
        Text(
            stringResource(
                if (showDefaults) R.string.dnssec_settings_defaults_hide else R.string.dnssec_settings_defaults_show,
            ),
        )
    }
    if (showDefaults) {
        Surface(
            color = MaterialTheme.colorScheme.surfaceVariant,
            shape = MaterialTheme.shapes.small,
            border = BorderStroke(1.dp, DigitDnsTheme.colors.borderColor),
            modifier = Modifier.fillMaxWidth(),
        ) {
            SelectionContainer {
                Text(
                    text = state.defaultAnchors.joinToString("\n"),
                    style = MaterialTheme.typography.bodySmall.copy(
                        fontFamily = PanelMono,
                        fontSize = 9.sp,
                        lineHeight = 12.sp,
                    ),
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(horizontal = 8.dp, vertical = 6.dp),
                )
            }
        }
    }
}

@Composable
private fun DnssecFooter(state: DnssecSettingsUiState, onSave: () -> Unit) {
    Text(
        text = stringResource(
            if (state.restartRequired) R.string.dnssec_settings_restart_pending else R.string.dnssec_settings_restart_note,
        ),
        style = MaterialTheme.typography.bodySmall,
        color = if (state.restartRequired) DigitDnsTheme.colors.warning else MaterialTheme.colorScheme.onSurfaceVariant,
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
            text = stringResource(
                if (state.restartRequired) R.string.dnssec_settings_saved_restart else R.string.dnssec_settings_saved,
            ),
            style = MaterialTheme.typography.bodySmall,
            color = DigitDnsTheme.colors.success,
        )
    }
    Button(
        onClick = onSave,
        enabled = !state.isSaving && !state.isLoading,
        modifier = Modifier.fillMaxWidth(),
    ) {
        if (state.isSaving) {
            CircularProgressIndicator(
                modifier = Modifier.padding(end = 8.dp).height(18.dp).width(18.dp),
                strokeWidth = 2.dp,
                color = MaterialTheme.colorScheme.onPrimary,
            )
        }
        Text(stringResource(R.string.dnssec_settings_save))
    }
}
