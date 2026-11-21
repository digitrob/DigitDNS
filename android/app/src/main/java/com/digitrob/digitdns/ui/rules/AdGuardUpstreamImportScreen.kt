package com.digitrob.digitdns.ui.rules

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Share
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.core.network.AdGuardPreviewDto
import com.digitrob.digitdns.core.network.AdGuardRowDto
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelCopyIconButton
import com.digitrob.digitdns.ui.components.PanelScreenHeader
import com.digitrob.digitdns.ui.components.PanelSectionTitle
import com.digitrob.digitdns.ui.components.PanelSnackbarHost
import com.digitrob.digitdns.ui.components.PanelSwitch
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.components.showPanelToast
import com.digitrob.digitdns.ui.theme.AdGuardImportPalette
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono

/**
 * A SettingsHub detail screen, not a navigation destination. The form, summary,
 * bulk controls, search and proposed block stay pinned above the scrolling domain
 * list: the operator can tick rows at the top of a long list without losing the
 * selection counts or the apply guardrails.
 */
@Suppress("LongMethod", "CyclomaticComplexMethod")
@Composable
fun AdGuardUpstreamImportScreen(viewModel: AdGuardUpstreamImportViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }
    val context = LocalContext.current
    val blockSaver = rememberLauncherForActivityResult(
        ActivityResultContracts.CreateDocument("text/plain"),
    ) { uri -> if (uri != null) viewModel.saveBlock(uri) }

    LaunchedEffect(state.message) {
        val message = state.message ?: return@LaunchedEffect
        snackbar.showPanelToast(message)
        viewModel.dismissMessage()
    }

    Scaffold(snackbarHost = { PanelSnackbarHost(snackbar) }) { padding ->
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            PanelScreenHeader(showBadge = false)

            Column(
                modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Text(
                    text = "AdGuard Upstream List Import",
                    style = MaterialTheme.typography.titleMedium,
                )
                Text(
                    text = "Paste an AdGuard upstream list. DigitDNS parses it, compares each domain with the " +
                        "Domains list, and shows what would change before anything is applied.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Text(
                    text = "Format: [domain1/domain2/]upstream",
                    style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                state.error?.let { error ->
                    Text(text = error, style = MaterialTheme.typography.bodySmall, color = DigitDnsTheme.colors.danger)
                }

                SourceFormCard(state, viewModel)

                state.preview?.let { preview ->
                    PreviewSummaryCard(preview, state)
                    val notices = friendlyAdGuardNotices(preview)
                    if (notices.isNotEmpty()) NoticesCard(notices)
                    BulkControlsCard(preview, state, viewModel)
                    SearchControlsCard(state, viewModel)
                    ManagedBlockCard(preview, state, viewModel, context, blockSaver)
                    ApplyCard(preview, state, viewModel)
                }
            }

            val visibleRows = state.preview?.let { preview ->
                filterAdGuardRows(
                    rows = adGuardDisplayRows(preview),
                    query = state.search,
                    selectedOnly = state.selectedOnly,
                    selectedImport = state.selectedImport,
                    selectedManaged = state.selectedManaged,
                )
            }.orEmpty()
            val visibleImportable = visibleRows.filter { it.bucket == "adguardOnly" }

            LazyColumn(
                modifier = Modifier.weight(1f).fillMaxWidth(),
                contentPadding = PaddingValues(horizontal = 12.dp, vertical = 8.dp),
                verticalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                if (state.preview == null) {
                    item {
                        Text(
                            text = "Preview a source to compare its upstream groups with the Domains list.",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                } else {
                    item(key = "visible-header") {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Checkbox(
                                checked = visibleImportable.isNotEmpty() &&
                                    visibleImportable.all { it.domain in state.selectedImport },
                                onCheckedChange = {
                                    viewModel.toggleVisibleImports(visibleImportable.map { it.domain })
                                },
                            )
                            Text(
                                text = "Select visible domains to add",
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                    if (visibleRows.isEmpty()) {
                        item {
                            Text(
                                text = "No domains match this search.",
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                    items(visibleRows, key = { it.domain }) { row ->
                        DomainRow(row, state, viewModel)
                    }
                }
            }
        }
    }
}

@Suppress("LongMethod")
@Composable
private fun SourceFormCard(state: AdGuardImportUiState, viewModel: AdGuardUpstreamImportViewModel) {
    var expanded by remember { mutableStateOf(false) }
    val selectedLabel = state.sources.find { it.id == state.selectedSourceId }?.let { source ->
        source.name.ifBlank { source.id }
    } ?: "New source…"

    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            PanelSectionTitle("Source")
            Column {
                OutlinedButton(onClick = { expanded = true }, modifier = Modifier.fillMaxWidth()) {
                    Text(selectedLabel)
                }
                DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
                    DropdownMenuItem(
                        text = { Text("New source…") },
                        onClick = { viewModel.newSource(); expanded = false },
                    )
                    state.sources.forEach { source ->
                        DropdownMenuItem(
                            text = { Text(source.name.ifBlank { source.id }) },
                            onClick = { viewModel.selectSource(source.id); expanded = false },
                        )
                    }
                }
            }
            OutlinedTextField(
                value = state.displayName,
                onValueChange = viewModel::setDisplayName,
                modifier = Modifier.fillMaxWidth(),
                label = { Text("Display name") },
                singleLine = true,
            )
            PanelSwitch(
                label = "Source enabled",
                checked = state.enabled,
                onCheckedChange = viewModel::setEnabled,
            )
            PanelSwitch(
                label = "Managed upstream for exported rules",
                checked = state.managedUpstream.isNotBlank(),
                onCheckedChange = { checked ->
                    viewModel.setManagedUpstream(if (checked) state.preview?.upstream.orEmpty() else "")
                },
                supporting = "Used to build the AdGuard block for DigitDNS-only domains.",
            )
            OutlinedTextField(
                value = state.managedUpstream,
                onValueChange = viewModel::setManagedUpstream,
                modifier = Modifier.fillMaxWidth(),
                label = { Text("Upstream address") },
                placeholder = { Text("this DigitDNS server") },
                singleLine = true,
            )
            OutlinedTextField(
                value = state.text,
                onValueChange = viewModel::setText,
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(max = 150.dp)
                    .verticalScroll(rememberScrollState()),
                label = { Text("AdGuard upstream list") },
                placeholder = { Text("[domain1/domain2/]upstream") },
                minLines = 5,
                maxLines = 8,
                textStyle = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
            )
            Row(
                modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                OutlinedButton(
                    onClick = viewModel::loadSelectedSource,
                    enabled = state.selectedSourceId.isNotBlank() && !state.previewing,
                ) { Text("Load") }
                OutlinedButton(
                    onClick = viewModel::saveSource,
                    enabled = state.text.isNotBlank() && !state.saving,
                ) { Text("Save source") }
                OutlinedButton(
                    onClick = viewModel::deleteSource,
                    enabled = state.selectedSourceId.isNotBlank() && !state.saving,
                ) { Text("Delete") }
                OutlinedButton(onClick = viewModel::newSource) { Text("New source") }
                Button(
                    onClick = { viewModel.preview(resetSelection = true) },
                    enabled = state.text.isNotBlank() && !state.previewing,
                ) { Text(if (state.previewing) "Previewing…" else "Preview") }
            }
        }
    }
}

@Composable
private fun PreviewSummaryCard(preview: AdGuardPreviewDto, state: AdGuardImportUiState) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            PanelSectionTitle("Preview summary")
            Row(
                modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                StatusBadge("${preview.counts.covered} already", AdGuardImportPalette.already)
                StatusBadge("+${preview.counts.adguardOnly} to DigitDNS", AdGuardImportPalette.add)
                StatusBadge("${preview.counts.digitdnsOnly} to AdGuard", AdGuardImportPalette.sendBack)
                StatusBadge("${preview.counts.conflict} conflicts", AdGuardImportPalette.conflict)
                if (preview.broadCount > 0 || adGuardDisplayRows(preview).any { it.broad }) {
                    StatusBadge("Broad", AdGuardImportPalette.broad)
                }
                if (friendlyAdGuardNotices(preview).any { it.kind == "external_upstream" }) {
                    StatusBadge("External upstream", AdGuardImportPalette.externalUpstream)
                }
            }
            Text(
                text = "Legend: covered · add · conflict · send back · broad · external upstream",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = "${preview.candidates} candidate rules · ${preview.groups.size} upstream groups",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (state.text != preview.text) {
                Text(
                    text = "Source text changed since the preview. Preview again before applying.",
                    style = MaterialTheme.typography.bodySmall,
                    color = DigitDnsTheme.colors.danger,
                )
            }
        }
    }
}

@Composable
private fun NoticesCard(notices: List<AdGuardNoticeUi>) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            PanelSectionTitle("Things to check")
            notices.take(3).forEach { notice ->
                Text(text = notice.title, style = MaterialTheme.typography.bodySmall, fontWeight = FontWeight.SemiBold)
                Text(
                    text = notice.message,
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (notices.size > 3) {
                Text(
                    text = "…and ${notices.size - 3} more",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun BulkControlsCard(
    preview: AdGuardPreviewDto,
    state: AdGuardImportUiState,
    viewModel: AdGuardUpstreamImportViewModel,
) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            PanelSectionTitle("Bulk selection")
            Row(
                modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                OutlinedButton(onClick = viewModel::selectAllToAdd) { Text("Select all to add") }
                OutlinedButton(onClick = viewModel::clearImports) { Text("Clear imports") }
                OutlinedButton(onClick = viewModel::selectAllToSendBack) { Text("Select all to send back") }
                OutlinedButton(onClick = viewModel::clearExports) { Text("Clear exports") }
            }
            Text(
                text = "${importableDomains(preview).size} available to add · ${preview.managed.size} available to send back",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = "${state.selectedImport.size} selected to add · ${state.selectedManaged.size} selected to send back",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (preview.managed.isNotEmpty()) {
                Row(
                    modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(6.dp),
                ) {
                    preview.managed.forEach { row ->
                        OutlinedButton(
                            onClick = { viewModel.toggleManaged(row.domain) },
                        ) {
                            Text(
                                text = if (row.domain in state.selectedManaged) "✓ ${row.domain}" else row.domain,
                                style = MaterialTheme.typography.labelSmall.copy(fontFamily = PanelMono),
                            )
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun SearchControlsCard(state: AdGuardImportUiState, viewModel: AdGuardUpstreamImportViewModel) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            OutlinedTextField(
                value = state.search,
                onValueChange = viewModel::setSearch,
                modifier = Modifier.fillMaxWidth(),
                label = { Text("Search domains") },
                placeholder = { Text("Domain, group or target") },
                singleLine = true,
            )
            PanelSwitch(
                label = "Show selected only",
                checked = state.selectedOnly,
                onCheckedChange = viewModel::setSelectedOnly,
            )
        }
    }
}

@Suppress("LongParameterList")
@Composable
private fun ManagedBlockCard(
    preview: AdGuardPreviewDto,
    state: AdGuardImportUiState,
    viewModel: AdGuardUpstreamImportViewModel,
    context: Context,
    blockSaver: androidx.activity.result.ActivityResultLauncher<String>,
) {
    val block = state.result?.generatedBlock?.takeIf { it.isNotBlank() }
        ?: preview.generatedBlock.takeIf { it.isNotBlank() }.orEmpty()
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            PanelSectionTitle("Managed block for AdGuard")
            Text(
                text = block.ifBlank { "No DigitDNS-only domains selected." },
                style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(max = 90.dp)
                    .verticalScroll(rememberScrollState()),
            )
            Row(verticalAlignment = Alignment.CenterVertically) {
                PanelCopyIconButton(
                    contentDescription = "Copy managed block",
                    enabled = block.isNotBlank(),
                    onCopy = {
                        val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                        clipboard.setPrimaryClip(ClipData.newPlainText("AdGuard block", block))
                        viewModel.blockCopied()
                    },
                )
                IconButton(
                    enabled = block.isNotBlank(),
                    onClick = {
                        val intent = Intent(Intent.ACTION_SEND).apply {
                            type = "text/plain"
                            putExtra(Intent.EXTRA_TEXT, block)
                        }
                        context.startActivity(Intent.createChooser(intent, "Share AdGuard block"))
                    },
                ) {
                    Icon(Icons.Filled.Share, contentDescription = "Share managed block")
                }
                OutlinedButton(
                    onClick = { blockSaver.launch("digitdns-managed-adguard.txt") },
                    enabled = block.isNotBlank() && !state.saving,
                ) { Text(if (state.saving) "Saving…" else "Save block") }
            }
        }
    }
}

@Composable
private fun ApplyCard(
    preview: AdGuardPreviewDto,
    state: AdGuardImportUiState,
    viewModel: AdGuardUpstreamImportViewModel,
) {
    val source = state.sources.find { it.id == state.selectedSourceId }
    val removals = state.result?.removed ?: estimatedRemovals(source, preview, state.removalMode)
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            PanelSectionTitle("Apply changes")
            Text(
                text = "+${state.selectedImport.size} to DigitDNS · ${state.selectedManaged.size} to AdGuard · $removals removals",
                style = MaterialTheme.typography.bodySmall,
                fontWeight = FontWeight.SemiBold,
            )
            Row(
                modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                OutlinedButton(
                    onClick = { viewModel.setRemovalMode(ADGUARD_REMOVAL_KEEP) },
                    enabled = state.removalMode != ADGUARD_REMOVAL_KEEP,
                ) { Text("Keep") }
                OutlinedButton(
                    onClick = { viewModel.setRemovalMode(ADGUARD_REMOVAL_SYNCED_ONLY) },
                    enabled = state.removalMode != ADGUARD_REMOVAL_SYNCED_ONLY,
                ) { Text("Synced only") }
                OutlinedButton(
                    onClick = { viewModel.setRemovalMode(ADGUARD_REMOVAL_MIRROR) },
                    enabled = state.removalMode != ADGUARD_REMOVAL_MIRROR,
                ) { Text("Mirror") }
            }
            Text(
                text = removalModeHelp(state.removalMode),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (state.removalMode == ADGUARD_REMOVAL_MIRROR) {
                PanelSwitch(
                    label = "I understand mirror removes unmanaged domains",
                    checked = state.confirmMirror,
                    onCheckedChange = viewModel::setConfirmMirror,
                )
            }
            if (requiresAcknowledgement(preview)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Checkbox(
                        checked = state.acknowledgeRisk,
                        onCheckedChange = viewModel::setAcknowledgeRisk,
                    )
                    Text(
                        text = "Acknowledge conflicts and broad domains",
                        style = MaterialTheme.typography.bodySmall,
                    )
                }
            }
            Button(
                onClick = viewModel::apply,
                enabled = canApplyAdGuardSync(state),
                modifier = Modifier.fillMaxWidth(),
            ) { Text(if (state.applying) "Applying…" else "Apply sync") }
        }
    }
}

@Composable
private fun DomainRow(
    row: AdGuardRowDto,
    state: AdGuardImportUiState,
    viewModel: AdGuardUpstreamImportViewModel,
) {
    val importable = row.bucket == "adguardOnly"
    val exportable = row.bucket == "digitdnsOnly"
    val selected = row.domain in state.selectedImport || row.domain in state.selectedManaged
    PanelCard(modifier = Modifier.fillMaxWidth(), contentPadding = 10.dp, verticalPadding = 8.dp) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            if (importable || exportable) {
                Checkbox(
                    checked = selected,
                    onCheckedChange = {
                        if (importable) viewModel.toggleImport(row.domain) else viewModel.toggleManaged(row.domain)
                    },
                )
            } else {
                Spacer(Modifier.width(34.dp))
            }
            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text = row.domain,
                    style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
                    fontWeight = FontWeight.SemiBold,
                )
                Text(
                    text = "${row.group.ifBlank { "Ungrouped" }} · ${targetLabel(row)}",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                val note = rowNote(row)
                if (note.isNotBlank()) {
                    Text(
                        text = note,
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            Column(horizontalAlignment = Alignment.End) {
                StatusBadge(bucketLabel(row.bucket), AdGuardImportPalette.forBucket(row.bucket))
                if (row.broad) Spacer(Modifier.size(3.dp))
                if (row.broad) StatusBadge("Broad", AdGuardImportPalette.broad)
                if (!row.targetIsSelf && row.target.isNotBlank()) {
                    Spacer(Modifier.size(3.dp))
                    StatusBadge("External", AdGuardImportPalette.externalUpstream)
                }
            }
        }
    }
}

private fun bucketLabel(bucket: String): String = when (bucket) {
    "covered" -> "Already in DigitDNS"
    "adguardOnly" -> "Add to DigitDNS"
    "digitdnsOnly" -> "Send back"
    "conflict" -> "Conflict"
    else -> bucket.ifBlank { "Unknown" }
}

private fun targetLabel(row: AdGuardRowDto): String = when {
    row.targetIsSelf -> "this DigitDNS server"
    row.target.isBlank() -> "unknown upstream"
    else -> row.target
}

/** Friendly, structured notes only; raw warning strings are intentionally not shown. */
private fun rowNote(row: AdGuardRowDto): String = when (row.bucket) {
    "covered" -> if (row.coveredBy.isBlank()) "Already in DigitDNS" else "Covered by ${row.coveredBy}"
    "conflict" -> if (row.coveredBy.isBlank()) "Conflicting rule" else "Conflicts with ${row.coveredBy}"
    "adguardOnly" -> "Proposed for custom_proxied"
    "digitdnsOnly" -> "Proposed for the AdGuard block"
    else -> ""
}

private fun removalModeHelp(mode: String): String = when (mode) {
    ADGUARD_REMOVAL_MIRROR -> "Mirror makes this source authoritative: DigitDNS-only domains not named here are removed."
    ADGUARD_REMOVAL_SYNCED_ONLY -> "Synced only removes domains this source imported previously but no longer names."
    else -> "Keep leaves every existing DigitDNS domain in place."
}
