package com.digitrob.digitdns.ui.rules

import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.UploadFile
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Switch
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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelDialog
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.PanelScreenHeader
import com.digitrob.digitdns.ui.components.PanelSnackbarHost
import com.digitrob.digitdns.ui.components.RouteAction
import com.digitrob.digitdns.ui.components.RouteActionMenu
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.components.SwipeToDeleteRow
import com.digitrob.digitdns.ui.components.showPanelToast
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono

/**
 * The Domains tab: the proxy routing list, sized for a phone.
 *
 * It shows only the proxied set — the active list and the paused one, so a rule can be
 * temporarily disabled without losing the domain. Each row carries a one-line domain and
 * an oval status pill ("Proxy" in cyan, "Paused" muted), a switch to pause/resume, and a
 * quick-action menu that moves the domain to Proxy / Block / Direct or removes it, under
 * the panel's match scope (exact / *.host / *.parent). Reload sits on the left; the live
 * connection badge on the top-right.
 */
@Composable
fun ProxiedDomainsScreen(viewModel: RulesViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }

    // The message floats over the list and needs no acknowledgement: a removal asks
    // first, and once it is done the toast only reports what happened.
    LaunchedEffect(state.savedMessage) {
        val message = state.savedMessage ?: return@LaunchedEffect
        snackbar.showPanelToast(message)
        viewModel.dismissMessage()
    }

    Scaffold(snackbarHost = { PanelSnackbarHost(snackbar) }, contentWindowInsets = WindowInsets(0)) { padding ->
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            PanelScreenHeader {
                TextButton(onClick = viewModel::load, modifier = Modifier.padding(end = 4.dp)) {
                    Icon(Icons.Filled.Refresh, contentDescription = null, modifier = Modifier.size(18.dp))
                    Spacer(Modifier.width(6.dp))
                    Text("Reload")
                }
            }

            PanelRefreshBox(
                isRefreshing = state.isRefreshing,
                onRefresh = viewModel::refresh,
                modifier = Modifier.fillMaxSize(),
            ) {
                LazyColumn(
                    modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp),
                    verticalArrangement = Arrangement.spacedBy(12.dp),
                ) {
                    state.error?.let { message ->
                        item {
                            Text(
                                message,
                                color = DigitDnsTheme.colors.danger,
                                style = MaterialTheme.typography.bodySmall,
                            )
                        }
                    }

                    item { AddRow(onAdd = viewModel::addDomain) }
                    item { SearchSortRow(state, viewModel::setSearch, viewModel::setSort) }
                    item { FilterChips(state.filter, viewModel::setFilter) }

                    if (state.rows.isEmpty() && !state.isLoading) {
                        item {
                            Text(
                                text = state.filter?.let { "No ${it.label.lowercase()} domains yet." }
                                    ?: "No domains yet.",
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                                modifier = Modifier.padding(vertical = 24.dp),
                            )
                        }
                    }

                    items(state.rows, key = { "${it.status}:${it.domain}" }) { row ->
                        DomainRowItem(
                            row = row,
                            onToggle = { viewModel.toggleDomain(row.domain, row.enabled) },
                            onRemove = { viewModel.removeDomain(row.domain) },
                            onAction = viewModel::routeAction,
                        )
                    }

                    item { PinnedRecordsCard(state.lists.records, viewModel::addRecord, viewModel::removeRecord) }

                    item { Spacer(Modifier.height(16.dp)) }
                }
            }
        }
    }
}

/** The panel's Custom Static Records: pin a hostname to a fixed IP. */
@Composable
private fun PinnedRecordsCard(
    records: Map<String, String>,
    onAdd: (String, String) -> Unit,
    onRemove: (String) -> Unit,
) {
    var domain by remember { mutableStateOf("") }
    var ip by remember { mutableStateOf("") }
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("Pinned records", style = MaterialTheme.typography.titleMedium)
            Text(
                text = "Map a hostname straight to an IP, bypassing upstreams.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                OutlinedTextField(
                    value = domain,
                    onValueChange = { domain = it },
                    label = { Text("Hostname") },
                    singleLine = true,
                    modifier = Modifier.weight(1f),
                )
                OutlinedTextField(
                    value = ip,
                    onValueChange = { ip = it },
                    label = { Text("IP") },
                    singleLine = true,
                    modifier = Modifier.weight(1f),
                )
            }
            TextButton(
                onClick = {
                    onAdd(domain, ip)
                    domain = ""
                    ip = ""
                },
                enabled = domain.isNotBlank() && ip.isNotBlank(),
            ) { Text("Pin") }
            records.forEach { (name, address) ->
                SwipeToDeleteRow(
                    onDelete = { onRemove(name) },
                    confirmTitle = "Remove $name?",
                    confirmText = "The pinned record is removed from the panel.",
                    confirmLabel = "Remove",
                ) {
                    Text(
                        text = "$name → $address",
                        style = MaterialTheme.typography.bodyMedium.copy(fontFamily = PanelMono),
                        color = MaterialTheme.colorScheme.onSurface,
                        modifier = Modifier.fillMaxWidth().padding(vertical = 8.dp),
                    )
                }
            }
        }
    }
}

/** A single domain input plus a bulk-import button (paste or file). */
@Composable
private fun AddRow(onAdd: (String) -> Unit) {
    var text by remember { mutableStateOf("") }
    var bulkOpen by remember { mutableStateOf(false) }
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        OutlinedTextField(
            value = text,
            onValueChange = { text = it },
            label = { Text("Add domain") },
            placeholder = { Text("e.g. api.target.com") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            TextButton(
                onClick = {
                    onAdd(text)
                    text = ""
                },
                enabled = text.isNotBlank(),
            ) { Text("Add") }
            OutlinedButton(onClick = { bulkOpen = true }, modifier = Modifier.weight(1f)) {
                Icon(Icons.Filled.UploadFile, contentDescription = null, modifier = Modifier.size(18.dp))
                Spacer(Modifier.width(6.dp))
                Text("Bulk import")
            }
        }
    }

    if (bulkOpen) {
        BulkImportDialog(
            onDismiss = { bulkOpen = false },
            onImport = {
                onAdd(it)
                bulkOpen = false
            },
        )
    }
}

/** Paste-or-upload dialog mirroring the panel's bulk-import modal. */
@Composable
private fun BulkImportDialog(onDismiss: () -> Unit, onImport: (String) -> Unit) {
    var body by remember { mutableStateOf("") }
    val context = LocalContext.current
    val picker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri: Uri? ->
        if (uri != null) {
            val text = runCatching {
                context.contentResolver.openInputStream(uri)?.use { it.readBytes().decodeToString() }
            }.getOrNull()
            if (!text.isNullOrBlank()) body = text
        }
    }

    PanelDialog(
        onDismissRequest = onDismiss,
        solid = true,
        title = { Text("Bulk import") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    text = "One name per line, or separated by commas, semicolons or " +
                        "spaces. Lines starting with # are ignored. A leading *. covers " +
                        "every subdomain.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                OutlinedTextField(
                    value = body,
                    onValueChange = { body = it },
                    placeholder = { Text("example.com\n*.sub.example.com") },
                    modifier = Modifier.fillMaxWidth().height(160.dp),
                )
                OutlinedButton(
                    onClick = { picker.launch(arrayOf("text/*", "text/plain", "*/*")) },
                ) {
                    Icon(Icons.Filled.UploadFile, contentDescription = null, modifier = Modifier.size(18.dp))
                    Spacer(Modifier.width(6.dp))
                    Text("Choose file")
                }
            }
        },
        confirmButton = {
            TextButton(
                onClick = { onImport(body) },
                enabled = body.isNotBlank(),
            ) { Text("Import") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@Composable
private fun SearchSortRow(
    state: RulesUiState,
    onSearch: (String) -> Unit,
    onSort: (DomainSort) -> Unit,
) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            OutlinedTextField(
                value = state.search,
                onValueChange = onSearch,
                label = { Text("Search") },
                singleLine = true,
                modifier = Modifier.weight(1f),
            )
            SortDropdown(state.sort, onSort)
        }
        Text(
            text = state.countLabel,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/** The sort selector as a dropdown button, placed to the right of the search field. */
@Composable
private fun SortDropdown(selected: DomainSort, onSelect: (DomainSort) -> Unit) {
    val colors = DigitDnsTheme.colors
    var expanded by remember { mutableStateOf(false) }
    OutlinedButton(onClick = { expanded = true }) {
        Text(selected.label, maxLines = 1)
        Icon(Icons.Filled.ArrowDropDown, contentDescription = "Sort")
    }
    DropdownMenu(
        expanded = expanded,
        onDismissRequest = { expanded = false },
        modifier = Modifier
            .background(colors.bgCard)
            .border(1.dp, colors.borderColor, RoundedCornerShape(12.dp)),
    ) {
        DomainSort.entries.forEach { sort ->
            DropdownMenuItem(
                text = { Text(sort.label) },
                trailingIcon = {
                    if (sort == selected) {
                        Icon(Icons.Filled.Check, contentDescription = null, modifier = Modifier.size(18.dp))
                    }
                },
                onClick = {
                    expanded = false
                    onSelect(sort)
                },
            )
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun FilterChips(selected: DomainFilter?, onSelect: (DomainFilter) -> Unit) {
    // The row spans the full width and the chips sit centred in it, so the filter
    // reads as one band across the page rather than a ragged strip on the left.
    FlowRow(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.CenterHorizontally),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        DomainFilter.entries.forEach { filter ->
            FilterChip(
                selected = filter == selected,
                onClick = { onSelect(filter) },
                label = { Text(filter.label) },
            )
        }
    }
}

@Composable
private fun DomainRowItem(
    row: DomainRow,
    onToggle: () -> Unit,
    onRemove: () -> Unit,
    onAction: (RouteAction, String) -> Unit,
) {
    var menuOpen by remember { mutableStateOf(false) }

    SwipeToDeleteRow(
        onDelete = onRemove,
        confirmTitle = "Remove ${row.domain}?",
        confirmText = "The domain is removed from the routing lists.",
        confirmLabel = "Remove",
    ) {
        PanelCard(modifier = Modifier.fillMaxWidth(), highlighted = menuOpen) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    text = row.domain,
                    style = MaterialTheme.typography.bodyMedium.copy(fontFamily = PanelMono),
                    color = MaterialTheme.colorScheme.onSurface,
                    maxLines = 1,
                    modifier = Modifier.weight(1f),
                )
                Spacer(Modifier.width(16.dp))
                Box {
                    DomainStatusPill(
                        status = row.status,
                        onClick = { menuOpen = true },
                    )
                    RouteActionMenu(
                        domain = row.domain,
                        expanded = menuOpen,
                        onDismiss = { menuOpen = false },
                        onAction = onAction,
                    )
                }
                Spacer(Modifier.width(16.dp))
                // Only a proxied row (active or paused) has a switch to flip. Blocked and
                // direct rows reserve the same slot as an invisible spacer, so every
                // status pill lines up in one column down the list.
                if (row.status == DomainStatus.PROXY || row.status == DomainStatus.PAUSED) {
                    Switch(checked = row.enabled, onCheckedChange = { onToggle() })
                } else {
                    Spacer(Modifier.width(SWITCH_SLOT_WIDTH))
                }
            }
        }
    }
}

/** The M3 switch's footprint, reserved on rows that do not carry one so pills align. */
private val SWITCH_SLOT_WIDTH = 52.dp

/** The row status as an oval pill; tapping it opens the routing action menu. */
@Composable
private fun DomainStatusPill(status: DomainStatus, onClick: () -> Unit) {
    val colors = DigitDnsTheme.colors
    val color = when (status) {
        DomainStatus.PROXY -> colors.info
        DomainStatus.BLOCKED -> colors.danger
        DomainStatus.PAUSED -> colors.textMuted
        DomainStatus.DIRECT -> colors.success
    }
    StatusBadge(
        text = status.label,
        color = color,
        modifier = Modifier.clickable(onClick = onClick),
    )
}
