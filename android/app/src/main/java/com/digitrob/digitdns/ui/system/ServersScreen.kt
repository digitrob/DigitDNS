package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material3.Checkbox
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
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
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.domain.model.CredentialMode
import com.digitrob.digitdns.ui.components.PanelDialog
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.SwipeToDeleteRow

/**
 * The Servers section: every saved install, which one is active, and the way to add,
 * switch, or forget one, alongside the sign-out control.
 *
 * A tap on a row switches to that install; a swipe forgets it, behind a confirmation.
 * Switching is the common action and removal is the destructive one, so the whole row is
 * the switch target and deletion stays a separate, deliberate gesture — the reverse would
 * make an accidental sign-out one stray tap away.
 */
@Composable
fun ServersScreen(
    onLoggedOut: () -> Unit,
    onAddServer: () -> Unit,
    onLoginRequired: () -> Unit = onLoggedOut,
    onSwitched: () -> Unit = {},
    embedded: Boolean = false,
    viewModel: ServersViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    // The edit dialog's target, or null while none is open. The dialog owns its
    // text fields: the screen remembers only which row opened it.
    var editing by remember { mutableStateOf<ServerRow?>(null) }

    editing?.let { target ->
        EditServerDialog(
            server = target,
            onConfirm = { edited ->
                viewModel.editServer(target.id, edited)
                editing = null
            },
            onDismiss = { editing = null },
        )
    }

    // A switch has two endings the host must act on: close the picker when it worked,
    // or replace it with the login form when this server needs the operator. A
    // transport failure stays in the list as state.error; nothing is navigated away.
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

    if (embedded) {
        ServersBody(
            state = state,
            onAddServer = onAddServer,
            onLoggedOut = onLoggedOut,
            onSwitch = viewModel::switchTo,
            onRemove = viewModel::remove,
            onEdit = { editing = it },
            modifier = Modifier.fillMaxWidth(),
            embedded = true,
        )
    } else {
        PanelRefreshBox(
            isRefreshing = state.isRefreshing,
            onRefresh = { viewModel.refresh(userInitiated = true) },
            modifier = Modifier.fillMaxSize(),
        ) {
            ServersBody(
                state = state,
                onAddServer = onAddServer,
                onLoggedOut = onLoggedOut,
                onSwitch = viewModel::switchTo,
                onRemove = viewModel::remove,
                onEdit = { editing = it },
                modifier = Modifier
                    .fillMaxSize()
                    .verticalScroll(rememberScrollState())
                    .padding(16.dp),
            )
        }
    }
}

@Composable
@Suppress("LongParameterList")
private fun ServersBody(
    state: ServersUiState,
    onAddServer: () -> Unit,
    onLoggedOut: () -> Unit,
    onSwitch: (String) -> Unit,
    onRemove: (String) -> Unit,
    onEdit: (ServerRow) -> Unit,
    modifier: Modifier,
    embedded: Boolean = false,
) {
    Column(
        modifier = modifier,
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text("Saved servers", style = MaterialTheme.typography.titleMedium)

        state.error?.let { message ->
            Text(
                text = message,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.error,
            )
        }

        if (embedded) {
            // The saved list takes the dialog's leftover height and is the only region
            // that scrolls: three uniform cards rest inside it, a fourth turns the list
            // into its own gentle scroll. The tail below (add-server, session box,
            // sign-out) then sits on the card's bottom edge, right above the Cancel/Done
            // footer, and the fixed-height card never scrolls as a whole. This scroll
            // never competes with the dialog's card scroll because that mode passes
            // scrollable = false (nested vertical scrolls are the runtime crash the
            // panel already met).
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .weight(1f)
                    .verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                SavedServersList(state, onSwitch, onRemove, onEdit)
            }
        } else {
            SavedServersList(state, onSwitch, onRemove, onEdit)
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

/** The server cards (or the empty hint) as they appear in the list. */
@Composable
private fun SavedServersList(
    state: ServersUiState,
    onSwitch: (String) -> Unit,
    onRemove: (String) -> Unit,
    onEdit: (ServerRow) -> Unit,
) {
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
                busy = state.busyId == server.id,
                onSwitch = { onSwitch(server.id) },
                onRemove = { onRemove(server.id) },
                onEdit = { onEdit(server) },
            )
        }
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun ServerCard(
    server: ServerRow,
    enabled: Boolean,
    busy: Boolean,
    onSwitch: () -> Unit,
    onRemove: () -> Unit,
    onEdit: () -> Unit,
) {
    SwipeToDeleteRow(
        onDelete = { if (enabled) onRemove() },
        confirmTitle = "Remove server?",
        confirmText = "This forgets ${server.label} (${server.authority}) and its saved " +
            "credentials on this device. The server itself is not changed.",
        confirmLabel = "Remove",
    ) {
        Card(
            colors = if (server.isActive) {
                CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.secondaryContainer)
            } else {
                CardDefaults.cardColors()
            },
            modifier = Modifier
                .fillMaxWidth()
                .combinedClickable(
                    onClick = {
                        if (enabled && !server.isActive) onSwitch()
                    },
                    onLongClick = {
                        if (enabled) onEdit()
                    },
                    role = Role.RadioButton,
                ),
        ) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(16.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                ServerCardLeading(server = server, busy = busy)
                ServerCardDetails(server = server, modifier = Modifier.weight(1f))
                IconButton(onClick = { if (enabled) onEdit() }) {
                    Icon(
                        Icons.Filled.Edit,
                        contentDescription = "Edit server",
                        modifier = Modifier.size(20.dp),
                    )
                }
            }
        }
    }
}

@Composable
private fun ServerCardLeading(server: ServerRow, busy: Boolean) {
    if (busy) {
        CircularProgressIndicator(
            strokeWidth = 2.dp,
            modifier = Modifier.size(20.dp),
        )
    } else if (server.isActive) {
        Icon(
            Icons.Filled.Check,
            contentDescription = "Active",
            tint = MaterialTheme.colorScheme.onSecondaryContainer,
        )
    }
}

/** The server card's text block. Every line is capped at one line (ellipsis) so each
 * card holds a predictable height: the manager's fixed saved-list slot then always
 * rests three cards and only scrolls when a fourth arrives. */
@Composable
private fun ServerCardDetails(server: ServerRow, modifier: Modifier = Modifier) {
    Column(
        modifier = modifier,
        verticalArrangement = Arrangement.spacedBy(2.dp),
    ) {
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
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        if (server.adminPath.isNotBlank()) {
            Text(
                text = "Admin path: ${server.adminPath}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                fontFamily = FontFamily.Monospace,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
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
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
    }
}


/** The edit dialog behind a row's pencil. The fields are deliberately narrow: they
 * rewrite the profile's connection details in place, never the id, so a saved
 * credential keyed by that id keeps working until the operator explicitly replaces it
 *
 * Credentials are optional: the password / key is entered only when it should change,
 * and "save credentials" controls whether the pair is kept for silent re-login.
 */
internal fun ServerRow.asEdit(): ServerEdit = ServerEdit(
    label = label,
    host = authority.substringBefore(':'),
    port = authority.substringAfter(':').toIntOrNull() ?: 443,
    adminPath = adminPath,
    isHttps = isHttps,
    credentialMode = credentialMode,
    username = "",
    password = "",
    apiKey = "",
    saveCredentials = credentialsSaved,
)

@Composable
private fun EditServerDialog(
    server: ServerRow,
    onConfirm: (ServerEdit) -> Unit,
    onDismiss: () -> Unit,
) {
    var state by remember { mutableStateOf(server.asEdit()) }

    PanelDialog(
        onDismissRequest = onDismiss,
        solid = true,
        title = { Text("Edit server") },
        text = { ServerEditFields(state, onStateChange = { state = it }) },
        confirmButton = {
            TextButton(
                enabled = state.host.isNotBlank() && state.port in 1..65535,
                onClick = { onConfirm(state) },
            ) { Text("Save") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@Composable
private fun ServerEditFields(state: ServerEdit, onStateChange: (ServerEdit) -> Unit) {
    Column(
        modifier = Modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        ServerConnectionFields(state, onStateChange)
        ServerCredentialFields(state, onStateChange)
    }
}

@Composable
private fun ServerConnectionFields(state: ServerEdit, onStateChange: (ServerEdit) -> Unit) {
    OutlinedTextField(
        value = state.label,
        onValueChange = { onStateChange(state.copy(label = it)) },
        label = { Text("Friendly name") },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
    OutlinedTextField(
        value = state.host,
        onValueChange = { onStateChange(state.copy(host = it.trim())) },
        label = { Text("Host") },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
    OutlinedTextField(
        value = state.port.toString(),
        onValueChange = { text -> text.toIntOrNull()?.let { onStateChange(state.copy(port = it)) } },
        label = { Text("Port") },
        singleLine = true,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
        modifier = Modifier.fillMaxWidth(),
    )
    OutlinedTextField(
        value = state.adminPath,
        onValueChange = { onStateChange(state.copy(adminPath = it.trim('/'))) },
        label = { Text("Admin path") },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
    TextButton(
        onClick = { onStateChange(state.copy(isHttps = !state.isHttps)) },
        modifier = Modifier.fillMaxWidth(),
    ) { Text(if (state.isHttps) "HTTPS" else "HTTP") }
}

@Composable
private fun ServerCredentialFields(state: ServerEdit, onStateChange: (ServerEdit) -> Unit) {
    TextButton(
        onClick = {
            val nextMode = when (state.credentialMode) {
                CredentialMode.SESSION -> CredentialMode.API_KEY
                CredentialMode.API_KEY -> CredentialMode.SESSION
            }
            onStateChange(state.copy(credentialMode = nextMode))
        },
        modifier = Modifier.fillMaxWidth(),
    ) {
        Text(
            when (state.credentialMode) {
                CredentialMode.SESSION -> "Account session"
                CredentialMode.API_KEY -> "Master API key"
            },
        )
    }

    if (state.credentialMode == CredentialMode.SESSION) {
        OutlinedTextField(
            value = state.username,
            onValueChange = { onStateChange(state.copy(username = it.trim())) },
            label = { Text("Username") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        OutlinedTextField(
            value = state.password,
            onValueChange = { onStateChange(state.copy(password = it)) },
            label = { Text("Password") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
    } else {
        OutlinedTextField(
            value = state.apiKey,
            onValueChange = { onStateChange(state.copy(apiKey = it.trim())) },
            label = { Text("API key") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
    }

    Row(verticalAlignment = Alignment.CenterVertically) {
        Checkbox(
            checked = state.saveCredentials,
            onCheckedChange = { onStateChange(state.copy(saveCredentials = it)) },
        )
        Text("Save credentials", style = MaterialTheme.typography.bodyMedium)
    }
}
