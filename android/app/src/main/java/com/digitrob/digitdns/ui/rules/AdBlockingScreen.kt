package com.digitrob.digitdns.ui.rules

import android.content.Context
import android.net.Uri
import android.provider.OpenableColumns
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.OpenInBrowser
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.Button
import androidx.compose.material3.CheckboxDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
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
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.core.network.FilterListDto
import com.digitrob.digitdns.core.network.RuleTestDto
import com.digitrob.digitdns.ui.formatCount
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelDialog
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.SettingsLoadingSkeleton
import com.digitrob.digitdns.ui.components.PanelSectionTitle
import com.digitrob.digitdns.ui.components.PanelSnackbarHost
import com.digitrob.digitdns.ui.components.PanelSwitch
import com.digitrob.digitdns.ui.components.showPanelToast
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/** The status pill and tick for an active list; upstream brand green, not the cyan accent. */
@Composable
private fun ActiveGreen(): Color = DigitDnsTheme.colors.success

/** The blocklist roster and policy, mirroring the web Ad Blocking page. */
@Composable
fun AdBlockingScreen(viewModel: AdBlockingViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val uriHandler = LocalUriHandler.current

    val filePicker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri == null) return@rememberLauncherForActivityResult
        scope.launch {
            val picked = readListFile(context, uri)
            if (picked == null) {
                viewModel.reportFileError()
            } else {
                viewModel.setCustomFile(picked.first, picked.second)
            }
        }
    }

    LaunchedEffect(state.message) {
        val message = state.message ?: return@LaunchedEffect
        snackbar.showPanelToast(message)
        viewModel.dismissMessage()
    }

    Scaffold(
        snackbarHost = { PanelSnackbarHost(snackbar) },
        contentWindowInsets = WindowInsets(0, 0, 0, 0),
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            PanelRefreshBox(
                isRefreshing = state.isLoading && (state.groups.isNotEmpty() || state.customLists.isNotEmpty()),
                onRefresh = viewModel::refresh,
                modifier = Modifier.fillMaxSize(),
            ) {
                Roster(
                    state = state,
                    viewModel = viewModel,
                    openUrl = { url -> runCatching { uriHandler.openUri(url) } },
                )
            }
        }
    }

    if (state.customDialogOpen) {
        CustomListDialog(
            state = state,
            viewModel = viewModel,
            chooseFile = {
                filePicker.launch(arrayOf("text/*", "application/octet-stream"))
            },
        )
    }

    state.deleteTarget?.let { target ->
        PanelDialog(
            onDismissRequest = viewModel::cancelDelete,
            title = { Text("Delete blocklist?") },
            text = { Text("Remove “${target.name}” and its downloaded rules?") },
            confirmButton = {
                TextButton(onClick = viewModel::confirmDelete) {
                    Text("Delete", color = DigitDnsTheme.colors.danger)
                }
            },
            dismissButton = {
                TextButton(onClick = viewModel::cancelDelete) { Text("Cancel") }
            },
        )
    }
}

@Composable
private fun Roster(
    state: AdBlockingUiState,
    viewModel: AdBlockingViewModel,
    openUrl: (String) -> Unit,
) {
    if (state.isLoading && state.groups.isEmpty() && state.customLists.isEmpty()) {
        SettingsLoadingSkeleton(cards = 3)
        return
    }

    // The lists actually in force come first so the operator can see what is
    // blocking without scrolling through the catalogue; enabled rows are left
    // out of their normal groups so nothing appears twice.
    val activePresets = state.groups.flatMap { it.rows }.filter { it.isEnabled }
    val activeCustom = state.customLists.filter { it.enabled }
    val inactiveCustom = state.customLists.filterNot { it.enabled }

    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(8.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        item { PolicyCard(state, viewModel) }
        item { RosterToolbar(state, viewModel) }

        if (activePresets.isNotEmpty() || activeCustom.isNotEmpty()) {
            item(key = "header-active") {
                PanelSectionTitle("Active", modifier = Modifier.padding(top = 6.dp))
            }
            items(activePresets, key = { "active-preset-${it.preset.id}" }) { row ->
                PresetRowItem(row, viewModel, openUrl)
            }
            items(activeCustom, key = { "active-custom-${it.id}" }) { list ->
                CustomRowItem(list, list.id in state.updating, viewModel, openUrl)
            }
        }

        state.groups.forEach { group ->
            val rows = group.rows.filterNot { it.isEnabled }
            if (rows.isEmpty()) return@forEach
            item(key = "header-${group.key}") {
                PanelSectionTitle(group.title, modifier = Modifier.padding(top = 6.dp))
            }
            items(rows, key = { "preset-${it.preset.id}" }) { row ->
                PresetRowItem(row, viewModel, openUrl)
            }
        }

        if (inactiveCustom.isNotEmpty()) {
            item(key = "header-custom") {
                PanelSectionTitle("Custom lists", modifier = Modifier.padding(top = 6.dp))
            }
            items(inactiveCustom, key = { "custom-${it.id}" }) { list ->
                CustomRowItem(list, list.id in state.updating, viewModel, openUrl)
            }
        }

        item {
            OutlinedButton(
                onClick = viewModel::openCustomDialog,
                modifier = Modifier.fillMaxWidth(),
            ) {
                Text("Add custom list")
            }
        }
        item {
            Text(
                text = "Preset URLs point at the upstream projects. List content is fetched at runtime " +
                    "and remains under each project's own licence; the home and file icons open the " +
                    "source page and the complete raw list.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 2.dp),
            )
        }
        item { RuleTesterCard(state, viewModel) }
    }
}

/**
 * Read-only rule probe: the same question the web panel answers. It never edits
 * the roster, so a test result cannot be mistaken for a policy change.
 */
@Composable
private fun RuleTesterCard(state: AdBlockingUiState, viewModel: AdBlockingViewModel) {
    PanelCard(modifier = Modifier.fillMaxWidth(), contentPadding = 12.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(
                "Rule tester",
                style = MaterialTheme.typography.titleSmall,
                fontWeight = FontWeight.SemiBold,
            )
            Text(
                "Check how the live rules classify a name before adding or debugging one. " +
                    "The result shows the action, the matching list and whether an exact entry " +
                    "or a parent wildcard applied.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                OutlinedTextField(
                    value = state.testDomain,
                    onValueChange = viewModel::onTestDomainChange,
                    modifier = Modifier.weight(1f),
                    singleLine = true,
                    label = { Text("Domain") },
                    placeholder = { Text("ads.example.com") },
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
                )
                Button(onClick = viewModel::testRule, enabled = !state.testLoading) {
                    if (state.testLoading) {
                        CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp)
                    } else {
                        Text("Test")
                    }
                }
            }
            state.testError?.let {
                Text(it, style = MaterialTheme.typography.bodySmall, color = DigitDnsTheme.colors.danger)
            }
            state.testResult?.let { RuleTestResultRow(it) }
        }
    }
}

@Composable
private fun RuleTestResultRow(result: RuleTestDto) {
    val actionColor = when (result.action.uppercase()) {
        "BLOCK" -> DigitDnsTheme.colors.danger
        "PROXY" -> MaterialTheme.colorScheme.primary
        else -> ActiveGreen()
    }
    val how = when {
        result.pattern.startsWith("*.") -> "parent wildcard ${result.pattern}"
        result.pattern.isNotEmpty() -> "exact entry ${result.pattern}"
        else -> "no rule matched"
    }
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Row(
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                result.domain,
                style = MaterialTheme.typography.bodySmall,
                fontFamily = PanelMono,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text("\u2192", color = MaterialTheme.colorScheme.onSurfaceVariant)
            Text(
                result.action,
                style = MaterialTheme.typography.bodyMedium,
                fontWeight = FontWeight.Bold,
                color = actionColor,
            )
            Text(result.rule.ifBlank { "Direct" }, style = MaterialTheme.typography.bodySmall)
        }
        Text(
            how,
            style = MaterialTheme.typography.bodySmall,
            fontFamily = PanelMono,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun PresetRowItem(
    row: PresetRow,
    viewModel: AdBlockingViewModel,
    openUrl: (String) -> Unit,
) {
    PresetRowCard(
        row = row,
        onToggle = { checked ->
            val installed = row.installed
            if (installed != null) {
                viewModel.toggleList(installed, checked)
            } else if (checked) {
                viewModel.addPreset(row.preset.id)
            }
        },
        onUpdate = { row.installed?.let(viewModel::updateList) },
        onDelete = { row.installed?.let(viewModel::requestDelete) },
        openUrl = openUrl,
    )
}

@Composable
private fun CustomRowItem(
    list: FilterListDto,
    updating: Boolean,
    viewModel: AdBlockingViewModel,
    openUrl: (String) -> Unit,
) {
    CustomRow(
        list = list,
        updating = updating,
        onToggle = { viewModel.toggleList(list, it) },
        onUpdate = { viewModel.updateList(list) },
        onDelete = { viewModel.requestDelete(list) },
        openUrl = openUrl,
    )
}

@Composable
@Suppress("LongMethod")
private fun PolicyCard(state: AdBlockingUiState, viewModel: AdBlockingViewModel) {
    val colors = DigitDnsTheme.colors
    PanelCard(modifier = Modifier.fillMaxWidth(), contentPadding = 12.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.Bottom) {
                Text(
                    text = formatCount(state.totalDomains),
                    style = MaterialTheme.typography.headlineSmall.copy(fontFamily = PanelMono),
                    color = MaterialTheme.colorScheme.onSurface,
                )
                Spacer(Modifier.width(5.dp))
                Text(
                    text = "unique domains",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(bottom = 3.dp),
                )
                Spacer(Modifier.weight(1f))
                Text(
                    text = "${state.activeCount}/${state.listCount} active",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(bottom = 3.dp),
                )
            }
            if (state.countsRunning) {
                LinearProgressIndicator(
                    modifier = Modifier.fillMaxWidth().height(2.dp),
                    color = colors.info,
                )
                Text(
                    text = "Counting catalogue domains…",
                    style = MaterialTheme.typography.bodySmall,
                    color = colors.warning,
                )
            }
            state.error?.let {
                Text(it, style = MaterialTheme.typography.bodySmall, color = colors.danger)
            }

            PanelSectionTitle("Ad blocking settings")
            PanelSwitch(
                label = "Enable ad blocking",
                checked = state.enabled,
                onCheckedChange = viewModel::setEnabled,
                supporting = if (state.enabled) {
                    "Blocked names are answered before a client sees a response."
                } else {
                    "Lists are kept, but no rule is applied."
                },
            )

            Row(
                modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                BlockModeChip("null_ip", "Null IP", state.blockMode, viewModel::setBlockMode)
                BlockModeChip("nxdomain", "NXDOMAIN", state.blockMode, viewModel::setBlockMode)
                BlockModeChip("refused", "REFUSED", state.blockMode, viewModel::setBlockMode)
            }

            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(
                    value = state.blockTtl,
                    onValueChange = viewModel::setBlockTtl,
                    label = { Text("Block TTL (s)") },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = Modifier.weight(1f),
                )
                OutlinedTextField(
                    value = state.updateIntervalHours,
                    onValueChange = viewModel::setUpdateInterval,
                    label = { Text("Update (hours)") },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = Modifier.weight(1f),
                )
            }

            OutlinedTextField(
                value = state.allowlist,
                onValueChange = viewModel::setAllowlist,
                label = { Text("Always allow (one per line)") },
                placeholder = { Text("example.com\n*.trusted.example") },
                minLines = 2,
                maxLines = 4,
                modifier = Modifier.fillMaxWidth().heightIn(min = 68.dp),
            )

            OutlinedTextField(
                value = state.customRules,
                onValueChange = viewModel::setCustomRules,
                label = { Text("Custom filtering rules (AdGuard syntax)") },
                placeholder = {
                    Text(
                        "||ads.example.com^\n@@||allowed.example.com^\n" +
                            "||tracking.example.com^\$important",
                    )
                },
                minLines = 4,
                maxLines = 10,
                modifier = Modifier.fillMaxWidth().heightIn(min = 112.dp),
            )
            Text(
                "AdGuard/uBlock rules; \$dnsrewrite=IP answers A/AAAA with that IP, " +
                    "and \$dnsrewrite=host returns a CNAME. Custom " +
                    "regex uses RE2 (max 64 patterns, 512 characters each). Downloaded-list " +
                    "regex, cosmetic rules, and scoped \$client, \$denyallow and " +
                    "\$badfilter modifiers are skipped.",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )

            Button(
                onClick = viewModel::saveSettings,
                enabled = !state.isSavingSettings,
                modifier = Modifier.fillMaxWidth(),
            ) {
                if (state.isSavingSettings) {
                    CircularProgressIndicator(
                        modifier = Modifier.padding(end = 8.dp).size(18.dp),
                        strokeWidth = 2.dp,
                        color = MaterialTheme.colorScheme.onPrimary,
                    )
                }
                Text("Save ad blocking settings")
            }
        }
    }
}

@Composable
private fun BlockModeChip(
    value: String,
    label: String,
    selected: String,
    onSelect: (String) -> Unit,
) {
    FilterChip(
        selected = selected == value,
        onClick = { onSelect(value) },
        label = { Text(label) },
    )
}

@Composable
private fun RosterToolbar(state: AdBlockingUiState, viewModel: AdBlockingViewModel) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        OutlinedTextField(
            value = state.search,
            onValueChange = viewModel::onSearchChange,
            label = { Text("Search blocklists") },
            leadingIcon = { Icon(Icons.Filled.Search, contentDescription = null) },
            singleLine = true,
            modifier = Modifier.weight(1f),
        )
        Spacer(Modifier.width(4.dp))
        IconButton(onClick = viewModel::refreshAll) {
            Icon(Icons.Filled.Refresh, contentDescription = "Refresh all lists")
        }
    }
}

@Composable
@Suppress("LongMethod", "CyclomaticComplexMethod")
private fun PresetRowCard(
    row: PresetRow,
    onToggle: (Boolean) -> Unit,
    onUpdate: () -> Unit,
    onDelete: () -> Unit,
    openUrl: (String) -> Unit,
) {
    val colors = DigitDnsTheme.colors
    PanelCard(modifier = Modifier.fillMaxWidth(), contentPadding = 8.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Row(verticalAlignment = Alignment.Top) {
                androidx.compose.material3.Checkbox(
                    checked = row.isEnabled || row.adding,
                    onCheckedChange = onToggle,
                    enabled = !row.adding && !row.updating,
                    colors = if (row.isInstalled && row.isEnabled) {
                        CheckboxDefaults.colors(checkedColor = ActiveGreen())
                    } else {
                        CheckboxDefaults.colors()
                    },
                )
                Spacer(Modifier.width(4.dp))
                Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    Text(
                        text = row.preset.name,
                        style = MaterialTheme.typography.titleSmall,
                        fontWeight = FontWeight.SemiBold,
                    )
                    PresetBadges(row)
                    if (!row.isInstalled) {
                        row.preset.description.takeIf { it.isNotBlank() }?.let {
                            Text(
                                text = it,
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                                maxLines = 2,
                                overflow = TextOverflow.Ellipsis,
                            )
                        }
                        row.preset.warning.takeIf { it.isNotBlank() }?.let {
                            Text(
                                text = it,
                                style = MaterialTheme.typography.bodySmall,
                                color = colors.warning,
                                maxLines = 2,
                                overflow = TextOverflow.Ellipsis,
                            )
                        }
                        val license = row.preset.license.takeIf { it.isNotBlank() }
                        Text(
                            text = listOfNotNull(
                                license?.let { "License: $it" },
                                domainLabel(row),
                            ).joinToString(" · "),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    row.installed?.let { installed ->
                        Text(
                            text = listMeta(installed),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                        installed.lastError.takeIf { it.isNotBlank() }?.let {
                            Text(it, style = MaterialTheme.typography.bodySmall, color = colors.danger)
                        }
                    }
                }
            }
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.End,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                LinkIcons(
                    homepage = if (row.isInstalled) row.installed?.homepage else row.preset.homepage,
                    file = if (row.isInstalled) row.installed?.url else row.preset.url,
                    openUrl = openUrl,
                )
                if (row.isInstalled) {
                    IconButton(onClick = onUpdate, enabled = !row.updating, modifier = Modifier.size(34.dp)) {
                        Icon(Icons.Filled.Refresh, contentDescription = "Update list", modifier = Modifier.size(17.dp))
                    }
                    IconButton(onClick = onDelete, modifier = Modifier.size(34.dp)) {
                        Icon(
                            Icons.Filled.Delete,
                            contentDescription = "Delete list",
                            tint = colors.danger,
                            modifier = Modifier.size(17.dp),
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun PresetBadges(row: PresetRow) {
    val colors = DigitDnsTheme.colors
    Row(horizontalArrangement = Arrangement.spacedBy(4.dp), verticalAlignment = Alignment.CenterVertically) {
        if (row.preset.recommended) TinyBadge("RECOMMENDED", colors.info)
        when {
            row.updating -> TinyBadge("UPDATING…", colors.warning)
            row.adding -> TinyBadge("ADDING…", colors.warning)
            row.isInstalled -> TinyBadge(
                if (row.isEnabled) "ACTIVE" else "DISABLED",
                if (row.isEnabled) ActiveGreen() else colors.warning,
            )
        }
    }
}

@Composable
private fun CustomRow(
    list: FilterListDto,
    updating: Boolean,
    onToggle: (Boolean) -> Unit,
    onUpdate: () -> Unit,
    onDelete: () -> Unit,
    openUrl: (String) -> Unit,
) {
    val colors = DigitDnsTheme.colors
    PanelCard(modifier = Modifier.fillMaxWidth(), contentPadding = 8.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Row(verticalAlignment = Alignment.Top) {
                androidx.compose.material3.Checkbox(
                    checked = list.enabled,
                    onCheckedChange = onToggle,
                    enabled = !updating,
                    colors = if (list.enabled) {
                        CheckboxDefaults.colors(checkedColor = ActiveGreen())
                    } else {
                        CheckboxDefaults.colors()
                    },
                )
                Spacer(Modifier.width(4.dp))
                Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    Text(
                        text = list.name,
                        style = MaterialTheme.typography.titleSmall,
                        fontWeight = FontWeight.SemiBold,
                    )
                    Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                        TinyBadge(
                            if (list.enabled) "ACTIVE" else "DISABLED",
                            if (list.enabled) ActiveGreen() else colors.warning,
                        )
                    }
                    Text(
                        text = list.url.ifBlank { "Uploaded file" },
                        style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                    Text(
                        text = listMeta(list),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    list.lastError.takeIf { it.isNotBlank() }?.let {
                        Text(it, style = MaterialTheme.typography.bodySmall, color = colors.danger)
                    }
                }
            }
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.End,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                LinkIcons(list.homepage, list.url, openUrl)
                IconButton(onClick = onUpdate, enabled = !updating, modifier = Modifier.size(34.dp)) {
                    Icon(Icons.Filled.Refresh, contentDescription = "Update list", modifier = Modifier.size(17.dp))
                }
                IconButton(onClick = onDelete, modifier = Modifier.size(34.dp)) {
                    Icon(
                        Icons.Filled.Delete,
                        contentDescription = "Delete list",
                        tint = colors.danger,
                        modifier = Modifier.size(17.dp),
                    )
                }
            }
        }
    }
}

@Composable
private fun LinkIcons(homepage: String?, file: String?, openUrl: (String) -> Unit) {
    if (!homepage.isNullOrBlank()) {
        IconButton(onClick = { openUrl(homepage) }, modifier = Modifier.size(34.dp)) {
            Icon(Icons.Filled.Home, contentDescription = "Open list homepage", modifier = Modifier.size(17.dp))
        }
    }
    if (!file.isNullOrBlank()) {
        IconButton(onClick = { openUrl(file) }, modifier = Modifier.size(34.dp)) {
            Icon(Icons.Filled.OpenInBrowser, contentDescription = "Open list file", modifier = Modifier.size(17.dp))
        }
    }
}

@Composable
private fun TinyBadge(text: String, color: Color) {
    Text(
        text = text,
        style = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.Medium),
        color = color,
        modifier = Modifier
            .border(1.dp, color.copy(alpha = 0.4f), RoundedCornerShape(999.dp))
            .padding(horizontal = 6.dp, vertical = 1.dp),
    )
}

@Composable
private fun CustomListDialog(
    state: AdBlockingUiState,
    viewModel: AdBlockingViewModel,
    chooseFile: () -> Unit,
) {
    val colors = DigitDnsTheme.colors
    PanelDialog(
        onDismissRequest = { if (!state.customSaving) viewModel.closeCustomDialog() },
        title = { Text("Add custom list") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(
                    value = state.customName,
                    onValueChange = viewModel::setCustomName,
                    label = { Text("Name") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                OutlinedTextField(
                    value = state.customUrl,
                    onValueChange = viewModel::setCustomUrl,
                    label = { Text("List URL") },
                    placeholder = { Text("https://example.com/list.txt") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                OutlinedTextField(
                    value = state.customHomepage,
                    onValueChange = viewModel::setCustomHomepage,
                    label = { Text("Homepage (optional)") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                Row(verticalAlignment = Alignment.CenterVertically) {
                    OutlinedButton(onClick = chooseFile, enabled = !state.customSaving) {
                        Text("Choose file")
                    }
                    Spacer(Modifier.width(8.dp))
                    if (state.customFileName.isNotBlank()) {
                        Text(
                            text = state.customFileName,
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            modifier = Modifier.weight(1f),
                        )
                        IconButton(onClick = viewModel::clearCustomFile, enabled = !state.customSaving) {
                            Icon(Icons.Filled.Close, contentDescription = "Remove selected file", modifier = Modifier.size(18.dp))
                        }
                    }
                }
                state.error?.let {
                    Text(it, style = MaterialTheme.typography.bodySmall, color = colors.danger)
                }
            }
        },
        confirmButton = {
            TextButton(onClick = viewModel::addCustomList, enabled = !state.customSaving) {
                if (state.customSaving) {
                    CircularProgressIndicator(
                        modifier = Modifier.padding(end = 6.dp).size(16.dp),
                        strokeWidth = 2.dp,
                    )
                }
                Text("Add list")
            }
        },
        dismissButton = {
            TextButton(onClick = viewModel::closeCustomDialog, enabled = !state.customSaving) {
                Text("Cancel")
            }
        },
    )
}

private fun domainLabel(row: PresetRow): String = when {
    row.domainCount != null -> "Domains: ${formatCount(row.domainCount)}"
    row.counting -> "Domains: counting…"
    row.countUnavailable -> "Domains: unavailable"
    else -> "Domains: —"
}

private fun listMeta(list: FilterListDto): String {
    val parts = mutableListOf(
        "${formatCount(list.domainCount)} domains",
        "${formatCount(list.allowedCount)} allow",
        "${formatCount(list.skipped)} skipped",
    )
    list.license.takeIf { it.isNotBlank() }?.let { parts.add(it) }
    parts.add("updated ${formatUpdated(list.lastUpdated)}")
    return parts.joinToString(" · ")
}

private fun formatUpdated(raw: String): String = runCatching {
    val instant = Instant.parse(raw)
    if (instant.epochSecond <= 0) return@runCatching "never"
    DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM)
        .withZone(ZoneId.systemDefault())
        .format(instant)
}.getOrDefault("never")

private const val MAX_UPLOAD_BYTES = 16 * 1024 * 1024

private suspend fun readListFile(context: Context, uri: Uri): Pair<String, String>? =
    withContext(Dispatchers.IO) {
        runCatching {
            var name = "uploaded-list.txt"
            var size = -1L
            context.contentResolver.query(uri, null, null, null, null)?.use { cursor ->
                if (cursor.moveToFirst()) {
                    val nameColumn = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
                    if (nameColumn >= 0) cursor.getString(nameColumn)?.let { name = it }
                    val sizeColumn = cursor.getColumnIndex(OpenableColumns.SIZE)
                    if (sizeColumn >= 0 && !cursor.isNull(sizeColumn)) size = cursor.getLong(sizeColumn)
                }
            }
            if (size > MAX_UPLOAD_BYTES) return@runCatching null
            val bytes = context.contentResolver.openInputStream(uri)?.use { it.readBytes() }
                ?: return@runCatching null
            if (bytes.size > MAX_UPLOAD_BYTES) return@runCatching null
            name to bytes.toString(Charsets.UTF_8)
        }.getOrNull()
    }
