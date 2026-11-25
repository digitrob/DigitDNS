package com.digitrob.digitdns.ui.rules

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.PanelToggle
import com.digitrob.digitdns.ui.components.SettingsLoadingSkeleton
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

/**
 * Connection maintenance and last-resort resolution. Opened from the bottom of
 * the Upstreams settings section; it replaces that section's content and comes
 * back with the back arrow, so the main navigation is untouched.
 */
@Composable
fun AdvancedUpstreamsScreen(
    onBack: () -> Unit,
    viewModel: AdvancedUpstreamsViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()

    Scaffold { padding ->
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            Row(
                modifier = Modifier.fillMaxWidth().padding(horizontal = 4.dp, vertical = 4.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                IconButton(onClick = onBack) {
                    Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back to upstreams")
                }
                Text(
                    text = "Advanced upstream settings",
                    style = MaterialTheme.typography.titleMedium,
                    color = MaterialTheme.colorScheme.onSurface,
                )
            }

            PanelRefreshBox(
                isRefreshing = state.isLoading,
                onRefresh = viewModel::load,
                modifier = Modifier.fillMaxSize(),
            ) {
                if (!state.loaded && state.isLoading) {
                    SettingsLoadingSkeleton(cards = 2)
                } else {
                    Column(
                        modifier = Modifier
                            .fillMaxSize()
                            .verticalScroll(rememberScrollState())
                            .padding(12.dp),
                        verticalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        AddressCard(state, viewModel)
                        MaintenanceCard(state, viewModel)
                        SaveCard(state, viewModel)
                    }
                }
            }
        }
    }
}

@Composable
private fun AddressCard(state: AdvancedUpstreamsUiState, viewModel: AdvancedUpstreamsViewModel) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(
                text = "Bootstrap & fallback",
                style = MaterialTheme.typography.titleSmall,
                color = MaterialTheme.colorScheme.onSurface,
            )
            OutlinedTextField(
                value = state.bootstrap,
                onValueChange = viewModel::onBootstrapChange,
                label = { Text("Bootstrap DNS") },
                supportingText = {
                    Text("Plain DNS used only to resolve DoT/DoH/DoQ hostnames, e.g. 1.1.1.1:53.")
                },
                singleLine = true,
                textStyle = MaterialTheme.typography.bodySmall,
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = state.fallback,
                onValueChange = viewModel::onFallbackChange,
                label = { Text("Fallback DNS") },
                supportingText = { Text("Plain DNS tried only after every primary resolver failed.") },
                singleLine = true,
                textStyle = MaterialTheme.typography.bodySmall,
                modifier = Modifier.fillMaxWidth(),
            )
            Text(
                text = "Changing Bootstrap DNS needs a resolver restart. Everything else applies on save.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun MaintenanceCard(state: AdvancedUpstreamsUiState, viewModel: AdvancedUpstreamsViewModel) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(
                text = "Keep-warm & probation",
                style = MaterialTheme.typography.titleSmall,
                color = MaterialTheme.colorScheme.onSurface,
            )
            Text(
                text = "Keep-warm scope",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Row(
                modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                KeepWarmChoice("off", "Off", state, viewModel)
                KeepWarmChoice("active", "Active", state, viewModel)
                KeepWarmChoice("active_next", "Active + next", state, viewModel)
                KeepWarmChoice("all", "All encrypted", state, viewModel)
            }
            Text(
                text = "Adaptive hedge",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                FilterChip(
                    selected = state.hedge == "balanced",
                    onClick = { viewModel.onHedgeChange("balanced") },
                    label = { Text("Balanced", style = MaterialTheme.typography.bodySmall) },
                    enabled = !state.isSaving,
                )
                FilterChip(
                    selected = state.hedge == "aggressive",
                    onClick = { viewModel.onHedgeChange("aggressive") },
                    label = { Text("Aggressive", style = MaterialTheme.typography.bodySmall) },
                    enabled = !state.isSaving,
                )
            }
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                NumberField(
                    value = state.keepWarmSeconds,
                    onValueChange = viewModel::onKeepWarmSecondsChange,
                    label = "Interval (s)",
                    min = 5,
                    max = 600,
                    modifier = Modifier.weight(1f),
                )
                NumberField(
                    value = state.probationSeconds,
                    onValueChange = viewModel::onProbationChange,
                    label = "Probation (s)",
                    min = 5,
                    max = 300,
                    modifier = Modifier.weight(1f),
                )
            }
            Text(
                text = "A resolver that fails repeatedly is skipped for the probation window before it is probed again.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun KeepWarmChoice(
    value: String,
    label: String,
    state: AdvancedUpstreamsUiState,
    viewModel: AdvancedUpstreamsViewModel,
) {
    FilterChip(
        selected = state.keepWarm == value,
        onClick = { viewModel.onKeepWarmChange(value) },
        label = { Text(label, style = MaterialTheme.typography.bodySmall) },
        enabled = !state.isSaving,
    )
}

@Composable
private fun NumberField(
    value: String,
    onValueChange: (String) -> Unit,
    label: String,
    min: Int,
    max: Int,
    modifier: Modifier = Modifier,
) {
    OutlinedTextField(
        value = value,
        onValueChange = { text ->
            if (text.all { it.isDigit() } && text.length <= 3) onValueChange(text)
        },
        label = { Text(label) },
        supportingText = { Text("$min–$max") },
        singleLine = true,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
        textStyle = MaterialTheme.typography.bodySmall,
        modifier = modifier,
    )
}

@Composable
private fun SaveCard(state: AdvancedUpstreamsUiState, viewModel: AdvancedUpstreamsViewModel) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Column(modifier = Modifier.weight(1f)) {
                    Text(
                        text = "Startup cache warm",
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurface,
                    )
                    Text(
                        text = "Resolve the most popular names from the last 24 h after a restart.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Spacer(Modifier.width(8.dp))
                PanelToggle(
                    checked = state.warmCache,
                    onCheckedChange = viewModel::onWarmCacheChange,
                    enabled = !state.isSaving,
                )
            }
            NumberField(
                value = state.warmCacheTop,
                onValueChange = viewModel::onWarmCacheTopChange,
                label = "Top names",
                min = 1,
                max = 100,
                modifier = Modifier.fillMaxWidth(),
            )
            state.error?.let {
                Text(
                    text = it,
                    style = MaterialTheme.typography.bodySmall,
                    color = DigitDnsTheme.colors.danger,
                )
            }
            if (state.saved) {
                Text(
                    text = if (state.restartRequired) {
                        "Saved · restart required for Bootstrap DNS"
                    } else {
                        "Saved"
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color = if (state.restartRequired) DigitDnsTheme.colors.warning else DigitDnsTheme.colors.success,
                )
            }
            Button(
                onClick = viewModel::save,
                enabled = !state.isSaving,
                modifier = Modifier.fillMaxWidth(),
            ) {
                if (state.isSaving) {
                    CircularProgressIndicator(
                        modifier = Modifier.padding(end = 8.dp).width(18.dp),
                        strokeWidth = 2.dp,
                        color = MaterialTheme.colorScheme.onPrimary,
                    )
                }
                Text("Save advanced settings")
            }
        }
    }
}
