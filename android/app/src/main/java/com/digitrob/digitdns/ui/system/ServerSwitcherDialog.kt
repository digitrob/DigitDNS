package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.Person
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.ui.components.PanelDialog
import com.digitrob.digitdns.ui.components.PanelWideDialog
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

/**
 * Fast switching from the dashboard.
 *
 * A tap on a row activates the saved server and tries its saved login immediately;
 * the picker only closes when the app has either a usable credential or a clear
 * answer that the operator must sign in. Managing the list is a second, deliberate
 * step (a full-height dialog), so the common case stays one tap.
 */
@Composable
fun ServerSwitcherDialog(
    onDismiss: () -> Unit,
    onAddServer: () -> Unit,
    onLoginRequired: () -> Unit,
    onSwitched: () -> Unit,
    onOpenSubscriber: () -> Unit = {},
    viewModel: ServersViewModel = hiltViewModel(key = "server-switcher"),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    var showManager by remember { mutableStateOf(false) }

    // The dialog is composed afresh on every open, but its view model outlives it.
    // Reload the list so a server added under Settings is visible immediately.
    LaunchedEffect(Unit) { viewModel.refresh() }

    // A subscription row asked to open the subscriber surface. The view model has
    // already flipped the app mode; the host only has to navigate.
    LaunchedEffect(state.openSubscriber) {
        if (state.openSubscriber) {
            viewModel.consumeOpenSubscriber()
            onOpenSubscriber()
        }
    }

    LaunchedEffect(state.switchOutcome) {
        when (state.switchOutcome) {
            ServerSwitchOutcome.Ready -> {
                viewModel.consumeSwitchOutcome()
                onSwitched()
            }
            ServerSwitchOutcome.LoginRequired -> {
                viewModel.consumeSwitchOutcome()
                onLoginRequired()
            }
            null -> Unit
        }
    }

    if (showManager) {
        ManageServersDialog(
            onDismiss = { showManager = false },
            onAddServer = {
                showManager = false
                onAddServer()
            },
            onLoginRequired = {
                showManager = false
                onLoginRequired()
            },
            onSwitched = {
                showManager = false
                onSwitched()
            },
            onOpenSubscriber = onOpenSubscriber,
        )
    } else {
        PanelDialog(
            onDismissRequest = onDismiss,
            solid = true,
            title = { Text("Switch server") },
            text = {
                ServerSwitchContent(
                    state = state,
                    onAddServer = onAddServer,
                    onSwitch = viewModel::switchTo,
                    onOpenSubscriber = viewModel::openSubscriber,
                )
            },
            confirmButton = {
                TextButton(onClick = { showManager = true }, enabled = !state.isBusy) {
                    Text("Manage servers")
                }
            },
            dismissButton = { TextButton(onClick = onDismiss) { Text("Close") } },
        )
    }
}

@Composable
private fun ServerSwitchContent(
    state: ServersUiState,
    onAddServer: () -> Unit,
    onSwitch: (String) -> Unit,
    onOpenSubscriber: () -> Unit,
) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        state.error?.let { message ->
            Text(
                text = message,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.error,
            )
        }

        state.subscription?.let { subscription ->
            SubscriptionSwitchRow(subscription = subscription, onOpen = onOpenSubscriber)
            HorizontalDivider()
        }

        Column(
            modifier = Modifier
                .fillMaxWidth()
                .heightIn(max = 360.dp)
                .verticalScroll(rememberScrollState()),
        ) {
            if (state.servers.isEmpty()) {
                Text(
                    "No servers saved yet.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            state.servers.forEachIndexed { index, server ->
                if (index > 0) HorizontalDivider()
                ServerSwitchRow(
                    server = server,
                    busy = state.busyId == server.id,
                    onClick = { onSwitch(server.id) },
                )
            }
        }

        OutlinedButton(
            onClick = onAddServer,
            enabled = !state.isBusy,
            modifier = Modifier.fillMaxWidth(),
        ) {
            Icon(
                Icons.Filled.Add,
                contentDescription = null,
                modifier = Modifier.padding(end = 8.dp),
            )
            Text("Add server")
        }
    }
}

/** The saved subscription, directly above the admin servers. */
@Composable
private fun SubscriptionSwitchRow(subscription: SubscriptionRow, onOpen: () -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onOpen)
            .padding(horizontal = 8.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Icon(
            Icons.Filled.Person,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.primary,
        )
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = subscription.label,
                style = MaterialTheme.typography.titleSmall,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                text = "Subscription · ${subscription.authority}",
                style = MaterialTheme.typography.bodySmall,
                fontFamily = FontFamily.Monospace,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
        Text(
            text = "Open",
            style = MaterialTheme.typography.labelLarge,
            color = MaterialTheme.colorScheme.primary,
        )
    }
}

/**
 * Convert the row's probe/credential state into the brief line under the server name.
 * The saved-credential text is evidence, not decoration: it must be true for both a
 * session pair and a master API key, so it is read from the row, not implied from
 * "is this server active at the moment".
 */
private fun savedCredentialsLine(server: ServerRow): String = when {
    server.isActive && server.credentialsSaved -> "Active · saved credentials"
    server.isActive -> "Active"
    server.credentialsSaved -> "Saved credentials — signs in automatically"
    else -> "No saved credentials"
}

@Composable
private fun ServerSwitchRow(
    server: ServerRow,
    busy: Boolean,
    onClick: () -> Unit,
) {
    val colors = DigitDnsTheme.colors
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(enabled = !busy && !server.isActive, onClick = onClick)
            .padding(horizontal = 8.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        if (busy && !server.isActive) {
            CircularProgressIndicator(strokeWidth = 2.dp, modifier = Modifier.size(20.dp))
        } else {
            Icon(
                imageVector = if (server.isActive) Icons.Filled.Check else Icons.Filled.Dns,
                contentDescription = null,
                tint = if (server.isActive) {
                    MaterialTheme.colorScheme.primary
                } else {
                    MaterialTheme.colorScheme.onSurfaceVariant
                },
            )
        }
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = server.label,
                style = MaterialTheme.typography.titleSmall,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                text = server.authority,
                style = MaterialTheme.typography.bodySmall,
                fontFamily = FontFamily.Monospace,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = savedCredentialsLine(server),
                style = MaterialTheme.typography.labelSmall,
                color = if (server.isActive) {
                    MaterialTheme.colorScheme.primary
                } else {
                    MaterialTheme.colorScheme.onSurfaceVariant
                },
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
        // The indicator sits at the row's end, decoupled from the name/authority
        // column so a long label cannot push it around.
        Box(
            modifier = Modifier
                .size(10.dp)
                .clip(CircleShape)
                .background(
                    when (server.isOnline) {
                        true -> colors.success
                        false -> colors.danger
                        null -> colors.textMuted
                    },
                ),
        )
    }
}

/** The full-height management list reached from the switcher. */
@Composable
private fun ManageServersDialog(
    onDismiss: () -> Unit,
    onAddServer: () -> Unit,
    onLoginRequired: () -> Unit,
    onSwitched: () -> Unit,
    onOpenSubscriber: () -> Unit,
) {
    PanelWideDialog(
        modifier = Modifier.fillMaxHeight(0.92f),
        onDismissRequest = onDismiss,
        scrollable = false,
        text = {
            ServersScreen(
                onLoggedOut = onLoginRequired,
                onAddServer = onAddServer,
                onLoginRequired = onLoginRequired,
                onSwitched = onSwitched,
                onOpenSubscriber = onOpenSubscriber,
                embedded = true,
                viewModel = hiltViewModel(key = "server-manager"),
            )
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text("Done") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}
