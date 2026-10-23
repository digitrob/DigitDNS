package com.digitrob.digitdns.ui.clients

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Autorenew
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.Key
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.RestartAlt
import androidx.compose.material.icons.filled.Visibility
import androidx.compose.material.icons.filled.VisibilityOff
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.TextButton
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.domain.model.Client
import com.digitrob.digitdns.domain.model.ClientAction
import com.digitrob.digitdns.domain.model.ClientLinks
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelDialog
import com.digitrob.digitdns.ui.components.PanelSnackbarHost
import com.digitrob.digitdns.ui.components.PanelWideDialog
import com.digitrob.digitdns.ui.components.showPanelToast
import com.digitrob.digitdns.ui.formatCount
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * Edit one client, shown as a popup over the client list.
 *
 * The list stays where it was: opening a client no longer pushes a page over it, so
 * the operator can close the popup and carry on with the row they were reading. The
 * form is the same one the new-client popup uses; the actions that used to sit in a
 * bottom tray now live in the title's overflow menu, keeping the card compact enough
 * to fit without scrolling.
 *
 * Action results are shown as a toast pinned over the card rather than as a card
 * inserted above the form. Inserting it moved every field down after each tap, which
 * read as the popup sliding apart; a toast takes no layout space.
 */
@Composable
fun ClientDetailDialog(
    clientId: String,
    origin: String,
    onDismiss: () -> Unit,
    onChanged: () -> Unit,
    viewModel: ClientDetailViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }
    var showUuidEditor by remember { mutableStateOf(false) }
    val clipboard = LocalClipboardManager.current
    val copy: (String, String) -> Unit = { value, label ->
        clipboard.setText(AnnotatedString(value))
        viewModel.noteCopied(label)
    }

    // Whether the state on screen belongs to this client at all. Reopening the popup
    // and switching clients both start from the previous client's state, so anything
    // that shows or acts on it has to wait until the load replaces it.
    val loadedForThisClient = (state.mode as? ClientDetailMode.Edit)?.id == clientId

    // Saving or deleting finishes the job: the popup closes and the list behind it
    // refreshes. The guard keeps a previous client's flag from closing a fresh popup.
    LaunchedEffect(state.saved, state.deleted, loadedForThisClient) {
        if (loadedForThisClient && (state.saved || state.deleted)) onChanged()
    }

    LaunchedEffect(state.message) {
        val message = state.message ?: return@LaunchedEffect
        snackbar.showPanelToast(message)
        viewModel.dismissMessage()
    }

    PanelWideDialog(
        onDismissRequest = onDismiss,
        // A stable height is what stops the popup resizing when the client arrives:
        // the spinner, the form and the credentials card all live in the same window,
        // so loading no longer looks like the panel growing out of the screen.
        title = {
            ClientDetailTitle(
                state = state,
                loaded = loadedForThisClient,
                onAction = viewModel::runAction,
                onDelete = viewModel::delete,
            )
        },
        text = {
            ClientDetailBody(
                state = state,
                loaded = loadedForThisClient,
                origin = origin,
                onCopy = copy,
                snackbar = snackbar,
                onEditUuid = { showUuidEditor = true },
                viewModel = viewModel,
            )
        },
        confirmButton = {
            TextButton(onClick = viewModel::save, enabled = state.canSave) {
                if (state.isSaving) {
                    CircularProgressIndicator(
                        modifier = Modifier.padding(end = 8.dp),
                        strokeWidth = 2.dp,
                        color = MaterialTheme.colorScheme.primary,
                    )
                }
                Text("Save")
            }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )

    if (showUuidEditor && state.client != null) {
        UuidEditorDialog(
            initial = state.uuid,
            onDismiss = { showUuidEditor = false },
            onConfirm = { uuid ->
                viewModel.onUuidChange(uuid)
                showUuidEditor = false
            },
        )
    }
}

@Composable
private fun ClientDetailTitle(
    state: ClientDetailUiState,
    loaded: Boolean,
    onAction: (ClientAction) -> Unit,
    onDelete: () -> Unit,
) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = if (loaded) state.client?.name?.ifBlank { null } ?: "Client" else "Client",
            style = MaterialTheme.typography.titleMedium,
            color = MaterialTheme.colorScheme.onSurface,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f),
        )
        if (state.busyAction) {
            CircularProgressIndicator(strokeWidth = 2.dp, modifier = Modifier.size(18.dp))
            Spacer(Modifier.width(4.dp))
        }
        val client = state.client
        if (loaded && client != null) {
            ClientOverflowMenu(
                client = client,
                busy = state.busyAction,
                onAction = onAction,
                onDelete = onDelete,
            )
        }
    }
}

@Composable
private fun ClientDetailBody(
    state: ClientDetailUiState,
    loaded: Boolean,
    origin: String,
    onCopy: (String, String) -> Unit,
    snackbar: SnackbarHostState,
    onEditUuid: () -> Unit,
    viewModel: ClientDetailViewModel,
) {
    Box {
        Column(
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            if (!loaded || state.isLoading) {
                // Avoid showing loading spinner; open details as soon as possible
                state.client?.let { 
                    ClientForm(state = state, viewModel = viewModel)
                    if (!state.isCreate) {
                        ClientCredentials(
                            client = it,
                            origin = origin,
                            onCopy = onCopy,
                            onEditUuid = onEditUuid,
                        )
                    }
                } ?: run {
                    Row(
                        modifier = Modifier.fillMaxWidth().heightIn(min = 200.dp),
                        horizontalArrangement = Arrangement.Center,
                        verticalAlignment = Alignment.CenterVertically,
                    ) {}
                }
            } else {
                state.error?.let {
                    Text(it, color = DigitDnsTheme.colors.danger, style = MaterialTheme.typography.bodySmall)
                }

                ClientForm(state = state, viewModel = viewModel)

                val client = state.client
                if (!state.isCreate && client != null) {
                    ClientCredentials(
                        client = client,
                        origin = origin,
                        onCopy = onCopy,
                        onEditUuid = onEditUuid,
                    )
                }
            }
        }
        Box(modifier = Modifier.align(Alignment.BottomCenter)) {
            PanelSnackbarHost(hostState = snackbar)
        }
    }
}

/** Edit the UUID without taking over the top of the form; the main Save writes it. */
@Composable
private fun UuidEditorDialog(
    initial: String,
    onDismiss: () -> Unit,
    onConfirm: (String) -> Unit,
) {
    var draft by remember(initial) { mutableStateOf(initial) }
    PanelDialog(
        onDismissRequest = onDismiss,
        solid = true,
        title = { Text("Edit UUID") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(
                    value = draft,
                    onValueChange = { draft = it.trim() },
                    label = { Text("UUID") },
                    singleLine = true,
                    textStyle = MaterialTheme.typography.bodyMedium.copy(fontFamily = FontFamily.Monospace),
                    modifier = Modifier.fillMaxWidth(),
                )
                Text(
                    "Saved with the client form, not on this screen.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        },
        confirmButton = {
            TextButton(onClick = { onConfirm(draft) }, enabled = draft.isNotBlank()) {
                Text("Use UUID")
            }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

/** The editable fields. Split out so the popup body stays readable. */
@Composable
internal fun ClientForm(
    state: ClientDetailUiState,
    viewModel: ClientDetailViewModel,
) {
    OutlinedTextField(
        value = state.name,
        onValueChange = viewModel::onNameChange,
        label = { Text("Name") },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
    OutlinedTextField(
        value = state.allowedIps,
        onValueChange = viewModel::onAllowedIpsChange,
        label = { Text("Allowed IPs (comma or space separated)") },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
    // The two numbers share a row so the form fits a single window; the labels
    // have to stay short enough for half the width.
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        OutlinedTextField(
            value = state.trafficLimitGb,
            onValueChange = viewModel::onTrafficLimitChange,
            label = { Text("Traffic (GB)") },
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
            modifier = Modifier.weight(1f),
        )
        OutlinedTextField(
            value = state.maxDevices,
            onValueChange = viewModel::onMaxDevicesChange,
            label = { Text("Max devices") },
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
            modifier = Modifier.weight(1f),
        )
    }
    ExpiryField(
        value = state.expiryField,
        onChange = viewModel::onExpiryChange,
        onClear = { viewModel.onExpiryChange("") },
    )
    if (state.isCreate) {
        Text(
            "Blank limit = unlimited. Blank expiry = lifetime.",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    } else if (!state.expiryValid) {
        Text(
            "Use YYYY-MM-DD HH:MM, or clear the field for lifetime.",
            style = MaterialTheme.typography.bodySmall,
            color = DigitDnsTheme.colors.danger,
        )
    }
    Column(verticalArrangement = Arrangement.spacedBy(0.dp)) {
        ResetCycleChips(selected = state.resetCycle, onSelect = viewModel::onResetCycleChange)
        NoteField(
            value = state.note,
            onValueChange = viewModel::onNoteChange,
        )
    }
}

/**
 * A collapsible note field. The note is the form value least likely to be edited,
 * and the one whose text field is tall enough to push the rest of the popup into a
 * scroll on a phone, so it is behind a small toggle by default.
 */
@Composable
internal fun NoteField(
    value: String,
    onValueChange: (String) -> Unit,
) {
    OutlinedTextField(
        value = value,
        onValueChange = onValueChange,
        label = { Text("Note") },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
}

/**
 * The read-only credentials and usage block: the link and secret the subscriber
 * needs, plus the figures the web card shows. Each value copies on tap, matching
 * the panel's copy buttons.
 */
@Composable
private fun ClientCredentials(
    client: Client,
    origin: String,
    onCopy: (String, String) -> Unit,
    onEditUuid: () -> Unit,
) {
    var secretVisible by remember { mutableStateOf(false) }
    val subscription = if (origin.isBlank()) "" else ClientLinks.subscriptionLink(origin, client)
    val expiry = ClientLinks.expiryLabel(client.expiresAt, ZoneId.systemDefault())
    val nextReset = client.nextTrafficReset?.let {
        DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM, FormatStyle.SHORT)
            .withZone(ZoneId.systemDefault())
            .format(it)
    }

    PanelCard(modifier = Modifier.fillMaxWidth(), contentPadding = 8.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(0.dp)) {
            CredentialRow(
                label = "Subscription",
                value = subscription.ifBlank { "Not configured" },
                copyValue = subscription.takeIf { it.isNotBlank() },
                onCopy = onCopy,
            )
            CredentialRow(
                label = "Secret",
                value = when {
                    client.registerSecret.isBlank() -> "No secret on file"
                    secretVisible -> client.registerSecret
                    else -> "••••••••••••"
                },
                copyValue = client.registerSecret.takeIf { it.isNotBlank() },
                onCopy = onCopy,
                trailing = {
                    if (client.registerSecret.isNotBlank()) {
                        IconButton(onClick = { secretVisible = !secretVisible }) {
                            Icon(
                                imageVector = if (secretVisible) Icons.Filled.VisibilityOff else Icons.Filled.Visibility,
                                contentDescription = if (secretVisible) "Hide secret" else "Show secret",
                            )
                        }
                    }
                },
            )
            CredentialRow(
                label = "UUID",
                value = client.uuid.ifBlank { "N/A" },
                copyValue = client.uuid.takeIf { it.isNotBlank() },
                onCopy = onCopy,
                trailing = {
                    IconButton(onClick = onEditUuid, modifier = Modifier.size(36.dp)) {
                        Icon(
                            Icons.Filled.Edit,
                            contentDescription = "Edit UUID",
                            modifier = Modifier.size(18.dp),
                        )
                    }
                },
            )
            InfoRow("Plan expiry", expiry)
            UsageGrid(client = client, nextReset = nextReset)
        }
    }
}

/** The four usage figures in a compact 2×2 grid instead of four full-width rows. */
@Composable
private fun UsageGrid(client: Client, nextReset: String?) {
    Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        UsageCell(
            label = "Traffic used",
            value = com.digitrob.digitdns.ui.formatBytes(client.trafficUsedBytes),
            modifier = Modifier.weight(1f),
        )
        UsageCell(
            label = "Queries",
            value = formatCount(client.totalQueries),
            modifier = Modifier.weight(1f),
        )
    }
    Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        UsageCell(
            label = "Last seen",
            value = client.lastSeen?.let { ClientLinks.expiryLabel(it) } ?: "Never",
            modifier = Modifier.weight(1f),
        )
        UsageCell(
            label = "Next reset",
            value = nextReset ?: "Not set",
            modifier = Modifier.weight(1f),
        )
    }
}

@Composable
private fun UsageCell(label: String, value: String, modifier: Modifier = Modifier) {
    Column(modifier = modifier) {
        Text(
            text = label.uppercase(),
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        Text(
            text = value,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurface,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
    }
}

@Composable
private fun CredentialRow(
    label: String,
    value: String,
    copyValue: String?,
    onCopy: (String, String) -> Unit,
    trailing: (@Composable () -> Unit)? = null,
) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = label,
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.width(80.dp),
        )
        Text(
            text = value,
            style = MaterialTheme.typography.bodySmall.copy(fontFamily = FontFamily.Monospace),
            color = MaterialTheme.colorScheme.onSurface,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f),
        )
        trailing?.invoke()
        if (copyValue != null) {
            IconButton(
                onClick = { onCopy(copyValue, label) },
                modifier = Modifier.size(36.dp),
            ) {
                Icon(
                    Icons.Filled.ContentCopy,
                    contentDescription = "Copy $label",
                    modifier = Modifier.size(18.dp),
                )
            }
        }
    }
}

@Composable
private fun InfoRow(label: String, value: String) {
    Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Text(
            text = label,
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.width(80.dp),
        )
        Text(
            text = value,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurface,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f),
        )
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun ResetCycleChips(selected: String, onSelect: (String) -> Unit) {
    val options = listOf("" to "Never", "daily" to "Daily", "weekly" to "Weekly", "monthly" to "Monthly")
    // The row spans the full width and the chips sit centred in it; a flow row so
    // they wrap rather than run off the edge on a narrow screen.
    FlowRow(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.CenterHorizontally),
        verticalArrangement = Arrangement.spacedBy(2.dp),
    ) {
        options.forEach { (value, label) ->
            FilterChip(
                selected = selected == value,
                onClick = { onSelect(value) },
                label = { Text(label, style = MaterialTheme.typography.labelMedium) },
            )
        }
    }
}

/**
 * The action menu beside the client name.
 *
 * Every action except Delete applies immediately — the panel behaves the same way
 * and a confirmation for "enable" is friction, not safety. Regenerating the UUID or
 * the secret is the one pair that invalidates every config the subscriber already
 * has, so those two warn first; the warning is a dialog, and the result is the
 * standard toast above.
 */
@Composable
private fun ClientOverflowMenu(
    client: Client,
    busy: Boolean,
    onAction: (ClientAction) -> Unit,
    onDelete: () -> Unit,
) {
    var expanded by remember { mutableStateOf(false) }
    var pendingInvalidate by remember { mutableStateOf<ClientAction?>(null) }
    var showDelete by remember { mutableStateOf(false) }

    val colors = DigitDnsTheme.colors
    Box {
        IconButton(onClick = { expanded = true }, enabled = !busy) {
            Icon(Icons.Filled.MoreVert, contentDescription = "Client actions")
        }
        DropdownMenu(
            expanded = expanded,
            onDismissRequest = { expanded = false },
            modifier = Modifier
                .background(colors.bgCard)
                .border(1.dp, colors.borderColor, RoundedCornerShape(12.dp)),
        ) {
            ClientOverflowItems(
                client = client,
                busy = busy,
                onDismiss = { expanded = false },
                onAction = onAction,
                onInvalidate = { pendingInvalidate = it },
                onDelete = { showDelete = true },
            )
        }
    }

    ClientOverflowDialogs(
        pendingInvalidate = pendingInvalidate,
        showDelete = showDelete,
        onDismissInvalidate = { pendingInvalidate = null },
        onConfirmInvalidate = { action ->
            pendingInvalidate = null
            onAction(action)
        },
        onDismissDelete = { showDelete = false },
        onConfirmDelete = {
            onDelete()
            showDelete = false
        },
    )
}

/** The menu rows themselves; the overflow wrapper only owns open/dialog state. */
@Composable
private fun ClientOverflowItems(
    client: Client,
    busy: Boolean,
    onDismiss: () -> Unit,
    onAction: (ClientAction) -> Unit,
    onInvalidate: (ClientAction) -> Unit,
    onDelete: () -> Unit,
) {
    DropdownMenuItem(
        text = { Text(if (client.enabled) "Disable" else "Enable") },
        leadingIcon = {
            Icon(
                if (client.enabled) Icons.Filled.Pause else Icons.Filled.PlayArrow,
                contentDescription = null,
            )
        },
        enabled = !busy,
        onClick = {
            onDismiss()
            onAction(ClientAction.Toggle(!client.enabled))
        },
    )
    HorizontalDivider()
    RenewMenuItems(
        enabled = !busy,
        onRenew = {
            onDismiss()
            onAction(ClientAction.Renew(it))
        },
    )
    HorizontalDivider()
    DropdownMenuItem(
        text = { Text("Reset traffic") },
        leadingIcon = { Icon(Icons.Filled.RestartAlt, contentDescription = null) },
        enabled = !busy,
        onClick = {
            onDismiss()
            onAction(ClientAction.ResetTraffic)
        },
    )
    DropdownMenuItem(
        text = { Text("New UUID") },
        leadingIcon = { Icon(Icons.Filled.Refresh, contentDescription = null) },
        enabled = !busy,
        onClick = {
            onDismiss()
            onInvalidate(ClientAction.RegenerateUuid)
        },
    )
    DropdownMenuItem(
        text = { Text("New secret") },
        leadingIcon = { Icon(Icons.Filled.Key, contentDescription = null) },
        enabled = !busy,
        onClick = {
            onDismiss()
            onInvalidate(ClientAction.RegenerateRegisterSecret)
        },
    )
    HorizontalDivider()
    DeleteMenuItem(
        enabled = !busy,
        onClick = {
            onDismiss()
            onDelete()
        },
    )
}

/** The two confirmations the menu can raise, split out to keep the menu itself short. */
@Composable
private fun ClientOverflowDialogs(
    pendingInvalidate: ClientAction?,
    showDelete: Boolean,
    onDismissInvalidate: () -> Unit,
    onConfirmInvalidate: (ClientAction) -> Unit,
    onDismissDelete: () -> Unit,
    onConfirmDelete: () -> Unit,
) {
    pendingInvalidate?.let { action ->
        InvalidateDialog(
            action = action,
            onDismiss = onDismissInvalidate,
            onConfirm = { onConfirmInvalidate(action) },
        )
    }

    if (showDelete) {
        DeleteDialog(onDismiss = onDismissDelete, onConfirm = onConfirmDelete)
    }
}

/** The four renew presets, listed directly rather than behind a second menu. */
@Composable
private fun RenewMenuItems(enabled: Boolean, onRenew: (Int) -> Unit) {
    listOf(7, 30, 60, 90).forEach { days ->
        DropdownMenuItem(
            text = { Text("+$days days") },
            leadingIcon = { Icon(Icons.Filled.Autorenew, contentDescription = null) },
            enabled = enabled,
            onClick = { onRenew(days) },
        )
    }
}

/** Kept separate so the destructive action is visually and structurally distinct. */
@Composable
private fun DeleteMenuItem(enabled: Boolean, onClick: () -> Unit) {
    DropdownMenuItem(
        text = { Text("Delete", color = DigitDnsTheme.colors.danger) },
        leadingIcon = {
            Icon(
                Icons.Filled.Delete,
                contentDescription = null,
                tint = DigitDnsTheme.colors.danger,
            )
        },
        enabled = enabled,
        onClick = onClick,
    )
}

/** Warn before regenerating a value that every existing config depends on. */
@Composable
private fun InvalidateDialog(
    action: ClientAction,
    onDismiss: () -> Unit,
    onConfirm: () -> Unit,
) {
    val what = if (action is ClientAction.RegenerateUuid) "UUID" else "secret"
    PanelDialog(
        onDismissRequest = onDismiss,
        solid = true,
        title = { Text("Regenerate $what?") },
        text = {
            Text(
                "The subscriber's existing configuration stops working immediately. " +
                    "Send them the new $what from the client card afterwards.",
            )
        },
        confirmButton = {
            TextButton(onClick = onConfirm) {
                Text("Regenerate", color = DigitDnsTheme.colors.warning)
            }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

/** The only action that asks for confirmation before it runs. */
@Composable
private fun DeleteDialog(onDismiss: () -> Unit, onConfirm: () -> Unit) {
    PanelDialog(
        onDismissRequest = onDismiss,
        solid = true,
        title = { Text("Delete this client?") },
        text = { Text("Their subscription link and whitelisted address stop working immediately. This cannot be undone.") },
        confirmButton = {
            TextButton(onClick = onConfirm) {
                Text("Delete", color = DigitDnsTheme.colors.danger)
            }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}
