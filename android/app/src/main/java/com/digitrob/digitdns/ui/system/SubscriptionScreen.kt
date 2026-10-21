package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
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
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono

/**
 * The subscriber portal settings. The portal hands clients their configuration, so its
 * origin matters — the computed origin is shown after a save so the operator can see
 * the URL the daemon will actually advertise.
 */
@Composable
fun SubscriptionScreen(viewModel: SubscriptionViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()

    Column(modifier = Modifier.fillMaxSize()) {
        PanelRefreshBox(
            isRefreshing = state.isLoading,
            onRefresh = viewModel::load,
            modifier = Modifier.fillMaxSize(),
        ) {
            Column(
                modifier = Modifier
                    .fillMaxSize()
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = 12.dp, vertical = 8.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                state.error?.let {
                    Text(it, color = DigitDnsTheme.colors.danger, style = MaterialTheme.typography.bodySmall)
                }
                state.savedMessage?.let { message ->
                    PanelCard {
                        Row(modifier = Modifier.fillMaxWidth()) {
                            Text(message, color = MaterialTheme.colorScheme.primary, modifier = Modifier.weight(1f))
                            TextButton(onClick = viewModel::dismissMessage) { Text("OK") }
                        }
                    }
                }
                if (state.origin.isNotBlank()) {
                    OriginCard(state.origin)
                }

                if (state.isLoading) {
                    CircularProgressIndicator(color = MaterialTheme.colorScheme.primary)
                    return@Column
                }

                ToggleCard(state, viewModel)
                PortalFields(state, viewModel)
                ThemeSourcePicker(state.themeCssSource, viewModel::onThemeSourceChange)
                ThemeFields(state, viewModel)
                SaveButton(state.isSaving, viewModel::save)
            }
        }
    }
}

@Composable
private fun OriginCard(origin: String) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(
                text = "Portal origin",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(origin, style = MaterialTheme.typography.bodyMedium.copy(fontFamily = PanelMono))
        }
    }
}

/** Both on/off switches in one card, each a single compact row, to save vertical space. */
@Composable
private fun ToggleCard(state: SubscriptionUiState, viewModel: SubscriptionViewModel) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                text = "Enable subscription portal",
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.weight(1f),
            )
            Switch(checked = state.enabled, onCheckedChange = viewModel::onEnabledChange)
        }
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                text = "Auto-renew certificate",
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.weight(1f),
            )
            Switch(checked = state.autoRenewAcme, onCheckedChange = viewModel::onAutoRenewChange)
        }
    }
}

@Composable
private fun PortalFields(state: SubscriptionUiState, viewModel: SubscriptionViewModel) {
    OutlinedTextField(
        value = state.domain,
        onValueChange = viewModel::onDomainChange,
        label = { Text("Domain") },
        supportingText = { Hint("Blank uses the panel's own host.") },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
    OutlinedTextField(
        value = state.port,
        onValueChange = viewModel::onPortChange,
        label = { Text("Port") },
        supportingText = { Hint("A custom domain needs its own port, distinct from the panel's.") },
        singleLine = true,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
        modifier = Modifier.fillMaxWidth(),
    )
    OutlinedTextField(
        value = state.title,
        onValueChange = viewModel::onTitleChange,
        label = { Text("Title") },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
}

@Composable
private fun ThemeFields(state: SubscriptionUiState, viewModel: SubscriptionViewModel) {
    when (state.themeCssSource) {
        "local" -> OutlinedTextField(
            value = state.themeCssPath,
            onValueChange = viewModel::onThemePathChange,
            label = { Text("Theme CSS path") },
            supportingText = { Hint("Absolute path on the server, e.g. /etc/digitdns/theme.css") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        "url" -> OutlinedTextField(
            value = state.themeCssUrl,
            onValueChange = viewModel::onThemeUrlChange,
            label = { Text("Theme CSS URL") },
            supportingText = { Hint("An absolute http(s) URL.") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        else -> OutlinedTextField(
            value = state.themeCss,
            onValueChange = viewModel::onThemeCssChange,
            label = { Text("Inline theme CSS") },
            minLines = 3,
            modifier = Modifier.fillMaxWidth().heightIn(min = 96.dp),
        )
    }
}

@Composable
private fun SaveButton(saving: Boolean, onSave: () -> Unit) {
    Button(
        onClick = onSave,
        enabled = !saving,
        modifier = Modifier.fillMaxWidth(),
    ) {
        if (saving) {
            CircularProgressIndicator(
                modifier = Modifier.padding(end = 8.dp),
                strokeWidth = 2.dp,
                color = MaterialTheme.colorScheme.onPrimary,
            )
        }
        Text("Save subscription")
    }
}

/** The one-line supporting text shared by the fields on this screen. */
@Composable
private fun Hint(text: String) {
    Text(text, style = MaterialTheme.typography.bodySmall)
}

@Composable
private fun ThemeSourcePicker(selected: String, onSelect: (String) -> Unit) {
    val options = listOf("inline" to "Inline", "local" to "Local file", "url" to "URL")
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        options.forEach { (value, label) ->
            FilterChip(
                selected = selected == value,
                onClick = { onSelect(value) },
                label = { Text(label) },
            )
        }
    }
}
