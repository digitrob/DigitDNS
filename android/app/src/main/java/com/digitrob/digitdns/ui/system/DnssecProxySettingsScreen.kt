package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
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
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.R
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelDialog
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.PanelToggle
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono

/**
 * The DNSSEC/Proxy Leak page. Both policies are server-wide and apply to the
 * running resolver; the DNSSEC save also applies the validator before the
 * response returns. The two cards are deliberately compact so the phone shows
 * both without scrolling.
 */
@Composable
fun DnssecProxySettingsScreen(
    viewModel: DnssecSettingsViewModel = hiltViewModel(),
    proxyViewModel: ProxyLeakSettingsViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val proxyState by proxyViewModel.state.collectAsStateWithLifecycle()

    PanelRefreshBox(
        // Do not run the pull-to-refresh arc for the first paint; the cards
        // already show their own loading state and the arc made the screen look
        // like a stuck spinner for the whole request.
        isRefreshing = state.isRefreshing || proxyState.isRefreshing,
        onRefresh = {
            viewModel.load()
            proxyViewModel.load()
        },
        modifier = Modifier.fillMaxSize(),
    ) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 12.dp, vertical = 8.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            when {
                state.unsupported -> UnsupportedDnssecSettings()
                !state.loaded && state.error != null -> DnssecLoadingCard(state, viewModel::load)
                else -> DnssecPolicyCard(state, viewModel)
            }
            ProxyLeakPolicyCard(proxyState, proxyViewModel)
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
    PanelCard(
        modifier = Modifier.fillMaxWidth(),
        contentPadding = 12.dp,
        verticalPadding = 12.dp,
        bottomPadding = 12.dp,
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(
                text = stringResource(R.string.dnssec_settings_title),
                style = MaterialTheme.typography.titleMedium,
            )
            if (state.error == null) {
                Text(
                    text = stringResource(R.string.dnssec_settings_loading),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            } else {
                Text(
                    text = state.error,
                    style = MaterialTheme.typography.bodySmall,
                    color = DigitDnsTheme.colors.danger,
                )
                TextButton(
                    onClick = onRetry,
                    contentPadding = PaddingValues(0.dp),
                    modifier = Modifier.height(30.dp),
                ) {
                    Text(stringResource(R.string.action_retry))
                }
            }
        }
    }
}

@Composable
private fun UnsupportedDnssecSettings() {
    PanelCard(
        modifier = Modifier.fillMaxWidth(),
        contentPadding = 12.dp,
        verticalPadding = 12.dp,
        bottomPadding = 12.dp,
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(
                text = stringResource(R.string.dnssec_settings_title),
                style = MaterialTheme.typography.titleMedium,
            )
            Text(
                text = stringResource(R.string.dnssec_settings_unsupported),
                style = MaterialTheme.typography.bodySmall,
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
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            DnssecHeader(state.running, state.loaded)
            DnssecSwitch(state, viewModel)
            DnssecAnchors(state, viewModel)
            DnssecFooter(state, viewModel::save)
        }
    }
}

@Composable
private fun DnssecHeader(running: Boolean, loaded: Boolean) {
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
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
        }
        Spacer(Modifier.width(8.dp))
        if (loaded) {
            RunningBadge(running)
        } else {
            StatusBadge(
                text = stringResource(R.string.dnssec_settings_loading_short),
                color = DigitDnsTheme.colors.textMuted,
            )
        }
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
        PanelToggle(
            checked = state.enabled,
            onCheckedChange = viewModel::onEnabledChange,
            enabled = state.loaded && !state.isSaving,
        )
    }
}

@Composable
private fun DnssecAnchors(state: DnssecSettingsUiState, viewModel: DnssecSettingsViewModel) {
    var showDefaults by rememberSaveable { mutableStateOf(false) }
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = stringResource(R.string.dnssec_settings_anchors),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.weight(1f),
        )
        TextButton(
            onClick = { showDefaults = !showDefaults },
            contentPadding = PaddingValues(horizontal = 6.dp),
            modifier = Modifier.height(30.dp),
        ) {
            Text(
                text = stringResource(R.string.dnssec_settings_builtin),
                style = MaterialTheme.typography.labelSmall,
            )
        }
    }
    OutlinedTextField(
        value = state.anchors,
        onValueChange = viewModel::onAnchorsChange,
        label = { Text(stringResource(R.string.dnssec_settings_anchors)) },
        supportingText = { Text(stringResource(R.string.dnssec_settings_anchors_help)) },
        minLines = 2,
        maxLines = 3,
        textStyle = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
        enabled = state.loaded && !state.isSaving,
        modifier = Modifier.fillMaxWidth(),
    )
    if (showDefaults) {
        Surface(
            color = MaterialTheme.colorScheme.surfaceVariant,
            shape = MaterialTheme.shapes.small,
            border = BorderStroke(1.dp, DigitDnsTheme.colors.borderColor),
            modifier = Modifier
                .fillMaxWidth()
                .heightIn(max = 84.dp),
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
                    modifier = Modifier
                        .verticalScroll(rememberScrollState())
                        .padding(horizontal = 8.dp, vertical = 6.dp),
                )
            }
        }
    }
}

@Composable
private fun DnssecFooter(state: DnssecSettingsUiState, onSave: () -> Unit) {
    if (!state.loaded) {
        Text(
            text = stringResource(R.string.dnssec_settings_loading),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    } else {
        Text(
            text = stringResource(
                if (state.restartRequired) R.string.dnssec_settings_restart_pending else R.string.dnssec_settings_restart_note,
            ),
            style = MaterialTheme.typography.bodySmall,
            color = if (state.restartRequired) DigitDnsTheme.colors.warning else MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
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
        // The button is only composed once the first load has finished, so a
        // pull-to-refresh must not disable it: that grey flash was the bug.
        enabled = state.loaded && !state.isSaving,
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

@Composable
private fun ProxyLeakPolicyCard(
    state: ProxyLeakSettingsUiState,
    viewModel: ProxyLeakSettingsViewModel,
) {
    var confirmDisable by rememberSaveable { mutableStateOf(false) }
    if (confirmDisable) {
        PanelDialog(
            onDismissRequest = { confirmDisable = false },
            title = { Text(stringResource(R.string.proxy_leak_settings_disable_title)) },
            text = { Text(stringResource(R.string.proxy_leak_settings_disable_message)) },
            confirmButton = {
                TextButton(
                    onClick = {
                        confirmDisable = false
                        viewModel.setEnabled(false)
                    },
                ) {
                    Text(
                        text = stringResource(R.string.proxy_leak_settings_disable_confirm),
                        color = DigitDnsTheme.colors.danger,
                    )
                }
            },
            dismissButton = {
                TextButton(onClick = { confirmDisable = false }) {
                    Text(stringResource(R.string.action_cancel))
                }
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
            ProxyLeakHeader(state) { want ->
                if (!want && state.enabled) confirmDisable = true else viewModel.setEnabled(want)
            }
            when {
                state.unsupported -> Text(
                    text = stringResource(R.string.proxy_leak_settings_unsupported),
                    style = MaterialTheme.typography.bodySmall,
                    color = DigitDnsTheme.colors.warning,
                )
                !state.loaded -> ProxyLeakLoading(state, viewModel::load)
                else -> {
                    ProxyLeakFacts()
                    ProxyLeakNote(state)
                    state.error?.let { error ->
                        Text(
                            text = error,
                            style = MaterialTheme.typography.bodySmall,
                            color = DigitDnsTheme.colors.danger,
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun ProxyLeakHeader(
    state: ProxyLeakSettingsUiState,
    onCheckedChange: (Boolean) -> Unit,
) {
    Column(
        modifier = Modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Text(
            text = stringResource(R.string.proxy_leak_settings_title),
            style = MaterialTheme.typography.titleMedium,
            color = MaterialTheme.colorScheme.onSurface,
        )
        Text(
            text = stringResource(R.string.proxy_leak_settings_help),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 3,
            overflow = TextOverflow.Ellipsis,
        )
        if (!state.unsupported) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.End,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                StatusBadge(
                    text = when {
                        !state.loaded -> stringResource(R.string.proxy_leak_settings_loading_short)
                        state.enabled -> stringResource(R.string.proxy_leak_settings_status_on)
                        else -> stringResource(R.string.proxy_leak_settings_status_off)
                    },
                    color = if (state.loaded && state.enabled) {
                        DigitDnsTheme.colors.success
                    } else {
                        DigitDnsTheme.colors.textMuted
                    },
                )
                Spacer(Modifier.width(8.dp))
                val switchLabel = stringResource(R.string.proxy_leak_settings_enabled)
                PanelToggle(
                    checked = state.enabled,
                    onCheckedChange = onCheckedChange,
                    enabled = state.loaded && !state.isSaving,
                    modifier = Modifier.semantics { contentDescription = switchLabel },
                )
            }
        }
    }
}

@Composable
private fun ProxyLeakLoading(state: ProxyLeakSettingsUiState, onRetry: () -> Unit) {
    if (state.error == null) {
        Text(
            text = stringResource(R.string.proxy_leak_settings_loading),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    } else {
        Text(
            text = state.error,
            style = MaterialTheme.typography.bodySmall,
            color = DigitDnsTheme.colors.danger,
        )
        TextButton(
            onClick = onRetry,
            contentPadding = PaddingValues(0.dp),
            modifier = Modifier.height(30.dp),
        ) {
            Text(stringResource(R.string.action_retry))
        }
    }
}

@Composable
private fun ProxyLeakFacts() {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        ProxyFact(
            label = stringResource(R.string.proxy_leak_settings_https),
            value = stringResource(R.string.proxy_leak_settings_https_value),
            modifier = Modifier.weight(1f),
        )
        ProxyFact(
            label = stringResource(R.string.proxy_leak_settings_h3),
            value = stringResource(R.string.proxy_leak_settings_h3_value),
            modifier = Modifier.weight(1f),
        )
        ProxyFact(
            label = stringResource(R.string.proxy_leak_settings_hints),
            value = stringResource(R.string.proxy_leak_settings_hints_value),
            modifier = Modifier.weight(1f),
        )
    }
}

@Composable
private fun RowScope.ProxyFact(label: String, value: String, modifier: Modifier = Modifier) {
    Surface(
        modifier = modifier,
        shape = RoundedCornerShape(6.dp),
        color = MaterialTheme.colorScheme.surfaceVariant,
        border = BorderStroke(1.dp, DigitDnsTheme.colors.borderColor),
    ) {
        Column(modifier = Modifier.padding(horizontal = 6.dp, vertical = 4.dp)) {
            Text(
                text = label.uppercase(),
                style = MaterialTheme.typography.labelSmall.copy(fontSize = 9.sp),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                text = value,
                style = MaterialTheme.typography.bodySmall.copy(fontSize = 10.sp, fontFamily = PanelMono),
                color = MaterialTheme.colorScheme.primary,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}

@Composable
private fun ProxyLeakNote(state: ProxyLeakSettingsUiState) {
    val warning = !state.enabled
    val color = if (warning) DigitDnsTheme.colors.danger else DigitDnsTheme.colors.warning
    Surface(
        color = color.copy(alpha = 0.12f),
        shape = RoundedCornerShape(6.dp),
        border = BorderStroke(1.dp, color.copy(alpha = 0.35f)),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Text(
            text = stringResource(
                if (warning) R.string.proxy_leak_settings_off_note else R.string.proxy_leak_settings_warning,
            ),
            style = MaterialTheme.typography.bodySmall,
            color = color,
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 6.dp),
        )
    }
}
