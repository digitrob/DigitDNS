package com.digitrob.digitdns.ui.rules

import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.positionInRoot
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.UploadFile
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
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
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogWindowProvider
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.IntSize
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.core.network.ResolutionProfileDto
import com.digitrob.digitdns.core.network.ResolutionSettings
import com.digitrob.digitdns.ui.components.DigitDnsDropdownMenu
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.dashboard.resolverExcludedByTransport
import com.digitrob.digitdns.ui.dashboard.resolverLooksEncrypted
import com.digitrob.digitdns.ui.dashboard.transportChoices
import com.digitrob.digitdns.ui.components.PanelRowDefaults
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

    var resolutionDomain by remember { mutableStateOf<String?>(null) }

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
                    // 8 dp keeps the rows related without the 12 dp band that used to
                    // separate cards this short.
                    verticalArrangement = Arrangement.spacedBy(8.dp),
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
                            hasResolution = state.resolution?.profiles?.containsKey(row.domain) == true,
                            onResolution = { viewModel.loadResolution(); resolutionDomain = row.domain },
                        )
                    }

                    item { PinnedRecordsCard(state.lists.records, viewModel::addRecord, viewModel::removeRecord) }

                    item { Spacer(Modifier.height(16.dp)) }
                }
            }
        }
    }

    ResolutionDialogHost(
        domain = resolutionDomain,
        state = state,
        onRetry = viewModel::loadResolution,
        onDismiss = { resolutionDomain = null },
        onSave = { domain, profile ->
            viewModel.saveResolution(domain, profile)
            resolutionDomain = null
        },
    )
}

/** Opens the override dialog for the tapped row, if any. */
@Composable
private fun ResolutionDialogHost(
    domain: String?,
    state: RulesUiState,
    onRetry: () -> Unit,
    onDismiss: () -> Unit,
    onSave: (String, ResolutionProfileDto?) -> Unit,
) {
    if (domain == null) return
    ResolutionDialog(
        domain = domain,
        settings = state.resolution,
        upstreams = state.resolution?.upstreams?.takeIf { it.isNotEmpty() }
            ?: state.resolutionUpstreams,
        error = state.resolutionError,
        onRetry = onRetry,
        onDismiss = onDismiss,
        onSave = { profile -> onSave(domain, profile) },
    )
}

/** The editable fields of a resolution override, held as Compose state. */
private class ResolutionDraft(profile: ResolutionProfileDto?) {
    private val pinned = profile?.upstream.orEmpty()
    var upstream by mutableStateOf(pinned)
    var strategy by mutableStateOf(profile?.strategy ?: "")

    // The transport follows the resolver: a stored rule that pins one resolver
    // and carries a conflicting transport is normalised to the resolver's type on
    // open rather than presenting a pair the pool cannot honour.
    var transport by mutableStateOf(
        if (pinned.isNotEmpty()) {
            if (resolverLooksEncrypted(pinned)) "encrypted" else "regular"
        } else {
            profile?.transport ?: "inherit"
        }
    )
    var fallback by mutableStateOf(profile?.fallback ?: "pool")

    fun toProfile() = ResolutionProfileDto(
        upstream = upstream,
        strategy = strategy,
        transport = if (transport == "inherit") "" else transport,
        fallback = if (fallback == "fail") "fail" else "",
    )
}

@Composable
private fun ResolutionDialog(
    domain: String,
    settings: ResolutionSettings?,
    upstreams: List<String>,
    error: String?,
    onRetry: () -> Unit,
    onDismiss: () -> Unit,
    onSave: (ResolutionProfileDto?) -> Unit,
) {
    val base = settings ?: ResolutionSettings()
    // Rebuild the draft when the snapshot arrives: reopening the dialog after a
    // failed load must edit the saved rule, not an empty defaults object.
    val draft = remember(domain, settings) { ResolutionDraft(base.profiles[domain]) }

    Dialog(onDismissRequest = onDismiss) {
        // The page behind the dialog stays visible: this dialog is a panel, not a
        // modal blackout, so the operator can keep reading the list it edits.
        val dialogView = LocalView.current
        LaunchedEffect(dialogView) {
            (dialogView.parent as? DialogWindowProvider)?.window?.setDimAmount(0f)
        }
        PanelCard(
            modifier = Modifier.fillMaxWidth(),
            contentPadding = 12.dp,
            verticalPadding = 12.dp,
            solid = true,
        ) {
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(
                    text = "Override Resolving",
                    style = MaterialTheme.typography.titleMedium,
                )
                Text(
                    text = domain,
                    style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                Text(
                    text = "Overrides the global defaults for this proxied domain. It applies " +
                        "when the name is not being spoofed (paused) or for query types other " +
                        "than A/AAAA.",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                ResolutionFields(base, draft, upstreams)
                if (settings == null) {
                    // The picker can still show the live pool, but a save built from
                    // an empty snapshot would overwrite the operator's profiles.
                    Text(
                        text = error ?: "Resolution settings are still loading",
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.End,
                ) {
                    if (settings == null) {
                        TextButton(onClick = onRetry) { Text("Retry") }
                    }
                    TextButton(onClick = onDismiss) { Text("Cancel") }
                    TextButton(
                        enabled = settings != null,
                        onClick = {
                            val profile = draft.toProfile()
                            onSave(if (profile == ResolutionProfileDto()) null else profile)
                        },
                    ) { Text("Save") }
                }
            }
        }
    }
}

/** The upstream that matches a chosen transport, keeping the current one if it fits. */
private fun upstreamForTransport(
    choice: String,
    upstreams: List<String>,
    current: String,
): String = when (choice) {
    "encrypted" -> current.takeIf { resolverLooksEncrypted(it) }
        ?: upstreams.firstOrNull { resolverLooksEncrypted(it) }.orEmpty()
    "regular" -> current.takeIf { it.isNotEmpty() && !resolverLooksEncrypted(it) }
        ?: upstreams.firstOrNull { !resolverLooksEncrypted(it) }.orEmpty()
    else -> current
}

/** The four override rows, with wording that matches the web dialog. */
@Composable
private fun ResolutionFields(
    base: ResolutionSettings,
    draft: ResolutionDraft,
    upstreams: List<String>,
) {
    val encrypted = base.encryptedUpstreams.toSet()
        .ifEmpty { upstreams.filter { resolverLooksEncrypted(it) }.toSet() }
    val hasEncrypted = base.encryptedAvailable || encrypted.isNotEmpty()
    val hasRegular = upstreams.any { !resolverLooksEncrypted(it) }
    // Keep a saved pin visible even if it was removed from the pool; the pool
    // will ignore it until it is added back.
    val choices = (upstreams + base.upstreams).distinct().let {
        if (draft.upstream.isNotEmpty() && draft.upstream !in it) it + draft.upstream else it
    }
    DialogPicker(
        label = "Upstream",
        value = draft.upstream,
        choices = listOf("" to "Inherit Global") + choices.map { it to it },
        onSelect = { picked ->
            draft.upstream = picked
            if (picked.isNotEmpty()) {
                draft.transport = if (resolverLooksEncrypted(picked)) "encrypted" else "regular"
            }
        },
    )
    val effectiveTransport = if (draft.transport == "inherit") base.transport else draft.transport
    if (draft.upstream.isNotEmpty() &&
        resolverExcludedByTransport(draft.upstream, effectiveTransport, encrypted)
    ) {
        Text(
            text = "The selected upstream is excluded by this transport and is ignored.",
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.tertiary,
        )
    }
    DialogPicker(
        label = "Strategy",
        value = draft.strategy,
        choices = listOf(
            "" to "Inherit Global",
            "fastest" to "Fastest Wins",
            "sequential" to "In Order",
        ),
        onSelect = { draft.strategy = it },
    )
    DialogPicker(
        label = "Transports",
        value = draft.transport,
        choices = listOf("inherit" to "Inherit Global") + transportChoices(),
        unavailable = buildSet {
            if (!hasEncrypted) add("encrypted")
            if (choices.isNotEmpty() && !hasRegular) add("regular")
        },
        onSelect = { choice ->
            draft.transport = choice
            // Choosing a transport selects a resolver that can actually use it;
            // choosing a resolver above selects its transport. The two fields can
            // no longer be saved in a contradictory state.
            draft.upstream = upstreamForTransport(choice, choices, draft.upstream)
        },
    )
    DialogPicker(
        label = "If That Fails",
        value = draft.fallback,
        choices = listOf(
            "pool" to "Fall Back to the Pool",
            "fail" to "Failure",
        ),
        onSelect = { draft.fallback = it },
    )
}

@Composable
private fun DialogPicker(
    label: String,
    value: String,
    choices: List<Pair<String, String>>,
    onSelect: (String) -> Unit,
    unavailable: Set<String> = emptySet(),
) {
    var expanded by remember { mutableStateOf(false) }
    val selected = choices.firstOrNull { it.first == value }?.second ?: choices.first().second
    Row(verticalAlignment = Alignment.CenterVertically) {
        Text(
            text = label,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.weight(1f),
        )
        Box {
            OutlinedButton(
                onClick = { expanded = true },
                contentPadding = PaddingValues(horizontal = 8.dp, vertical = 2.dp),
            ) {
                Text(selected, style = MaterialTheme.typography.labelMedium, maxLines = 1)
                Icon(Icons.Filled.ArrowDropDown, contentDescription = null, modifier = Modifier.size(16.dp))
            }
            DigitDnsDropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
                choices.forEach { (key, text) ->
                    DropdownMenuItem(
                        text = { Text(text, style = MaterialTheme.typography.bodySmall) },
                        enabled = key !in unavailable,
                        trailingIcon = {
                            if (key == value) Icon(Icons.Filled.Check, contentDescription = null, modifier = Modifier.size(16.dp))
                        },
                        onClick = { expanded = false; onSelect(key) },
                    )
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
    var expanded by remember { mutableStateOf(false) }
    Box {
        OutlinedButton(onClick = { expanded = true }) {
            Text(selected.label, maxLines = 1)
            Icon(Icons.Filled.ArrowDropDown, contentDescription = "Sort")
        }
        DigitDnsDropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
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
    hasResolution: Boolean,
    onResolution: () -> Unit,
) {
    var menuOpen by remember { mutableStateOf(false) }

    SwipeToDeleteRow(
        onDelete = onRemove,
        confirmTitle = "Remove ${row.domain}?",
        confirmText = "The domain is removed from the routing lists.",
        confirmLabel = "Remove",
    ) {
        var rowSize by remember { mutableStateOf(IntSize.Zero) }
        var rowY by remember { mutableStateOf(0f) }
        Box(modifier = Modifier.onGloballyPositioned { coords -> rowSize = coords.size; rowY = coords.positionInRoot().y }) {
            PanelCard(
                // Every routing row is the same height as a live-log row, so the two
                // lists read at the same density. The switch is the tallest content
                // here (32 dp); the shared floor plus centred content absorbs the rest.
                modifier = Modifier.fillMaxWidth().heightIn(min = PanelRowDefaults.height),
                verticalPadding = 8.dp,
                contentAlignment = Alignment.CenterStart,
                highlighted = menuOpen,
            ) {
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
                    // Only a proxied row can own a resolution override: those are the
                    // names the server accepts a rule for. Blocked/direct rows have
                    // their own action menu and no override.
                    if (row.status == DomainStatus.PROXY || row.status == DomainStatus.PAUSED) {
                        Spacer(Modifier.width(2.dp))
                        // 40 dp keeps the whole glyph tappable without changing the
                        // 56 dp row; the larger icon and the extra gap to the status
                        // pill stop it reading as part of the switch group.
                        IconButton(onClick = onResolution, modifier = Modifier.size(40.dp)) {
                            Icon(
                                imageVector = Icons.Filled.Settings,
                                contentDescription = "Override Resolving",
                                tint = if (hasResolution) MaterialTheme.colorScheme.primary
                                else MaterialTheme.colorScheme.onSurfaceVariant,
                                modifier = Modifier.size(20.dp),
                            )
                        }
                        Spacer(Modifier.width(10.dp))
                    }
                    Box {
                        DomainStatusPill(
                            status = row.status,
                            onClick = { menuOpen = true },
                        )
                    }
                    Spacer(Modifier.width(6.dp))
                    Switch(checked = row.enabled, onCheckedChange = { onToggle() })
                }
            }
            RouteActionMenu(
                domain = row.domain,
                expanded = menuOpen,
                onDismiss = { menuOpen = false },
                onAction = onAction,
                anchorSize = rowSize,
                anchorY = rowY,
            )
        }
    }
}

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
