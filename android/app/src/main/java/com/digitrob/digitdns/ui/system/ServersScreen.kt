package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Check
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.domain.model.CredentialMode
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.PanelDialog
import com.digitrob.digitdns.ui.components.SwipeToDeleteRow

/**
 * The Servers section: every saved install, which one is active, and the way to add,
 * switch, or forget one, alongside the sign-out control.
 *
 * A tap on a row switches to that install; a delete icon forgets it. Switching is the
 * common action and removal is the destructive one, so the whole row is the switch
 * target and deletion is a separate, confirmed button — the reverse would make an
 * accidental sign-out one stray tap away.
 */
@Composable
fun ServersScreen(
    onLoggedOut: () -> Unit,
    onAddServer: () -> Unit,
    viewModel: ServersViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    var pendingRemoval by remember { mutableStateOf<ServerRow?>(null) }

    PanelRefreshBox(
        isRefreshing = state.isBusy,
        onRefresh = viewModel::refresh,
        modifier = Modifier.fillMaxSize(),
    ) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
        Text("Saved servers", style = MaterialTheme.typography.titleMedium)

        if (state.servers.isEmpty()) {
            Text(
                text = "No servers saved yet.",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        } else {
            state.servers.forEach { server ->
                ServerCard(
                    server = server,
                    enabled = !state.isBusy,
                    onSwitch = { viewModel.switchTo(server.id) },
                    onRemove = { pendingRemoval = server },
                )
            }
        }

        OutlinedButton(
            onClick = onAddServer,
            enabled = !state.isBusy,
            modifier = Modifier.fillMaxWidth(),
        ) {
            Icon(Icons.Filled.Add, contentDescription = null, modifier = Modifier.padding(end = 8.dp))
            Text("Add server")
        }

        SessionScreen(onLoggedOut = onLoggedOut)
    }
    }

    pendingRemoval?.let { target ->
        RemoveServerDialog(
            server = target,
            onConfirm = {
                viewModel.remove(target.id)
                pendingRemoval = null
            },
            onDismiss = { pendingRemoval = null },
        )
    }
}

@Composable
private fun ServerCard(
    server: ServerRow,
    enabled: Boolean,
    onSwitch: () -> Unit,
    onRemove: () -> Unit,
) {
    SwipeToDeleteRow(
        onDelete = { if (enabled) onRemove() },
    ) {
        Card(
            modifier = Modifier
                .fillMaxWidth()
                .selectable(
                    selected = server.isActive,
                    enabled = enabled && !server.isActive,
                    role = Role.RadioButton,
                    onClick = onSwitch,
                ),
            colors = if (server.isActive) {
                CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.secondaryContainer)
            } else {
                CardDefaults.cardColors()
            },
        ) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(16.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                if (server.isActive) {
                    Icon(
                        Icons.Filled.Check,
                        contentDescription = "Active",
                        tint = MaterialTheme.colorScheme.onSecondaryContainer,
                    )
                }
                Column(
                    modifier = Modifier.weight(1f),
                    verticalArrangement = Arrangement.spacedBy(2.dp),
                ) {
                    Text(server.label, style = MaterialTheme.typography.titleSmall)
                    Text(
                        text = server.authority,
                        style = MaterialTheme.typography.bodySmall,
                        fontFamily = FontFamily.Monospace,
                    )
                    if (server.adminPath.isNotBlank()) {
                        Text(
                            text = "Admin path: ${server.adminPath}",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            fontFamily = FontFamily.Monospace,
                        )
                    }
                    Text(
                        text = buildString {
                            append(
                                when (server.credentialMode) {
                                    CredentialMode.SESSION -> "Account session"
                                    CredentialMode.API_KEY -> "Master API key"
                                },
                            )
                            if (server.credentialsSaved) append(" · credentials saved")
                        },
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
    }
}

@Composable
private fun RemoveServerDialog(
    server: ServerRow,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    PanelDialog(
        onDismissRequest = onDismiss,
        title = { Text("Remove server?") },
        text = {
            Text(
                "This forgets ${server.label} (${server.authority}) and its saved " +
                    "credentials on this device. The server itself is not changed.",
            )
        },
        confirmButton = { TextButton(onClick = onConfirm) { Text("Remove") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}
