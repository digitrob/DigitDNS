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
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.domain.model.Policy
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelDialog
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.components.clickableText
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

/**
 * The raw policy store. Most of what appears here is the rule lists the rules screen
 * already edits, so this view is informational; the one write it offers is storing an
 * extra named entry by key.
 */
@Composable
fun PoliciesScreen(viewModel: PoliciesViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()

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
            isRefreshing = state.isRefreshing,
            onRefresh = viewModel::refresh,
            modifier = Modifier.fillMaxSize(),
        ) {
            when {
                state.isLoading && state.policies.isEmpty() -> CenterBox {
                    CircularProgressIndicator(color = MaterialTheme.colorScheme.primary)
                }
                state.policies.isEmpty() -> CenterBox {
                    Text("No stored policies.", color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                else -> LazyColumn(
                    modifier = Modifier.fillMaxSize(),
                    contentPadding = PaddingValues(12.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    items(state.policies, key = { it.key }) { policy ->
                        PolicyRow(
                            policy = policy,
                            busy = state.pendingKey == policy.key,
                            onToggle = { viewModel.toggleEnabled(policy) },
                            onEdit = { viewModel.openEdit(policy) },
                        )
                    }
                }
            }
        }
    }

    if (state.editorOpen) {
        PolicyEditor(viewModel)
    }
}

@Composable
private fun PolicyRow(
    policy: Policy,
    busy: Boolean,
    onToggle: () -> Unit,
    onEdit: () -> Unit,
) {
    PanelCard(modifier = Modifier.fillMaxWidth().clickableText(onEdit)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                text = friendlyPolicyName(policy),
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.onSurface,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f),
            )
            Text(
                text = "${policy.customDomains.size}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            StatusBadge(
                text = if (policy.enabled) "On" else "Off",
                color = if (policy.enabled) DigitDnsTheme.colors.success else DigitDnsTheme.colors.textMuted,
                modifier = Modifier.padding(start = 8.dp),
            )
            Switch(
                checked = policy.enabled,
                onCheckedChange = { onToggle() },
                enabled = !busy,
                modifier = Modifier.padding(start = 8.dp),
            )
        }
    }
}

/** A readable label for a policy: its display name, else its key without the `custom_` prefix. */
private fun friendlyPolicyName(policy: Policy): String =
    policy.name.ifBlank { policy.key.removePrefix("custom_").replace('_', ' ') }

@Composable
private fun PolicyEditor(viewModel: PoliciesViewModel) {
    val state by viewModel.state.collectAsStateWithLifecycle()

    PanelDialog(
        onDismissRequest = viewModel::closeEditor,
        title = { Text(if (state.editingExisting) "Edit policy" else "New policy") },
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
                    value = state.draftKey,
                    onValueChange = viewModel::onKeyChange,
                    label = { Text("Key") },
                    singleLine = true,
                    enabled = !state.editingExisting,
                    modifier = Modifier.fillMaxWidth(),
                )
                OutlinedTextField(
                    value = state.draftName,
                    onValueChange = viewModel::onNameChange,
                    label = { Text("Display name (optional)") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
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

@Composable
private fun CenterBox(content: @Composable () -> Unit) {
    Column(
        modifier = Modifier.fillMaxSize(),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) { content() }
}
