package com.digitrob.digitdns.ui.clients

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
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
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

/**
 * Create or edit one client. The two modes share the form; what differs is the
 * action tray (only an existing client has actions and a delete) and whether
 * validity is a fresh duration or an extension of the current expiry.
 */
@Composable
fun ClientDetailScreen(
    onDone: () -> Unit,
    viewModel: ClientDetailViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()

    LaunchedEffect(state.saved, state.deleted) {
        if (state.saved || state.deleted) onDone()
    }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(12.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        state.error?.let {
            Text(it, color = DigitDnsTheme.colors.danger, style = MaterialTheme.typography.bodySmall)
        }

        state.message?.let { message ->
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

        ClientForm(state = state, viewModel = viewModel)

        Button(
            onClick = viewModel::save,
            enabled = state.canSave,
            modifier = Modifier.fillMaxWidth(),
        ) {
            if (state.isSaving) {
                CircularProgressIndicator(
                    modifier = Modifier.padding(end = 8.dp),
                    strokeWidth = 2.dp,
                    color = MaterialTheme.colorScheme.onPrimary,
                )
            }
            Text("Save")
        }

        if (!state.isCreate && state.client != null) {
            ActionTray(
                onAction = viewModel::runAction,
                onDelete = viewModel::delete,
            )
        }
    }
}

/** The editable fields. Split out so the screen body stays readable. */
@Composable
private fun ClientForm(state: ClientDetailUiState, viewModel: ClientDetailViewModel) {
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
        modifier = Modifier.fillMaxWidth(),
    )
    OutlinedTextField(
        value = state.trafficLimitGb,
        onValueChange = viewModel::onTrafficLimitChange,
        label = { Text("Traffic limit (GB, blank = unlimited)") },
        singleLine = true,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
        modifier = Modifier.fillMaxWidth(),
    )
    ResetCycleChips(selected = state.resetCycle, onSelect = viewModel::onResetCycleChange)
    OutlinedTextField(
        value = state.maxDevices,
        onValueChange = viewModel::onMaxDevicesChange,
        label = { Text("Max devices (blank = unlimited)") },
        singleLine = true,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
        modifier = Modifier.fillMaxWidth(),
    )
    if (state.isCreate) {
        OutlinedTextField(
            value = state.validityDays,
            onValueChange = viewModel::onValidityDaysChange,
            label = { Text("Valid for (days, blank = never expires)") },
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
            modifier = Modifier.fillMaxWidth(),
        )
    }
    OutlinedTextField(
        value = state.note,
        onValueChange = viewModel::onNoteChange,
        label = { Text("Note") },
        modifier = Modifier.fillMaxWidth(),
    )
}

@Composable
private fun ResetCycleChips(selected: String, onSelect: (String) -> Unit) {
    val options = listOf("" to "Never", "daily" to "Daily", "weekly" to "Weekly", "monthly" to "Monthly")
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
