package com.digitrob.digitdns.ui.rules

import com.digitrob.digitdns.R
import android.net.Uri
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
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
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.UploadFile
import androidx.compose.material3.Checkbox
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
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogWindowProvider
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.rememberTextMeasurer
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
    SavedMessageToast(state.savedMessage, snackbar, viewModel::dismissMessage)

    var resolutionDomain by remember { mutableStateOf<String?>(null) }
    // The row whose service picker is open. A dialog rather than a second column:
    // a phone has no room for the panel's two-pane layout.
    var servicePickerDomain by remember { mutableStateOf<String?>(null) }
    var managingServices by remember { mutableStateOf(false) }
    var bulkAssignOpen by remember { mutableStateOf(false) }
    var bulkEgressOpen by remember { mutableStateOf(false) }
    // The row whose relay chooser is open. The chip lives on the second line, so
    // opening it never reflows the gear / status / switch group.
    var relayPickerDomain by remember { mutableStateOf<String?>(null) }
    val selectedDomains = state.selectedDomains
    BackHandler(enabled = selectedDomains.isNotEmpty()) { viewModel.clearSelection() }

    Scaffold(snackbarHost = { PanelSnackbarHost(snackbar) }, contentWindowInsets = WindowInsets(0)) { padding ->
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            DomainsHeader(viewModel::load)

            DomainsList(
                state = state,
                viewModel = viewModel,
                selectedDomains = selectedDomains,
                onOpenResolution = { resolutionDomain = it },
                onOpenService = { servicePickerDomain = it },
                onOpenBulkAssign = { bulkAssignOpen = true },
                onOpenBulkEgress = { bulkEgressOpen = true },
                onOpenRelay = { relayPickerDomain = it },
                onOpenManageServices = { managingServices = true },
            )
        }
    }

    ResolutionHost(resolutionDomain, state, viewModel) { resolutionDomain = null }

    BulkRelayHost(
        open = bulkEgressOpen,
        domains = selectedDomains,
        state = state,
        onApply = { id ->
            viewModel.assignRelayToMany(selectedDomains, id)
            bulkEgressOpen = false
        },
        onDismiss = { bulkEgressOpen = false },
    )

    relayPickerDomain?.let { domain ->
        RelayPickerDialog(
            domain = domain,
            state = state,
            onPick = { id ->
                viewModel.assignRelay(domain, id)
                relayPickerDomain = null
            },
            onDismiss = { relayPickerDomain = null },
        )
    }

    BulkAssignHost(
        open = bulkAssignOpen,
        domains = selectedDomains,
        state = state,
        onApply = { id ->
            viewModel.assignServiceToMany(selectedDomains, id)
            bulkAssignOpen = false
        },
        onDismiss = { bulkAssignOpen = false },
    )

    ServicePickerHost(
        domain = servicePickerDomain,
        visible = servicePickerDomain != null || managingServices,
        state = state,
        onAssign = { target, id ->
            viewModel.assignService(target, id)
            servicePickerDomain = null
        },
        onSaveService = viewModel::saveService,
        onDeleteService = viewModel::deleteService,
        onDismiss = {
            servicePickerDomain = null
            managingServices = false
        },
    )
}


@Composable
@Suppress("LongParameterList")
private fun DomainsList(
    state: RulesUiState,
    viewModel: RulesViewModel,
    selectedDomains: Set<String>,
    onOpenResolution: (String) -> Unit,
    onOpenService: (String) -> Unit,
    onOpenBulkAssign: () -> Unit,
    onOpenBulkEgress: () -> Unit,
    onOpenRelay: (String) -> Unit,
    onOpenManageServices: () -> Unit,
) {
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
                state.error?.let { message -> item { DomainErrorLine(message) } }

                item { AddRow(viewModel::addDomain) }
                item { SearchBulkRow(state, viewModel::setSearch, viewModel::addDomain) }
                item {
                    ServicesSortRow(
                        state = state,
                        onServiceFilter = viewModel::setServiceFilter,
                        onSort = viewModel::setSort,
                        selectedCount = selectedDomains.size,
                        onAssignSelected = { onOpenBulkAssign() },
                        onAssignEgress = onOpenBulkEgress,
                        onClearSelection = viewModel::clearSelection,
                        onManageServices = { onOpenManageServices() },
                    )
                }
                item {
                    FilterChips(
                        selected = state.filter,
                        onSelect = viewModel::setFilter,
                        allSelected = state.rows.isNotEmpty() &&
                            state.rows.all { row -> row.domain in selectedDomains },
                        onToggleAll = viewModel::toggleSelectAllVisible,
                    )
                }

                if (state.rows.isEmpty() && !state.isLoading) {
                    item {
                        Text(
                            text = emptyListText(state),
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
                        onAction = { action, targets -> viewModel.routeAction(action, targets, row.domain) },
                        onScopeChange = { scope, previous -> viewModel.changeScope(row.domain, scope, previous) },
                        patterns = state.lists.managedPatterns,
                        hasResolution = state.resolution?.profiles?.containsKey(row.domain) == true,
                        onResolution = { viewModel.loadResolution(); onOpenResolution(row.domain) },
                        serviceLabel = row.serviceId.takeIf { it.isNotEmpty() }?.let(state::serviceName),
                        serviceIconUrl = state.lists.serviceIcons[row.serviceId],
                        onService = { onOpenService(row.domain) },
                        serverCountry = state.serverCountry,
                        onRelay = { onOpenRelay(row.domain) },
                        selected = row.domain in selectedDomains,
                        selectionMode = selectedDomains.isNotEmpty(),
                        onLongSelect = { viewModel.toggleSelection(row.domain) },
                        onToggleSelect = { viewModel.toggleSelection(row.domain) },
                    )
                }

                item { PinnedRecordsCard(state.lists.records, viewModel::addRecord, viewModel::removeRecord) }

                item { Spacer(Modifier.height(16.dp)) }
            }
        }}

@Composable
private fun SavedMessageToast(
    message: String?,
    snackbar: SnackbarHostState,
    onShown: () -> Unit,
) {
    LaunchedEffect(message) {
        val text = message ?: return@LaunchedEffect
        snackbar.showPanelToast(text)
        onShown()
    }
}

@Composable
private fun DomainsHeader(onReload: () -> Unit) {
    PanelScreenHeader {
        TextButton(onClick = onReload, modifier = Modifier.padding(end = 4.dp)) {
            Icon(Icons.Filled.Refresh, contentDescription = null, modifier = Modifier.size(18.dp))
            Spacer(Modifier.width(6.dp))
            Text(stringResource(R.string.domains_reload))
        }
    }
}

/** Opens the service picker for a row, if one is open. */
@Composable
private fun ServicePickerHost(
    domain: String?,
    visible: Boolean,
    state: RulesUiState,
    onAssign: (String, String) -> Unit,
    onSaveService: (String?, String, String, String) -> Unit,
    onDeleteService: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    if (!visible) return
    ServicePickerDialog(
        domain = domain,
        state = state,
        onAssign = { id -> domain?.let { onAssign(it, id) } },
        onSaveService = onSaveService,
        onDeleteService = onDeleteService,
        onDismiss = onDismiss,
    )
}

/** Opens the override dialog for the tapped row, if any. */
@Composable
private fun ResolutionHost(
    domain: String?,
    state: RulesUiState,
    viewModel: RulesViewModel,
    onDismiss: () -> Unit,
) {
    ResolutionDialogHost(
        domain = domain,
        state = state,
        onRetry = viewModel::loadResolution,
        onDismiss = onDismiss,
        onSave = { target, profile ->
            viewModel.saveResolution(target, profile)
            onDismiss()
        },
    )
}
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
    fun toProfile() = ResolutionProfileDto(
        upstream = upstream,
        strategy = strategy,
        transport = if (transport == "inherit") "" else transport,
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
                    text = stringResource(R.string.domains_override_title),
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
                    text = stringResource(R.string.domains_override_body),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                ResolutionFields(base, draft, upstreams)
                if (settings == null) {
                    // The picker can still show the live pool, but a save built from
                    // an empty snapshot would overwrite the operator's profiles.
                    Text(
                        text = error ?: stringResource(R.string.domains_resolution_loading),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.End,
                ) {
                    if (settings == null) {
                        TextButton(onClick = onRetry) { Text(stringResource(R.string.action_retry)) }
                    }
                    TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
                    TextButton(
                        enabled = settings != null,
                        onClick = {
                            val profile = draft.toProfile()
                            onSave(if (profile == ResolutionProfileDto()) null else profile)
                        },
                    ) { Text(stringResource(R.string.action_save)) }
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
        label = stringResource(R.string.domains_upstream),
        value = draft.upstream,
        choices = listOf("" to stringResource(R.string.domains_inherit_global)) + choices.map { it to it },
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
            text = stringResource(R.string.domains_transport_excluded),
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.tertiary,
        )
    }
    DialogPicker(
        label = stringResource(R.string.domains_strategy),
        value = draft.strategy,
        choices = listOf(
            "" to stringResource(R.string.domains_inherit_global),
            "fastest" to stringResource(R.string.strategy_fastest),
            "sequential" to stringResource(R.string.strategy_sequential),
            "hedged" to stringResource(R.string.strategy_hedged),
            "best" to stringResource(R.string.strategy_best),
        ),
        onSelect = { draft.strategy = it },
    )
    DialogPicker(
        label = stringResource(R.string.domains_transports),
        value = draft.transport,
        choices = listOf("inherit" to stringResource(R.string.domains_inherit_global)) + transportChoices(),
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
    Text(
        text = stringResource(R.string.domains_fallback),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
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
            Text(stringResource(R.string.domains_pinned_records), style = MaterialTheme.typography.titleMedium)
            Text(
                text = stringResource(R.string.domains_pinned_body),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                OutlinedTextField(
                    value = domain,
                    onValueChange = { domain = it },
                    label = { Text(stringResource(R.string.domains_hostname)) },
                    singleLine = true,
                    modifier = Modifier.weight(1f),
                )
                OutlinedTextField(
                    value = ip,
                    onValueChange = { ip = it },
                    label = { Text(stringResource(R.string.domains_ip)) },
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
            ) { Text(stringResource(R.string.domains_pin)) }
            records.forEach { (name, address) ->
                SwipeToDeleteRow(
                    onDelete = { onRemove(name) },
                    confirmTitle = stringResource(R.string.domains_remove_named, name),
                    confirmText = stringResource(R.string.domains_pinned_removed),
                    confirmLabel = stringResource(R.string.action_remove),
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
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        OutlinedTextField(
            value = text,
            onValueChange = { text = it },
            label = { Text(stringResource(R.string.domains_add_hint)) },
            placeholder = { Text(stringResource(R.string.domains_add_placeholder)) },
            singleLine = true,
            modifier = Modifier.weight(1f),
        )
        TextButton(
            onClick = {
                onAdd(text)
                text = ""
            },
            enabled = text.isNotBlank(),
        ) { Text(stringResource(R.string.action_add)) }
    }
}

/**
 * The service filter and Bulk import. This sits below Search because both filter
 * or add to the same list; the header is page-level (Reload, status), not a place
 * for list controls.
 */
@Composable
private fun SearchBulkRow(
    state: RulesUiState,
    onSearch: (String) -> Unit,
    onAdd: (String) -> Unit,
) {
    var bulkOpen by remember { mutableStateOf(false) }
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        OutlinedTextField(
            value = state.search,
            onValueChange = onSearch,
            label = { Text(stringResource(R.string.domains_search)) },
            singleLine = true,
            modifier = Modifier.weight(1f),
        )
        OutlinedButton(
            onClick = { bulkOpen = true },
            modifier = Modifier.width(CompactActionWidth),
            contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp),
        ) {
            Icon(Icons.Filled.UploadFile, contentDescription = null, modifier = Modifier.size(18.dp))
            Spacer(Modifier.width(6.dp))
            Text(stringResource(R.string.domains_bulk_import), maxLines = 1, overflow = TextOverflow.Ellipsis)
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
        title = { Text(stringResource(R.string.domains_bulk_import)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    text = stringResource(R.string.domains_bulk_body),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                OutlinedTextField(
                    value = body,
                    onValueChange = { body = it },
                    placeholder = { Text(stringResource(R.string.domains_bulk_placeholder)) },
                    modifier = Modifier.fillMaxWidth().height(160.dp),
                )
                OutlinedButton(
                    onClick = { picker.launch(arrayOf("text/*", "text/plain", "*/*")) },
                ) {
                    Icon(Icons.Filled.UploadFile, contentDescription = null, modifier = Modifier.size(18.dp))
                    Spacer(Modifier.width(6.dp))
                    Text(stringResource(R.string.domains_choose_file))
                }
            }
        },
        confirmButton = {
            TextButton(
                onClick = { onImport(body) },
                enabled = body.isNotBlank(),
            ) { Text(stringResource(R.string.domains_import)) }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) } },
    )
}

@Composable
@Suppress("LongParameterList")
private fun ServicesSortRow(
    state: RulesUiState,
    onServiceFilter: (String?) -> Unit,
    onSort: (DomainSort) -> Unit,
    selectedCount: Int,
    onAssignSelected: () -> Unit,
    onAssignEgress: () -> Unit,
    onClearSelection: () -> Unit,
    onManageServices: () -> Unit,
) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            // Fills the row beside the sort selector: the service name is the widest
            // thing here and was being truncated into "BBC i…".
            Box(modifier = Modifier.weight(1f)) {
                ServiceFilterDropdown(
                    state = state,
                    onSelect = onServiceFilter,
                    onManage = onManageServices,
                    modifier = Modifier.fillMaxWidth(),
                )
            }
            SortDropdown(state.sort, onSort)
        }
        // One line either way: the result count normally, the bulk controls while
        // a selection exists, so the list below never jumps.
        if (selectedCount > 0) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(4.dp),
            ) {
                Text(
                    text = stringResource(R.string.domains_selected_count, selectedCount),
                    modifier = Modifier.weight(1f),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                TextButton(onClick = onAssignSelected) {
                    Text(stringResource(R.string.domains_assign_service))
                }
                TextButton(onClick = onAssignEgress) {
                    Text(stringResource(R.string.domains_assign_egress))
                }
                TextButton(onClick = onClearSelection) {
                    Text(stringResource(R.string.action_clear))
                }
            }
        } else {
            Text(
                text = countText(state),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun DomainErrorLine(message: String) {
    Text(
        message,
        color = DigitDnsTheme.colors.danger,
        style = MaterialTheme.typography.bodySmall,
    )
}

@Composable
private fun emptyListText(state: RulesUiState): String {
    val service = state.serviceFilter
    val filter = state.filter
    return when {
        service != null -> stringResource(
            R.string.domains_empty_service,
            if (service == NO_SERVICE) stringResource(R.string.services_none) else state.serviceName(service),
        )
        filter != null -> stringResource(
            R.string.domains_empty_filtered,
            stringResource(filter.labelRes).lowercase(),
        )
        else -> stringResource(R.string.domains_empty)
    }
}

@Composable
private fun countText(state: RulesUiState): String {
    val filter = state.filter
    val base = when {
        filter != null -> stringResource(
            R.string.domains_count_filtered,
            state.rows.size,
            stringResource(filter.labelRes).lowercase(),
        )
        state.rows.size == 1 -> stringResource(R.string.domains_count_one)
        else -> stringResource(R.string.domains_count, state.rows.size)
    }
    val service = state.serviceFilter ?: return base
    val name = if (service == NO_SERVICE) stringResource(R.string.services_none) else state.serviceName(service)
    return "$base · $name"
}

/** Bulk import and the sort selector share a width so the two row-ends align. */
internal val CompactActionWidth = 150.dp

@Composable
private fun FilterChips(
    selected: DomainFilter?,
    onSelect: (DomainFilter) -> Unit,
    allSelected: Boolean,
    onToggleAll: () -> Unit,
) {
    // One row, never wrapping: the tick claims the left edge and the four chips
    // share the rest. A FlowRow wrapped to a second line on a 360 dp phone, which
    // pushed the chip band under the list. 36 dp is enough for the four labels;
    // the checkbox keeps its accessibility label without a visible "All" word.
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        val selectAllLabel = stringResource(R.string.domains_select_all)
        Checkbox(
            checked = allSelected,
            onCheckedChange = { onToggleAll() },
            modifier = Modifier
                .size(36.dp)
                .semantics { contentDescription = selectAllLabel },
        )
        Row(
            modifier = Modifier.weight(1f),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp, Alignment.CenterHorizontally),
        ) {
            DomainFilter.entries.forEach { filter ->
                FilterChip(
                    selected = filter == selected,
                    onClick = { onSelect(filter) },
                    label = { Text(stringResource(filter.labelRes)) },
                )
            }
        }
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
@Suppress("LongParameterList")
private fun DomainRowItem(
    row: DomainRow,
    onToggle: () -> Unit,
    onRemove: () -> Unit,
    onAction: (RouteAction, List<String>) -> Unit,
    onScopeChange: (MatchScope, List<String>) -> Unit,
    patterns: Set<String>,
    hasResolution: Boolean,
    onResolution: () -> Unit,
    serviceLabel: String?,
    serviceIconUrl: String?,
    onService: () -> Unit,
    serverCountry: String,
    onRelay: () -> Unit,
    selected: Boolean,
    selectionMode: Boolean,
    onLongSelect: () -> Unit,
    onToggleSelect: () -> Unit,
) {
    var menuOpen by remember { mutableStateOf(false) }

    SwipeToDeleteRow(
        onDelete = onRemove,
        confirmTitle = stringResource(R.string.domains_remove_title, row.domain),
        confirmText = stringResource(R.string.domains_remove_body),
        confirmLabel = stringResource(R.string.action_remove),
    ) {
        var rowSize by remember { mutableStateOf(IntSize.Zero) }
        var rowY by remember { mutableStateOf(0f) }
        Box(modifier = Modifier.onGloballyPositioned { coords -> rowSize = coords.size; rowY = coords.positionInRoot().y }) {
            PanelCard(
                // Every routing row is the same height as a live-log row, so the two
                // lists read at the same density. The switch is the tallest content
                // here (32 dp); the shared floor plus centred content absorbs the rest.
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(min = PanelRowDefaults.height)
                    .combinedClickable(
                        onClick = { if (selectionMode) onToggleSelect() },
                        onLongClick = onLongSelect,
                    ),
                // The Material switch already guarantees a 48 dp touch height; 5 dp
                // of padding around it lands the card at 58 dp, the live-log height.
                verticalPadding = 5.dp,
                contentAlignment = Alignment.CenterStart,
                highlighted = menuOpen || selected,
            ) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    DomainNameCell(
                        row = row,
                        serviceLabel = serviceLabel,
                        serviceIconUrl = serviceIconUrl,
                        onService = onService,
                        serverCountry = serverCountry,
                        onRelay = onRelay,
                        modifier = Modifier.weight(1f),
                    )
                    // The action set is identical on every status. Blocked/direct rows
                    // keep the gear and pause switch visible but disabled, so the row
                    // does not reflow and the unavailable actions stay legible.
                    val canAdjust = row.status == DomainStatus.PROXY || row.status == DomainStatus.PAUSED
                    Spacer(Modifier.width(2.dp))
                    DomainGearButton(
                        enabled = canAdjust,
                        highlighted = hasResolution,
                        onClick = onResolution,
                    )
                    // No spacer: the 36 dp gear keeps 6 dp of its own padding, so the
                    // glyph ends 6 dp from the pill just like the pill ends 6 dp from
                    // the switch.
                    Box {
                        DomainStatusPill(
                            status = row.status,
                            onClick = { menuOpen = true },
                        )
                    }
                    Spacer(Modifier.width(6.dp))
                    Switch(
                        checked = row.enabled,
                        onCheckedChange = { onToggle() },
                        enabled = canAdjust,
                        modifier = Modifier.alpha(if (canAdjust) 1f else 0.4f),
                    )
                }
            }
            RouteActionMenu(
                domain = row.domain,
                expanded = menuOpen,
                onDismiss = { menuOpen = false },
                onAction = onAction,
                onScopeChange = onScopeChange,
                patterns = patterns,
                anchorSize = rowSize,
                anchorY = rowY,
            )
        }
    }
}

/**
 * The domain with its second line underneath: the service label on the left, then the
 * wildcard type active for this row (or "exact" when none). The label is tappable and
 * opens the picker; tapping it never changes routing, only which group the row is in.
 * Relative to the row, so *.example.com reads as *.host here and *.parent on
 * api.example.com.
 */
@Composable
private fun DomainNameCell(
    row: DomainRow,
    serviceLabel: String?,
    serviceIconUrl: String?,
    onService: () -> Unit,
    serverCountry: String,
    onRelay: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(modifier = modifier) {
        Text(
            text = row.domain,
            style = MaterialTheme.typography.bodyMedium.copy(fontFamily = PanelMono),
            color = MaterialTheme.colorScheme.onSurface,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        Row(verticalAlignment = Alignment.CenterVertically) {
            // Service and relay are icon-only controls on the row's second line.
            // The names live in the pickers they open, so a long agent name can
            // never squeeze the wildcard text or force the row taller.
            val serviceDescription = if (serviceLabel != null) {
                stringResource(R.string.domains_service_content, serviceLabel)
            } else {
                stringResource(R.string.domains_service_none_content)
            }
            Box(
                modifier = Modifier
                    .size(26.dp)
                    .clip(RoundedCornerShape(5.dp))
                    .clickable(role = Role.Button, onClick = onService)
                    .semantics { contentDescription = serviceDescription },
                contentAlignment = Alignment.Center,
            ) {
                ServiceIcon(
                    url = serviceIconUrl,
                    modifier = Modifier.size(18.dp),
                    noService = serviceLabel == null,
                )
            }
            Text(
                text = "· " + if (row.wildcards.isEmpty()) {
                    "exact"
                } else {
                    row.wildcards.joinToString(" · ") { it.type }
                },
                style = MaterialTheme.typography.labelSmall.copy(fontFamily = PanelMono, fontSize = 10.sp),
                color = if (row.wildcards.isEmpty()) {
                    MaterialTheme.colorScheme.onSurfaceVariant
                } else {
                    DigitDnsTheme.colors.purple
                },
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            if (row.status == DomainStatus.PROXY || row.status == DomainStatus.PAUSED) {
                val relay = row.relayAgent
                Spacer(Modifier.width(2.dp))
                RelayRowIcon(
                    label = relay?.name?.ifBlank { relay.id } ?: stringResource(R.string.relay_this_server),
                    country = relay?.country ?: serverCountry,
                    onClick = onRelay,
                )
            }
        }
    }
}

/**
 * The Override Resolving gear. A 36 dp hit area with a centred 24 dp glyph: the
 * ripple is centred on the glyph, and the 6 dp of padding inside the button is the
 * same gap the status pill leaves before the switch, so no spacer is needed. The
 * hit area is dimmed when the status does not accept the action.
 */
@Composable
private fun DomainGearButton(
    enabled: Boolean,
    highlighted: Boolean,
    onClick: () -> Unit,
) {
    Box(
        modifier = Modifier
            .size(36.dp)
            .alpha(if (enabled) 1f else 0.4f)
            .clip(CircleShape)
            .clickable(enabled = enabled, role = Role.Button, onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        Icon(
            imageVector = Icons.Filled.Settings,
            contentDescription = stringResource(R.string.domains_status_pill),
            tint = if (highlighted && enabled) MaterialTheme.colorScheme.primary
            else MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(24.dp),
        )
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
    // One width for every status: the widest label at the current font scale.
    // Measuring it means the pill is as short as it can be without clipping, and
    // changing PROXY/PAUSED/BLOCKED/DIRECT never resizes it or moves the gear.
    val style = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.Medium)
    val measurer = rememberTextMeasurer()
    val density = LocalDensity.current
    val labels = DomainStatus.entries.map { stringResource(it.labelRes) }
    val pillWidth = remember(style, density, labels) {
        val widest = labels.maxOf { measurer.measure(it, style).size.width }
        with(density) { widest.toDp() } + 22.dp
    }
    StatusBadge(
        text = stringResource(status.labelRes),
        color = color,
        modifier = Modifier.width(pillWidth).clickable(onClick = onClick),
    )
}
