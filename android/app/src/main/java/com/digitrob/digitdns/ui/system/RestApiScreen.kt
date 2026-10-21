package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.PanelSnackbarHost
import com.digitrob.digitdns.ui.components.clickableText
import com.digitrob.digitdns.ui.components.showPanelToast
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

/**
 * The REST API Gateway section, matching the panel's top-level `REST API Gateway`.
 *
 * On a phone the useful parts are the two the operator actually touches — the master
 * key (copy, rotate) and the exposure binding — not the integration snippets, which
 * are a desktop-copy affair. The key is shown read-only and rotates only through the
 * password-confirmed action, exactly as the panel does.
 */
@Composable
fun RestApiScreen(
    viewModel: SettingsViewModel = hiltViewModel(),
    maintenanceViewModel: MaintenanceViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val maintenance by maintenanceViewModel.state.collectAsStateWithLifecycle()
    val clipboard = LocalClipboardManager.current
    val snackbar = remember { SnackbarHostState() }

    LaunchedEffect(maintenance.message) {
        val message = maintenance.message ?: return@LaunchedEffect
        snackbar.showPanelToast(message)
        maintenanceViewModel.consumeMessage()
    }

    Scaffold(snackbarHost = { PanelSnackbarHost(snackbar) }, contentWindowInsets = WindowInsets(0)) { scaffoldPadding ->
        Column(modifier = Modifier.fillMaxSize().padding(scaffoldPadding)) {
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
            state.regeneratedApiKey?.let { key ->
                PanelCard(modifier = Modifier.fillMaxWidth()) {
                    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                        Text("New API key", style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.primary)
                        Text(key, style = MaterialTheme.typography.bodyMedium.copy(fontFamily = FontFamily.Monospace))
                        Text(
                            "Store this now — it is not shown again.",
                            style = MaterialTheme.typography.bodySmall,
                            color = DigitDnsTheme.colors.warning,
                        )
                        Text(
                            "Done",
                            style = MaterialTheme.typography.labelLarge,
                            color = MaterialTheme.colorScheme.primary,
                            modifier = Modifier.clickableText { viewModel.dismissRegenerated() },
                        )
                    }
                }
            }

            if (state.isLoading) {
                CircularProgressIndicator(color = MaterialTheme.colorScheme.primary)
                return@Column
            }

            MasterKeyCard(
                apiKey = state.apiKey,
                onCopy = { clipboard.setText(AnnotatedString(state.apiKey)) },
                onRegenerate = viewModel::openRegenKey,
            )
            BindingCard(bindHost = state.bindHost, webPort = state.webPort, apiBind = state.apiBind)
            AdminPathCard(adminPath = state.adminPath, onRegenerate = viewModel::openRegenPath)

            CacheCard(maintenance, maintenanceViewModel)
        }
        }
        }
    }

    if (state.regenKeyOpen || state.regenPathOpen) {
        RegenerateDialog(viewModel)
    }
}

@Composable
private fun MasterKeyCard(apiKey: String, onCopy: () -> Unit, onRegenerate: () -> Unit) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("Master API key", style = MaterialTheme.typography.titleMedium)
            Text(
                text = apiKey.ifBlank { "—" },
                style = MaterialTheme.typography.bodyMedium.copy(fontFamily = FontFamily.Monospace),
            )
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedButton(onClick = onCopy, enabled = apiKey.isNotBlank()) { Text("Copy key") }
                OutlinedButton(onClick = onRegenerate) { Text("Regenerate") }
            }
        }
    }
}

@Composable
private fun BindingCard(bindHost: String, webPort: Int, apiBind: String) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text("Network binding & security", style = MaterialTheme.typography.titleMedium)
            InfoLine("Bind host", "$bindHost:$webPort")
            InfoLine("API bind", apiBind.ifBlank { "—" })
            Text(
                text = "Exposure addresses are edited under Network & Server.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun AdminPathCard(adminPath: String, onRegenerate: () -> Unit) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Column(modifier = Modifier.weight(1f)) {
                Text("Admin path", style = MaterialTheme.typography.titleMedium)
                Text(
                    text = adminPath.ifBlank { "—" },
                    style = MaterialTheme.typography.bodyMedium.copy(fontFamily = FontFamily.Monospace),
                )
            }
            OutlinedButton(onClick = onRegenerate) { Text("Regenerate", maxLines = 1) }
        }
    }
}
