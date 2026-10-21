package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelDialog
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

/**
 * Server settings: address exposure, session window, and the two credential
 * rotations. The API key is shown read-only (copyable from the field) and the admin
 * path is shown but not editable — both change only through their regenerate actions.
 */
@Composable
fun SettingsScreen(viewModel: SettingsViewModel = hiltViewModel()) {
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
        state.savedMessage?.let { message ->
            PanelCard {
                Row(modifier = Modifier.fillMaxWidth()) {
                    Text(message, color = MaterialTheme.colorScheme.primary, modifier = Modifier.weight(1f))
                    TextButton(onClick = viewModel::dismissMessage) { Text("OK") }
                }
            }
        }

        if (state.isLoading) {
            CircularProgressIndicator(color = MaterialTheme.colorScheme.primary)
            return@Column
        }

        state.regeneratedApiKey?.let { key ->
            RevealCard(
                title = "New API key",
                value = key,
                warning = "Store this now — it is not shown again.",
                onDismiss = viewModel::dismissRegenerated,
            )
        }
        state.regeneratedAdminPath?.let { path ->
            RevealCard(
                title = "New admin path",
                value = path,
                warning = "Every bookmark and session under the old path has stopped working.",
                onDismiss = viewModel::dismissRegenerated,
            )
        }

        ServerCard(state)
        ServerFields(state, viewModel)

        Button(
            onClick = viewModel::save,
            enabled = !state.isSaving,
            modifier = Modifier.fillMaxWidth(),
        ) {
            if (state.isSaving) {
                CircularProgressIndicator(
                    modifier = Modifier.padding(end = 8.dp),
                    strokeWidth = 2.dp,
                    color = MaterialTheme.colorScheme.onPrimary,
                )
            }
            Text("Save settings")
        }
    }
    }

    if (state.regenKeyOpen || state.regenPathOpen) {
        RegenerateDialog(viewModel)
    }
}

@Composable
private fun ServerCard(state: SettingsUiState) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text("Server", style = MaterialTheme.typography.titleMedium)
            InfoRow("Admin user", state.adminUsername)
            InfoRow("Bind host", "${state.bindHost}:${state.webPort}")
            if (state.adminPasswordWeak) {
                Text(
                    text = "The admin password is weak — change it in the web panel.",
                    color = DigitDnsTheme.colors.warning,
                    style = MaterialTheme.typography.bodySmall,
                )
            }
        }
    }
}

@Composable
private fun ServerFields(state: SettingsUiState, viewModel: SettingsViewModel) {
    OutlinedTextField(
        value = state.publicIp,
        onValueChange = viewModel::onPublicIpChange,
        label = { Text("Public IP") },
        supportingText = { Hint("Advertised to clients; blank leaves it unchanged.") },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
    OutlinedTextField(
        value = state.apiBind,
        onValueChange = viewModel::onApiBindChange,
        label = { Text("API bind") },
        supportingText = { Hint("IP or CIDR the REST API listens on.") },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
    OutlinedTextField(
        value = state.apiBindList,
        onValueChange = viewModel::onApiBindListChange,
        label = { Text("API bind list") },
        supportingText = { Hint("One CIDR per line. Non-empty overrides the single bind above.") },
        minLines = 2,
        modifier = Modifier.fillMaxWidth(),
    )
    OutlinedTextField(
        value = state.sessionIdleMinutes,
        onValueChange = viewModel::onSessionIdleChange,
        label = { Text("Session idle minutes") },
        supportingText = { Hint("5–1440. How long a session survives without use.") },
        singleLine = true,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
        modifier = Modifier.fillMaxWidth(),
    )
}

/** The one-line supporting text shared by the fields on this screen. */
@Composable
private fun Hint(text: String) {
    Text(text, style = MaterialTheme.typography.bodySmall)
}

@Composable
internal fun InfoRow(label: String, value: String) {
    Row(modifier = Modifier.fillMaxWidth()) {
        Text(
            text = label,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.weight(1f),
        )
        Text(
            text = value,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurface,
        )
    }
}

@Composable
private fun RevealCard(title: String, value: String, warning: String, onDismiss: () -> Unit) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(title, style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.primary)
            Text(value, style = MaterialTheme.typography.bodyMedium.copy(fontFamily = com.digitrob.digitdns.ui.theme.PanelMono))
            Text(warning, style = MaterialTheme.typography.bodySmall, color = DigitDnsTheme.colors.warning)
            TextButton(onClick = onDismiss) { Text("Done") }
        }
    }
}

@Composable
internal fun RegenerateDialog(viewModel: SettingsViewModel) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val isKey = state.regenKeyOpen

    PanelDialog(
        onDismissRequest = viewModel::closeRegen,
        title = { Text(if (isKey) "Regenerate API key?" else "Regenerate admin path?") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Text(
                    text = if (isKey) {
                        "The current key stops working immediately. Integrations using it must be updated."
                    } else {
                        "Every session and bookmark under the current path stops working immediately. The app will adopt the new path."
                    },
                    style = MaterialTheme.typography.bodySmall,
                )
                OutlinedTextField(
                    value = state.currentPassword,
                    onValueChange = viewModel::onCurrentPasswordChange,
                    label = { Text("Current password") },
                    singleLine = true,
                    visualTransformation = PasswordVisualTransformation(),
                    modifier = Modifier.fillMaxWidth(),
                )
                OutlinedTextField(
                    value = state.totpCode,
                    onValueChange = viewModel::onTotpCodeChange,
                    label = { Text("2FA code (if enabled)") },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.NumberPassword),
                    modifier = Modifier.fillMaxWidth(),
                )
            }
        },
        confirmButton = {
            TextButton(
                onClick = { if (isKey) viewModel.regenerateApiKey() else viewModel.regenerateAdminPath() },
                enabled = !state.regenerating && state.currentPassword.isNotBlank(),
            ) { Text(if (state.regenerating) "Working…" else "Regenerate") }
        },
        dismissButton = { TextButton(onClick = viewModel::closeRegen) { Text("Cancel") } },
    )
}
