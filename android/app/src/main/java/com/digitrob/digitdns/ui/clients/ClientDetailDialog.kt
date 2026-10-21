package com.digitrob.digitdns.ui.clients

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Autorenew
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Key
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.RestartAlt
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
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
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.domain.model.ClientAction
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelDialog
import com.digitrob.digitdns.ui.components.PanelWideDialog
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

/**
 * Edit one client, shown as a popup over the client list.
 *
 * The list stays where it was: opening a client no longer pushes a page over it, so
 * the operator can close the popup and carry on with the row they were reading. The
 * form is the same one the new-client popup uses, and the action tray rides below it
 * — everything the client page had, on one card.
 */
@Composable
fun ClientDetailDialog(
    clientId: String,
    onDismiss: () -> Unit,
    onChanged: () -> Unit,
    viewModel: ClientDetailViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()

    // The host names the client and has already reset the view model before this
    // popup mounted, so the state on screen is already for this client.

    // Whether the state on screen belongs to this client at all. Reopening the popup
    // and switching clients both start from the previous client's state, so anything
    // that shows or acts on it has to wait until the load replaces it.
    val loadedForThisClient = (state.mode as? ClientDetailMode.Edit)?.id == clientId

    // Saving or deleting finishes the job: the popup closes and the list behind it
    // refreshes. The guard keeps a previous client's flag from closing a fresh popup.
    LaunchedEffect(state.saved, state.deleted, loadedForThisClient) {
        if (loadedForThisClient && (state.saved || state.deleted)) onChanged()
    }

    PanelWideDialog(
        onDismissRequest = onDismiss,
        title = {
            Text(
                text = if (loadedForThisClient) {
                    state.client?.name?.ifBlank { null } ?: "Client"
                } else {
                    "Client"
                },
                style = MaterialTheme.typography.titleMedium,
                color = MaterialTheme.colorScheme.onSurface,
            )
        },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                if (!loadedForThisClient || state.isLoading) {
                    CircularProgressIndicator(color = MaterialTheme.colorScheme.primary)
                } else {
                    state.error?.let {
                        Text(it, color = DigitDnsTheme.colors.danger, style = MaterialTheme.typography.bodySmall)
                    }

                    state.message?.let { message ->
                        PanelCard {
                            Row(modifier = Modifier.fillMaxWidth()) {
                                Text(
                                    text = message,
                                    color = MaterialTheme.colorScheme.primary,
                                    modifier = Modifier.weight(1f),
                                )
                                TextButton(onClick = viewModel::dismissMessage) { Text("OK") }
                            }
                        }
                    }

                    ClientForm(state = state, viewModel = viewModel)

                    if (!state.isCreate && state.client != null) {
                        ActionTray(
                            onAction = viewModel::runAction,
                            onDelete = viewModel::delete,
                        )
                    }
                }
            }
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
}

/** The editable fields. Split out so the popup body stays readable. */
@Composable
internal fun ClientForm(state: ClientDetailUiState, viewModel: ClientDetailViewModel) {
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
        maxLines = 2,
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
    if (state.isCreate) {
        OutlinedTextField(
            value = state.validityDays,
            onValueChange = viewModel::onValidityDaysChange,
            label = { Text("Valid for (days)") },
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
            modifier = Modifier.fillMaxWidth(),
        )
    }
    Text(
        text = if (state.isCreate) {
            "Blank limit = unlimited, blank days never expire."
        } else {
            "Blank limit = unlimited."
        },
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    ResetCycleChips(selected = state.resetCycle, onSelect = viewModel::onResetCycleChange)
    OutlinedTextField(
        value = state.note,
        onValueChange = viewModel::onNoteChange,
        label = { Text("Note") },
        maxLines = 2,
        modifier = Modifier.fillMaxWidth(),
    )
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
        verticalArrangement = Arrangement.spacedBy(4.dp),
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

@Composable
private fun ActionTray(onAction: (ClientAction) -> Unit, onDelete: () -> Unit) {
    val colors = DigitDnsTheme.colors
    var showRenew by remember { mutableStateOf(false) }
    var renewDays by remember { mutableStateOf("30") }
    var showDelete by remember { mutableStateOf(false) }

    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            // Two tidy columns of equal-width actions rather than three ragged rows; a
            // filled red Delete sits apart at the bottom so it never reads as a peer.
            ActionButtonRow(
                left = { ActionButton("Enable", Icons.Filled.PlayArrow) { onAction(ClientAction.Toggle(true)) } },
                right = { ActionButton("Disable", Icons.Filled.Pause) { onAction(ClientAction.Toggle(false)) } },
            )
            ActionButtonRow(
                left = { ActionButton("Renew", Icons.Filled.Autorenew) { showRenew = true } },
                right = { ActionButton("Reset traffic", Icons.Filled.RestartAlt) { onAction(ClientAction.ResetTraffic) } },
            )
            ActionButtonRow(
                left = { ActionButton("New UUID", Icons.Filled.Refresh) { onAction(ClientAction.RegenerateUuid) } },
                right = { ActionButton("New secret", Icons.Filled.Key) { onAction(ClientAction.RegenerateRegisterSecret) } },
            )
            Button(
                onClick = { showDelete = true },
                modifier = Modifier.fillMaxWidth(),
                colors = ButtonDefaults.buttonColors(
                    containerColor = colors.danger,
                    contentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            ) {
                Icon(Icons.Filled.Delete, contentDescription = null, modifier = Modifier.size(18.dp))
                Spacer(Modifier.width(8.dp))
                Text("Delete client")
            }
        }
    }

    if (showRenew) {
        PanelDialog(
            onDismissRequest = { showRenew = false },
            title = { Text("Renew") },
            text = {
                OutlinedTextField(
                    value = renewDays,
                    onValueChange = { renewDays = it.filter { c -> c.isDigit() } },
                    label = { Text("Days to add") },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    renewDays.toIntOrNull()?.takeIf { it > 0 }?.let { onAction(ClientAction.Renew(it)) }
                    showRenew = false
                }) { Text("Add days") }
            },
            dismissButton = { TextButton(onClick = { showRenew = false }) { Text("Cancel") } },
        )
    }

    if (showDelete) {
        PanelDialog(
            onDismissRequest = { showDelete = false },
            title = { Text("Delete this client?") },
            text = { Text("Their subscription link and whitelisted address stop working immediately. This cannot be undone.") },
            confirmButton = {
                TextButton(onClick = {
                    onDelete()
                    showDelete = false
                }) { Text("Delete", color = colors.danger) }
            },
            dismissButton = { TextButton(onClick = { showDelete = false }) { Text("Cancel") } },
        )
    }
}

/** A two-column row of equal-width action buttons. */
@Composable
private fun ActionButtonRow(
    left: @Composable () -> Unit,
    right: @Composable () -> Unit,
) {
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        Box(modifier = Modifier.weight(1f)) { left() }
        Box(modifier = Modifier.weight(1f)) { right() }
    }
}

/** One action: a tonal button that fills its column, with a leading glyph and label. */
@Composable
private fun ActionButton(label: String, icon: ImageVector, onClick: () -> Unit) {
    FilledTonalButton(
        onClick = onClick,
        modifier = Modifier.fillMaxWidth(),
        contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 8.dp, vertical = 6.dp),
    ) {
        Icon(icon, contentDescription = null, modifier = Modifier.size(16.dp))
        Spacer(Modifier.width(6.dp))
        Text(label, maxLines = 1, overflow = TextOverflow.Ellipsis)
    }
}
