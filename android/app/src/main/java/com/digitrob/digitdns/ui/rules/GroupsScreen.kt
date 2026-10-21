package com.digitrob.digitdns.ui.rules

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.domain.model.CustomGroup
import com.digitrob.digitdns.domain.model.GroupAction
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelDialog
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.components.SwipeToDeleteRow
import com.digitrob.digitdns.ui.components.clickableText
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

/**
 * The custom groups list: named domain sets, each with an action and an on/off state.
 * Create and edit happen in a dialog because the form is four fields and a phone
 * benefits from the modal focus.
 */
@Composable
fun GroupsScreen(viewModel: GroupsViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    var pendingDelete by remember { mutableStateOf<CustomGroup?>(null) }

    Column(modifier = Modifier.fillMaxSize()) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.End,
        ) {
            TextButton(onClick = { viewModel.openCreate() }) {
                Icon(Icons.Filled.Add, contentDescription = null, modifier = Modifier.size(18.dp))
                Spacer(Modifier.width(6.dp))
                Text("New")
            }
        }

        state.error?.let {
            Text(
                it,
                color = DigitDnsTheme.colors.danger,
                style = MaterialTheme.typography.bodySmall,
                modifier = Modifier.padding(horizontal = 12.dp),
            )
        }

        PanelRefreshBox(
            isRefreshing = state.isLoading && state.groups.isNotEmpty(),
            onRefresh = viewModel::refresh,
            modifier = Modifier.fillMaxSize(),
        ) {
            when {
                state.isLoading && state.groups.isEmpty() -> CenterBox {
                    CircularProgressIndicator(color = MaterialTheme.colorScheme.primary)
                }
                state.groups.isEmpty() -> CenterBox {
                    Text("No custom groups yet.", color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                else -> LazyColumn(
                    modifier = Modifier.fillMaxSize(),
                    contentPadding = PaddingValues(12.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    items(state.groups, key = { it.id }) { group ->
                        GroupRow(
                            group = group,
                            onClick = { viewModel.openEdit(group) },
                            onDelete = { pendingDelete = group },
                        )
                    }
                }
            }
        }
    }

    if (state.editorOpen) {
        GroupEditor(viewModel)
    }

    pendingDelete?.let { group ->
        PanelDialog(
            onDismissRequest = { pendingDelete = null },
            title = { Text("Delete \"${group.name}\"?") },
            text = { Text("The ${group.domains.size} domain(s) in this group stop being ${group.action.wire}ed immediately.") },
            confirmButton = {
                TextButton(onClick = {
                    viewModel.delete(group.id)
                    pendingDelete = null
                }) { Text("Delete", color = DigitDnsTheme.colors.danger) }
            },
            dismissButton = { TextButton(onClick = { pendingDelete = null }) { Text("Cancel") } },
        )
    }
}

@Composable
private fun GroupRow(group: CustomGroup, onClick: () -> Unit, onDelete: () -> Unit) {
    val colors = DigitDnsTheme.colors
    SwipeToDeleteRow(onDelete = onDelete) {
    PanelCard(modifier = Modifier.fillMaxWidth().clickableText(onClick)) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    text = group.name,
                    style = MaterialTheme.typography.titleMedium,
                    color = MaterialTheme.colorScheme.onSurface,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f),
                )
                Spacer(Modifier.width(8.dp))
                StatusBadge(text = group.action.wire, color = actionColor(group.action, colors))
                Spacer(Modifier.width(6.dp))
                StatusBadge(
                    text = if (group.enabled) "On" else "Off",
                    color = if (group.enabled) colors.success else colors.textMuted,
                )
            }
            Text(
                text = group.domains.joinToString(", "),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
    }
}

@Composable
private fun GroupEditor(viewModel: GroupsViewModel) {
    val state by viewModel.state.collectAsStateWithLifecycle()

    PanelDialog(
        onDismissRequest = viewModel::closeEditor,
        title = { Text(if (state.editingId == null) "New group" else "Edit group") },
        solid = true,
        text = {
            Column(
                modifier = Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(10.dp),
            ) {
                state.draftError?.let {
                    Text(it, color = DigitDnsTheme.colors.danger, style = MaterialTheme.typography.bodySmall)
                }
                OutlinedTextField(
                    value = state.draftName,
                    onValueChange = viewModel::onNameChange,
                    label = { Text("Name") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    GroupAction.entries.forEach { action ->
                        FilterChip(
                            selected = state.draftAction == action,
                            onClick = { viewModel.onActionChange(action) },
                            label = { Text(action.wire) },
                        )
                    }
                }
                OutlinedTextField(
                    value = state.draftDomains,
                    onValueChange = viewModel::onDomainsChange,
                    label = { Text("Domains (one per line)") },
                    minLines = 3,
                    modifier = Modifier.fillMaxWidth(),
                )
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("Enabled", modifier = Modifier.weight(1f))
                    Switch(checked = state.draftEnabled, onCheckedChange = viewModel::onEnabledChange)
                }
            }
        },
        confirmButton = {
            TextButton(onClick = viewModel::saveDraft, enabled = !state.isSaving) {
                Text(if (state.isSaving) "Saving…" else "Save")
            }
        },
        dismissButton = { TextButton(onClick = viewModel::closeEditor) { Text("Cancel") } },
    )
}

private fun actionColor(
    action: GroupAction,
    colors: com.digitrob.digitdns.ui.theme.ExtendedColors,
) = when (action) {
    GroupAction.Proxy -> colors.info
    GroupAction.Direct -> colors.success
    GroupAction.Block -> colors.danger
}

@Composable
private fun CenterBox(content: @Composable () -> Unit) {
    Column(
        modifier = Modifier.fillMaxSize(),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) { content() }
}
