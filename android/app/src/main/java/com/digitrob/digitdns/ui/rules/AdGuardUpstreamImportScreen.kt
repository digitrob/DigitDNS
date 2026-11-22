@file:Suppress(
    "LongMethod",
    "CyclomaticComplexMethod",
    "LongParameterList",
    "TooManyFunctions",
)

package com.digitrob.digitdns.ui.rules

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
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Checkbox
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHostState
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
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.positionInRoot
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.core.network.AdGuardPreviewDto
import com.digitrob.digitdns.core.network.AdGuardRowDto
import com.digitrob.digitdns.core.network.AdGuardSourceDto
import com.digitrob.digitdns.ui.components.DigitDnsDropdownMenu
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelDialog
import com.digitrob.digitdns.ui.components.PanelRowDefaults
import com.digitrob.digitdns.ui.components.PanelSnackbarHost
import com.digitrob.digitdns.ui.components.RouteAction
import com.digitrob.digitdns.ui.components.RouteActionMenu
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.components.showPanelToast
import com.digitrob.digitdns.ui.stream.queryActionStyle
import com.digitrob.digitdns.ui.theme.AdGuardImportPalette
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono

/**
 * The SettingsHub detail screen for the add-only AdGuard importer.
 *
 * The SettingsHub already names the section, so this screen starts with the one-line
 * description and the review status instead of repeating the title. The layout follows
 * the Domains and Live Log pages: one [LazyColumn] owns the vertical scroll, every row
 * is a [PanelCard] of the shared row height, and controls wrap instead of forcing a
 * sideways scroll.
 *
 * Review rows no longer carry a second status pill. The bucket is shown by tinting the
 * row (green covered, amber conflict) and is exposed through semantics, so the only
 * chip in the row is the route the operator can change.
 */
@Composable
fun AdGuardUpstreamImportScreen(viewModel: AdGuardUpstreamImportViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }

    LaunchedEffect(state.message) {
        val message = state.message ?: return@LaunchedEffect
        snackbar.showPanelToast(message)
        viewModel.dismissMessage()
    }

    if (state.deleteConfirmation) {
        DeleteListDialog(viewModel)
    }

    Scaffold(
        snackbarHost = { PanelSnackbarHost(snackbar) },
        contentWindowInsets = WindowInsets(0),
    ) { padding ->
        LazyColumn(
            modifier = Modifier.fillMaxSize().padding(padding).padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
            contentPadding = PaddingValues(vertical = 8.dp),
        ) {
            item { IntroRow(state) }
            state.error?.let { error ->
                item {
                    Text(
                        text = error,
                        style = MaterialTheme.typography.bodySmall,
                        color = DigitDnsTheme.colors.danger,
                    )
                }
            }

            item { SourceCard(state, viewModel) }

            state.preview?.let { preview ->
                val visibleRows = filterAdGuardRows(
                    preview.rows,
                    state.search,
                    state.filter,
                    state.selectedImport,
                    state.recentlyUnchecked,
                )
                item { SummaryRow(preview) }
                item { BulkRouteRow(state, viewModel) }
                item { ReviewControls(visibleRows, state, viewModel) }
                item {
                    Text(
                        text = "${state.selectedImport.size} Domains",
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                if (visibleRows.isEmpty()) {
                    item { EmptyReview(preview) }
                } else {
                    items(visibleRows, key = { row -> row.domain }) { row ->
                        ReviewRow(row, state, viewModel)
                    }
                }
            }

            item { ApplyBar(state, viewModel) }
        }
    }
}

@Composable
private fun IntroRow(state: AdGuardImportUiState) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.Top,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(
            text = "Paste an AdGuard upstream list. DigitDNS compares it with the " +
                "Domains list and shows every route change before anything is applied.",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.weight(1f),
        )
        StatusBadge(
            text = state.status,
            color = AdGuardImportPalette.statusColor(state.status),
            uppercase = false,
        )
    }
}

@Composable
private fun SourceCard(state: AdGuardImportUiState, viewModel: AdGuardUpstreamImportViewModel) {
    val selected = state.sources.find { it.id == state.selectedSourceId }
    val focusManager = LocalFocusManager.current
    val colors = DigitDnsTheme.colors
    PanelCard(modifier = Modifier.fillMaxWidth(), contentPadding = 16.dp, verticalPadding = 12.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(
                text = "Saved Lists",
                style = MaterialTheme.typography.labelLarge,
            )
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Box(modifier = Modifier.weight(1f)) {
                    OutlinedButton(
                        onClick = viewModel::toggleSourceMenu,
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(
                            text = selected?.name?.ifBlank { selected.id } ?: "New source",
                            style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            modifier = Modifier.weight(1f),
                        )
                        Spacer(Modifier.width(4.dp))
                        Icon(
                            imageVector = Icons.Filled.ArrowDropDown,
                            contentDescription = null,
                            modifier = Modifier.size(18.dp),
                        )
                    }
                    DigitDnsDropdownMenu(
                        expanded = state.sourceMenuOpen,
                        onDismissRequest = viewModel::dismissSourceMenu,
                    ) {
                        DropdownMenuItem(
                            text = {
                                SourceOptionText("New source", "Paste a fresh AdGuard list")
                            },
                            onClick = { viewModel.loadSource("") },
                        )
                        state.sources.forEach { source ->
                            DropdownMenuItem(
                                text = {
                                    SourceOptionText(source.name.ifBlank { source.id }, sourceMeta(source))
                                },
                                onClick = { viewModel.loadSource(source.id) },
                            )
                        }
                    }
                }
                IconButton(
                    onClick = viewModel::requestDelete,
                    enabled = state.selectedSourceId.isNotBlank() && !state.saving,
                ) {
                    Icon(
                        imageVector = Icons.Filled.Delete,
                        contentDescription = "Delete saved list",
                        tint = if (state.selectedSourceId.isNotBlank() && !state.saving) {
                            colors.danger
                        } else {
                            colors.textMuted
                        },
                    )
                }
            }

            OutlinedTextField(
                value = state.displayName,
                onValueChange = viewModel::setDisplayName,
                modifier = Modifier.fillMaxWidth(),
                label = { Text("List Name") },
                placeholder = { Text("Leave blank for AdGuard Home — date and time") },
                singleLine = true,
                textStyle = MaterialTheme.typography.bodySmall,
            )
            OutlinedTextField(
                value = state.text,
                onValueChange = viewModel::setText,
                modifier = Modifier.fillMaxWidth(),
                label = { Text("Pasted AdGuard upstream config") },
                placeholder = { Text("[/example.com/]https://host:8443/dns-query") },
                minLines = 12,
                maxLines = 12,
                textStyle = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
            )

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                OutlinedButton(
                    onClick = viewModel::saveSource,
                    enabled = state.text.isNotBlank() && !state.saving,
                    modifier = Modifier.weight(1f),
                ) {
                    Text(if (state.saving) "Saving…" else "Save List", maxLines = 1)
                }
                Button(
                    onClick = {
                        // A focused 12-line field keeps the IME window in charge of touch,
                        // so the review list below cannot be scrolled until focus leaves it.
                        focusManager.clearFocus()
                        viewModel.preview(resetSelection = true)
                    },
                    enabled = state.text.isNotBlank() && !state.previewing,
                    modifier = Modifier.weight(1f),
                ) {
                    Text(if (state.previewing) "Previewing…" else "Preview", maxLines = 1)
                }
            }
        }
    }
}

@Composable
private fun SourceOptionText(name: String, meta: String) {
    Column {
        Text(
            text = name,
            style = MaterialTheme.typography.bodySmall,
            fontWeight = FontWeight.SemiBold,
        )
        Text(
            text = meta,
            style = MaterialTheme.typography.labelSmall.copy(fontFamily = PanelMono),
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

private fun sourceMeta(source: AdGuardSourceDto): String {
    val whenText = source.updatedAt.ifBlank { source.lastAppliedAt }
        .take(SOURCE_TIME_CHARS).replace('T', ' ')
    return buildList {
        val count = adGuardSourceDomainCount(source)
        if (count > 0) {
            add("$count domains")
        } else if (source.text.isNotBlank()) {
            add("${source.text.length} chars")
        } else {
            add("no text")
        }
        if (whenText.isNotBlank()) add(whenText)
        if (source.lastChecksum.isNotBlank()) add(source.lastChecksum.take(CHECKSUM_CHARS))
    }.joinToString(" · ")
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun SummaryRow(preview: AdGuardPreviewDto) {
    FlowRow(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(6.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        StatusBadge(
            text = "${preview.counts.adguardOnly} to DigitDNS",
            color = AdGuardImportPalette.bucketColor("adguardOnly"),
            uppercase = false,
        )
        StatusBadge(
            text = "${preview.counts.covered} already covered",
            color = AdGuardImportPalette.bucketColor("covered"),
            uppercase = false,
        )
        StatusBadge(
            text = "${preview.counts.conflict} conflict",
            color = AdGuardImportPalette.bucketColor("conflict"),
            uppercase = false,
        )
    }
}

/**
 * Search, the bulk select/reset checkbox and the two list filters. The checkbox acts
 * on the importable rows currently visible, which is the only bulk-selection control:
 * the separate Select all / Clear imports buttons duplicated it.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun ReviewControls(
    visibleRows: List<AdGuardRowDto>,
    state: AdGuardImportUiState,
    viewModel: AdGuardUpstreamImportViewModel,
) {
    val importable = visibleRows.filter { it.bucket == "adguardOnly" }
    val allSelected = importable.isNotEmpty() &&
        importable.all { it.domain in state.selectedImport }

    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        OutlinedTextField(
            value = state.search,
            onValueChange = viewModel::setSearch,
            modifier = Modifier.fillMaxWidth(),
            placeholder = { Text("Search domains...") },
            leadingIcon = { Icon(Icons.Filled.Search, contentDescription = null) },
            singleLine = true,
            textStyle = MaterialTheme.typography.bodySmall,
        )
        FlowRow(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(6.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Checkbox(
                checked = allSelected,
                onCheckedChange = {
                    viewModel.toggleVisibleImports(importable.map { it.domain })
                },
                modifier = Modifier.size(36.dp).align(Alignment.CenterVertically),
            )
            FilterChip(
                selected = state.filter == AdGuardFilter.SelectedOnly,
                onClick = { viewModel.setFilter(AdGuardFilter.SelectedOnly) },
                label = { Text("Selected Only") },
            )
            FilterChip(
                selected = state.filter == AdGuardFilter.AlreadyPresent,
                onClick = { viewModel.setFilter(AdGuardFilter.AlreadyPresent) },
                label = { Text("Already in DigitDNS") },
            )
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun BulkRouteRow(state: AdGuardImportUiState, viewModel: AdGuardUpstreamImportViewModel) {
    val counts = adGuardStagedCounts(state)
    val staged = buildList {
        if (counts.routeChanges > 0) add("${counts.routeChanges} route changes")
    }.joinToString(" · ").ifBlank { "" }

    Column(
        modifier = Modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        FlowRow(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(6.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Text(
                text = "Apply Route",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.align(Alignment.CenterVertically),
            )
            listOf(
                ADGUARD_ACTION_PROXY to "Proxy",
                ADGUARD_ACTION_DIRECT to "Direct",
                ADGUARD_ACTION_BLOCK to "Block",
            ).forEach { (action, label) ->
                FilterChip(
                    selected = false,
                    onClick = { viewModel.setBulkRoute(action) },
                    label = { Text(label) },
                )
            }
        }
        if (staged.isNotBlank()) {
            Text(
                text = staged,
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun EmptyReview(preview: AdGuardPreviewDto) {
    PanelCard(modifier = Modifier.fillMaxWidth(), verticalPadding = 12.dp) {
        Text(
            text = if (preview.rows.isEmpty()) {
                "No active AdGuard upstream groups were found in this list."
            } else {
                "No domains match this filter."
            },
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun ReviewRow(
    row: AdGuardRowDto,
    state: AdGuardImportUiState,
    viewModel: AdGuardUpstreamImportViewModel,
) {
    var menuOpen by remember { mutableStateOf(false) }
    var rowSize by remember { mutableStateOf(IntSize.Zero) }
    var rowY by remember { mutableStateOf(0f) }
    val importable = row.bucket == "adguardOnly"
    val action = adGuardRowAction(row, state)
    val target = if (row.targetIsSelf) {
        "this DigitDNS server"
    } else {
        row.target.ifBlank { "unknown upstream" }
    }
    val upstream = "${row.group.ifBlank { "Ungrouped" }} · $target"
    val routePillWidth = rememberAdGuardPillWidth(ADGUARD_ROUTE_PILL_LABELS)
    val accent = when (row.bucket) {
        "covered" -> DigitDnsTheme.colors.success
        "conflict" -> DigitDnsTheme.colors.warning
        else -> null
    }

    Box(modifier = Modifier.onGloballyPositioned { coords -> rowSize = coords.size; rowY = coords.positionInRoot().y }) {
        PanelCard(
            modifier = Modifier
                .fillMaxWidth()
                .heightIn(min = PanelRowDefaults.height)
                .semantics { stateDescription = bucketLabel(row.bucket) },
            verticalPadding = 8.dp,
            contentAlignment = Alignment.CenterStart,
            highlighted = menuOpen || row.domain in state.selectedImport,
            accent = accent,
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Box(Modifier.size(36.dp), contentAlignment = Alignment.Center) {
                    if (importable) {
                        Checkbox(
                            checked = row.domain in state.selectedImport,
                            onCheckedChange = { viewModel.toggleImport(row.domain) },
                            modifier = Modifier.size(36.dp),
                        )
                    }
                }
                Column(
                    modifier = Modifier.weight(1f),
                    verticalArrangement = Arrangement.spacedBy(2.dp),
                ) {
                    Text(
                        text = row.domain,
                        style = MaterialTheme.typography.bodyMedium.copy(fontFamily = PanelMono),
                        color = MaterialTheme.colorScheme.onSurface,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                    Text(
                        text = upstream,
                        style = MaterialTheme.typography.labelSmall.copy(fontFamily = PanelMono),
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
                StatusBadge(
                    text = queryActionStyle(action).label,
                    color = AdGuardImportPalette.routeColor(action),
                    modifier = Modifier
                        .width(routePillWidth)
                        .clickable { menuOpen = true },
                    uppercase = false,
                )
            }
        }
        RouteActionMenu(
            domain = row.domain,
            expanded = menuOpen,
            onDismiss = { menuOpen = false },
            onAction = { routeAction, targets ->
                viewModel.setRoute(row.domain, routeAction.apiValue, targets)
            },
            // Selecting a scope stages the row's current action on the new patterns,
            // exactly as the web importer does with the same shared menu.
            onScopeChange = { scope, _ ->
                viewModel.setRoute(
                    row.domain,
                    action,
                    adGuardScopeTargets(row.domain, scope),
                )
            },
            allowRemove = false,
            patterns = state.digitDnsPatterns,
            anchorSize = rowSize,
            anchorY = rowY,
            targetResolver = { domain, scope -> adGuardScopeTargets(domain, scope) },
            disabledTargets = state.digitDnsPatterns,
        )
    }
}

/**
 * One width for every importer route pill, measured the same way Domains measures its
 * status pills. Only route labels participate now that the per-row bucket badge is
 * gone, so the pill is as narrow as the longest action it can show.
 */
@Composable
private fun rememberAdGuardPillWidth(labels: List<String>): Dp {
    val style = MaterialTheme.typography.labelSmall.copy(
        fontWeight = FontWeight.Bold,
        fontSize = 10.sp,
        lineHeight = 12.sp,
        letterSpacing = 0.5.sp,
    )
    val measurer = rememberTextMeasurer()
    val density = LocalDensity.current
    return remember(labels, style, density) {
        val widest = labels.maxOf { measurer.measure(it, style).size.width }
        with(density) { widest.toDp() } + 18.dp
    }
}

private val RouteAction.apiValue: String
    get() = when (this) {
        RouteAction.Proxy -> ADGUARD_ACTION_PROXY
        RouteAction.Block -> ADGUARD_ACTION_BLOCK
        RouteAction.Direct -> ADGUARD_ACTION_DIRECT
        RouteAction.Remove -> ADGUARD_ACTION_PROXY
    }

@Composable
private fun ApplyBar(state: AdGuardImportUiState, viewModel: AdGuardUpstreamImportViewModel) {
    PanelCard(
        modifier = Modifier.fillMaxWidth(),
        contentPadding = 16.dp,
        verticalPadding = 12.dp,
        contentAlignment = Alignment.CenterStart,
    ) {
        Column(
            modifier = Modifier.fillMaxWidth(),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Text(
                text = "Only listed domains are changed. All other DigitDNS routes stay as they are.",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Button(
                onClick = viewModel::apply,
                enabled = canApplyAdGuardSync(state),
                modifier = Modifier.align(Alignment.End),
            ) {
                Text(
                    text = if (state.applying) "Applying…" else "Add Missing Domains",
                    maxLines = 1,
                )
            }
        }
    }
}

@Composable
private fun DeleteListDialog(viewModel: AdGuardUpstreamImportViewModel) {
    PanelDialog(
        onDismissRequest = viewModel::dismissDelete,
        title = { Text("Delete AdGuard List?") },
        text = { Text("Domains already imported into DigitDNS are kept.") },
        dismissButton = {
            TextButton(onClick = viewModel::dismissDelete) { Text("Cancel") }
        },
        confirmButton = {
            Button(
                onClick = viewModel::confirmDelete,
                colors = ButtonDefaults.buttonColors(
                    containerColor = DigitDnsTheme.colors.danger,
                ),
            ) { Text("DELETE LIST") }
        },
        solid = true,
    )
}

private fun bucketLabel(bucket: String): String = when (bucket) {
    "covered" -> "Already in DigitDNS"
    "adguardOnly" -> "Add to DigitDNS"
    "conflict" -> "Conflict"
    else -> "Unchanged"
}

/** A compact saved-source metric; the server owns the authoritative parse. */
internal fun adGuardSourceDomainCount(source: AdGuardSourceDto): Int {
    val found = mutableSetOf<String>()
    val bracket = Regex("^\\[([^]]+)]")
    val port = Regex(":\\d+$")
    for (line in source.text.lineSequence()) {
        val value = line.trim()
        if (value.isEmpty() || value.startsWith("!") || value.startsWith("#")) continue
        val match = bracket.find(value)
        val tokens = if (match != null) {
            match.groupValues[1].split('/')
        } else {
            listOf(value.substringBefore(' '))
        }
        for (token in tokens) {
            val domain = token.trim()
                .removePrefix("||")
                .removePrefix("*.")
                .removeSuffix("^")
                .lowercase()
            if (domain.isEmpty() || domain.contains("://") || port.containsMatchIn(domain)) continue
            found += domain
        }
    }
    return found.size
}

private const val SOURCE_TIME_CHARS = 16
private const val CHECKSUM_CHARS = 8

private val ADGUARD_ROUTE_PILL_LABELS = listOf("Proxy", "Blocked", "Direct")
