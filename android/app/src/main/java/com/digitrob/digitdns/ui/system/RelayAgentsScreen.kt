package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Autorenew
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Speed
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenuItem
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
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.R
import com.digitrob.digitdns.core.network.EgressSaveRequest
import com.digitrob.digitdns.core.network.RelayModeDto
import com.digitrob.digitdns.core.network.RelayModeTestResponse
import com.digitrob.digitdns.domain.model.EgressInfo
import com.digitrob.digitdns.domain.model.RelayAgentInfo
import com.digitrob.digitdns.domain.model.egressTrafficLabel
import com.digitrob.digitdns.domain.model.relayRelativeAge
import com.digitrob.digitdns.ui.components.CountryFlag
import com.digitrob.digitdns.ui.components.DigitDnsDropdownMenu
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelCopyIconButton
import com.digitrob.digitdns.ui.components.PanelDialog
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.PanelToggle
import com.digitrob.digitdns.ui.components.SettingsLoadingSkeleton
import com.digitrob.digitdns.ui.components.PanelSnackbarHost
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.components.showPanelToast
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import java.util.Locale

/**
 * Settings → Relay Agents: the agent catalogue, shaped like a client card.
 *
 * The grid is adaptive: one full-width card on a phone, two side by side when the
 * window is wide enough for the IP/key rows to stay readable. Two cards fit in the
 * phone's first screen height, which is the density the panel's two-column card grid
 * gives on a desktop.
 */
@Composable
@Suppress("LongMethod", "CyclomaticComplexMethod")
fun RelayAgentsScreen(viewModel: RelayAgentsViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }
    val clipboard = LocalClipboardManager.current
    val scope = rememberCoroutineScope()

    var creating by remember { mutableStateOf(false) }
    var editing by remember { mutableStateOf<RelayAgentInfo?>(null) }
    var deleting by remember { mutableStateOf<RelayAgentInfo?>(null) }
    var creatingEgressKind by remember { mutableStateOf<String?>(null) }
    var editingEgress by remember { mutableStateOf<EgressInfo?>(null) }
    var deletingEgress by remember { mutableStateOf<EgressInfo?>(null) }
    val onlineCount = state.agents.count { it.isOnline() }

    LaunchedEffect(state.message) {
        val message = state.message ?: return@LaunchedEffect
        snackbar.showPanelToast(message)
        viewModel.dismissMessage()
    }
    LaunchedEffect(state.clipboardText) {
        val text = state.clipboardText ?: return@LaunchedEffect
        clipboard.setText(AnnotatedString(text))
        viewModel.consumeClipboard()
    }
    // Egress latency/health and agent traffic age on their own, exactly like the
    // web cards' 10-second poll. Pull-to-refresh stays for an immediate read.
    LaunchedEffect(Unit) {
        while (isActive) {
            delay(10_000)
            viewModel.autoRefresh()
        }
    }

    // The Settings hub already lays this screen out below the status bar and above
    // the bottom navigation. Applying the default scaffold insets here again left a
    // large dead band under the chips and another above the nav bar.
    Scaffold(
        snackbarHost = { PanelSnackbarHost(snackbar) },
        contentWindowInsets = WindowInsets(0),
    ) { padding ->
        // The header lives inside the grid on purpose. Pull-to-refresh is a nested
        // scroll gesture: a header outside the scrollable surface is dead space, and
        // on this short page that dead space is exactly where an operator pulls. One
        // scroll container from the Add button down means every downward drag reaches
        // the refresh indicator.
        PanelRefreshBox(
            isRefreshing = state.isRefreshing,
            onRefresh = viewModel::refresh,
            modifier = Modifier.fillMaxSize().padding(padding),
        ) {
            LazyVerticalGrid(
                columns = GridCells.Adaptive(minSize = 320.dp),
                // Only a hair of top padding: the chip row above is the header, and
                // the two cards need the full remaining height to fit without
                // scrolling on a phone.
                contentPadding = PaddingValues(start = 12.dp, top = 0.dp, end = 12.dp, bottom = 6.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
                modifier = Modifier.fillMaxSize(),
            ) {
                item(span = { GridItemSpan(maxLineSpan) }) {
                    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        EgressSummary(state.egresses)
                        if (state.agents.isNotEmpty()) {
                            Text(
                                text = stringResource(
                                    if (state.agents.size == 1) R.string.agents_summary_one else R.string.agents_summary,
                                    state.agents.size,
                                    onlineCount,
                                ),
                                style = MaterialTheme.typography.labelSmall.copy(fontFamily = PanelMono),
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                                maxLines = 1,
                                modifier = Modifier.padding(horizontal = 12.dp),
                            )
                        }
                    }
                }

                item(span = { GridItemSpan(maxLineSpan) }) {
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        RelayGlobalSwitch(
                            enabled = state.relayGlobalEnabled,
                            onChange = viewModel::setRelayGlobal,
                        )
                        state.relayHubs.forEach { hub ->
                            RelayModeCard(
                                hub = hub,
                                isNew = false,
                                saving = state.relaySavingId == hub.id,
                                testing = state.relayTestingId == hub.id,
                                testResult = state.relayTests[hub.id],
                                onSave = { name, enabled, url, key, token ->
                                    viewModel.saveRelayHub(hub.id, name, enabled, url, key, token)
                                },
                                onTest = { viewModel.testRelayHub(hub.id) },
                                onDelete = { viewModel.deleteRelayHub(hub.id) },
                            )
                        }
                        RelayModeCard(
                            hub = null,
                            isNew = true,
                            saving = state.relaySavingId == "new",
                            testing = false,
                            testResult = null,
                            onSave = { name, enabled, url, key, token ->
                                viewModel.saveRelayHub("", name, enabled, url, key, token)
                            },
                            onTest = {},
                            onDelete = {},
                        )
                    }
                }

                state.error?.let { message ->
                    item(span = { GridItemSpan(maxLineSpan) }) {
                        Row(
                            modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 2.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Text(
                                message,
                                color = DigitDnsTheme.colors.danger,
                                style = MaterialTheme.typography.bodySmall,
                                modifier = Modifier.weight(1f),
                            )
                            TextButton(onClick = viewModel::clearError) {
                                Text(stringResource(R.string.action_dismiss))
                            }
                        }
                    }
                }

                item(span = { GridItemSpan(maxLineSpan) }) {
                    EgressSectionHeader(
                        title = stringResource(R.string.egress_relay_agents_title),
                        body = stringResource(R.string.egress_relay_agents_body),
                        action = stringResource(R.string.agents_add),
                        onAdd = { creating = true },
                    )
                }

                when {
                    state.isLoading && state.agents.isEmpty() -> item(span = { GridItemSpan(maxLineSpan) }) {
                        SettingsLoadingSkeleton(cards = 2)
                    }
                    state.agents.isEmpty() -> item(span = { GridItemSpan(maxLineSpan) }) {
                        EmptyAgents()
                    }
                    else -> items(state.agents, key = { it.id }) { agent ->
                        AgentCard(
                            agent = agent,
                            assignedDomains = state.assignedCounts[agent.id] ?: 0,
                            serviceLabels = state.serviceLabels[agent.id].orEmpty(),
                            testing = state.testingId == agent.id,
                            onEdit = { editing = agent },
                            onToggle = { viewModel.setEnabled(agent.id, !agent.enabled) },
                            onDelete = { deleting = agent },
                            onCopyIp = { viewModel.copyIp(agent.publicIp) },
                            onCopyKey = { viewModel.copyKey(agent.id) },
                            onRotateKey = { viewModel.rotateKey(agent.id) },
                            onTest = { viewModel.test(agent.id) },
                        )
                    }
                }

                val socks = state.egresses.filter { !it.isWireGuard }
                item(span = { GridItemSpan(maxLineSpan) }) {
                    EgressSectionHeader(
                        title = stringResource(R.string.egress_socks5_title),
                        body = stringResource(R.string.egress_socks5_body),
                        action = stringResource(R.string.egress_add_socks5),
                        onAdd = { creatingEgressKind = "socks5" },
                    )
                }
                if (socks.isEmpty()) {
                    item(span = { GridItemSpan(maxLineSpan) }) {
                        EmptyEgress(stringResource(R.string.egress_empty_socks5))
                    }
                } else {
                    items(socks, key = { "egress-" + it.id }) { egress ->
                        EgressCard(
                            egress = egress,
                            testing = state.testingEgressId == egress.id,
                            onEdit = { editingEgress = egress },
                            onToggle = { viewModel.setEgressEnabled(egress.id, !egress.enabled) },
                            onDelete = { deletingEgress = egress },
                            onTest = { viewModel.testEgress(egress.id) },
                            onRefresh = viewModel::refresh,
                        )
                    }
                }

                val wireguard = state.egresses.filter { it.isWireGuard }
                item(span = { GridItemSpan(maxLineSpan) }) {
                    EgressSectionHeader(
                        title = stringResource(R.string.egress_wireguard_title),
                        body = stringResource(R.string.egress_wireguard_body),
                        action = stringResource(R.string.egress_add_wireguard),
                        onAdd = { creatingEgressKind = "wireguard" },
                    )
                }
                if (wireguard.isEmpty()) {
                    item(span = { GridItemSpan(maxLineSpan) }) {
                        EmptyEgress(stringResource(R.string.egress_empty_wireguard))
                    }
                } else {
                    items(wireguard, key = { "egress-" + it.id }) { egress ->
                        EgressCard(
                            egress = egress,
                            testing = state.testingEgressId == egress.id,
                            onEdit = { editingEgress = egress },
                            onToggle = { viewModel.setEgressEnabled(egress.id, !egress.enabled) },
                            onDelete = { deletingEgress = egress },
                            onTest = { viewModel.testEgress(egress.id) },
                            onRefresh = viewModel::refresh,
                        )
                    }
                }
            }
        }
    }

    if (creating) {
        AgentFormDialog(
            agent = null,
            saving = state.isSaving,
            onDismiss = { creating = false },
            onSave = { name, country, _, _, _ ->
                viewModel.create(name, country)
                creating = false
            },
        )
    }

    editing?.let { agent ->
        AgentFormDialog(
            agent = agent,
            saving = state.isSaving,
            onDismiss = { editing = null },
            onSave = { name, country, publicIp, publicIpv6, ipv6Enabled ->
                viewModel.update(agent.id, name, country, publicIp, publicIpv6, ipv6Enabled)
                editing = null
            },
        )
    }

    deleting?.let { agent ->
        PanelDialog(
            onDismissRequest = { deleting = null },
            solid = true,
            title = { Text(stringResource(R.string.agents_delete_title, agent.displayName)) },
            text = { Text(stringResource(R.string.agents_delete_body)) },
            confirmButton = {
                TextButton(onClick = {
                    viewModel.delete(agent.id)
                    deleting = null
                }) { Text(stringResource(R.string.action_delete)) }
            },
            dismissButton = {
                TextButton(onClick = { deleting = null }) { Text(stringResource(R.string.action_cancel)) }
            },
        )
    }

    creatingEgressKind?.let { kind ->
        EgressFormDialog(
            kind = kind,
            egress = null,
            saving = state.isSaving,
            onDismiss = { creatingEgressKind = null },
            onSave = { body ->
                viewModel.createEgress(body)
                creatingEgressKind = null
            },
        )
    }

    editingEgress?.let { egress ->
        EgressFormDialog(
            kind = egress.kind,
            egress = egress,
            saving = state.isSaving,
            testing = state.testingEgressId == egress.id,
            onTest = { viewModel.testEgress(egress.id) },
            onDismiss = { editingEgress = null },
            onSave = { body ->
                viewModel.updateEgress(egress.id, body)
                editingEgress = null
            },
        )
    }

    deletingEgress?.let { egress ->
        PanelDialog(
            onDismissRequest = { deletingEgress = null },
            solid = true,
            title = { Text(stringResource(R.string.egress_delete_title, egress.displayName)) },
            text = { Text(stringResource(R.string.egress_delete_body)) },
            confirmButton = {
                TextButton(onClick = {
                    viewModel.deleteEgress(egress.id)
                    deletingEgress = null
                }) { Text(stringResource(R.string.action_delete)) }
            },
            dismissButton = {
                TextButton(onClick = { deletingEgress = null }) { Text(stringResource(R.string.action_cancel)) }
            },
        )
    }

    state.revealedKey?.let { revealed ->
        PanelDialog(
            onDismissRequest = viewModel::dismissKey,
            solid = true,
            title = { Text(stringResource(R.string.agents_key_title, revealed.agentName)) },
            text = {
                Column {
                    Text(
                        stringResource(
                            if (revealed.rotated) R.string.agents_key_rotated_body else R.string.agents_key_created_body,
                        ),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Spacer(Modifier.height(8.dp))
                    Text(
                        revealed.key,
                        style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
                        color = DigitDnsTheme.colors.info,
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(6.dp))
                            .background(MaterialTheme.colorScheme.surface)
                            .padding(10.dp),
                    )
                }
            },
            confirmButton = {
                TextButton(onClick = {
                    clipboard.setText(AnnotatedString(revealed.key))
                    viewModel.dismissKey()
                    scope.launch { snackbar.showPanelToast("Agent key copied.") }
                }) { Text(stringResource(R.string.agents_copy_key)) }
            },
            dismissButton = {
                TextButton(onClick = viewModel::dismissKey) { Text(stringResource(R.string.action_done)) }
            },
        )
    }
}

@Composable
private fun RelayGlobalSwitch(enabled: Boolean, onChange: (Boolean) -> Unit) {
    PanelCard(contentPadding = 8.dp) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(modifier = Modifier.weight(1f)) {
                Text("This Server as Relay", style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.Bold)
                Text(
                    "Master switch for every hub below. Off stops all relay workers and leaves local proxying untouched.",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            PanelToggle(checked = enabled, onCheckedChange = onChange)
        }
    }
}

@Suppress("LongMethod", "CyclomaticComplexMethod", "LongParameterList")
@Composable
private fun RelayModeCard(
    hub: RelayModeDto?,
    isNew: Boolean,
    saving: Boolean,
    testing: Boolean,
    testResult: RelayModeTestResponse?,
    onSave: (String, Boolean, String, String, String) -> Unit,
    onTest: () -> Unit,
    onDelete: () -> Unit,
) {
    var name by remember(hub?.name) { mutableStateOf(hub?.name.orEmpty()) }
    var url by remember(hub?.hubUrl) { mutableStateOf(hub?.hubUrl.orEmpty()) }
    var key by remember { mutableStateOf("") }
    var token by remember { mutableStateOf("") }
    var enabled by remember(hub?.enabled) { mutableStateOf(hub?.enabled == true) }
    val status = when {
        hub == null -> "unknown"
        !hub.enabled -> "disabled"
        hub.online -> "online"
        else -> "offline"
    }
    val tone = when {
        hub?.online == true -> DigitDnsTheme.colors.success
        hub?.enabled == true -> DigitDnsTheme.colors.warning
        else -> MaterialTheme.colorScheme.onSurfaceVariant
    }
    PanelCard(contentPadding = 8.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                OutlinedTextField(
                    value = name,
                    onValueChange = { name = it },
                    label = { Text("Hub name") },
                    singleLine = true,
                    modifier = Modifier.weight(1f),
                )
                if (!isNew) {
                    Spacer(Modifier.width(6.dp))
                    StatusBadge(status, tone)
                    AgentActionButton(
                        icon = Icons.Filled.Delete,
                        label = "Remove hub",
                        onClick = onDelete,
                        tint = DigitDnsTheme.colors.danger,
                    )
                }
            }
            if (isNew) {
                Text(
                    "Let another DigitDNS instance use this server as a relay agent. " +
                        "Local proxying continues on the same 80/443 listeners; " +
                        "each hub's clients only reach that hub's approved hosts.",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            OutlinedTextField(
                value = url,
                onValueChange = { url = it },
                label = { Text("Hub URL (including the hidden admin path)") },
                placeholder = { Text("https://hub:port/<admin-path>") },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = key,
                onValueChange = { key = it },
                label = { Text("Agent key (write-only)") },
                placeholder = {
                    Text(if (hub?.agentKeySet == true) "Saved — leave blank to keep" else "id:secret from the hub")
                },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = token,
                onValueChange = { token = it },
                label = { Text("Enrolment token (first time only)") },
                placeholder = { Text("One-time token from the hub") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            ToggleRow(
                title = "Relay mode for this hub",
                subtitle = "Sync this server with the hub",
                checked = enabled,
                onCheckedChange = { enabled = it },
            )
            Text(
                text = buildString {
                    append("${hub?.hosts ?: 0} hosts · ${hub?.clients ?: 0} clients")
                    append(" · ")
                    append(egressTrafficLabel((hub?.relayBytesIn ?: 0) + (hub?.relayBytesOut ?: 0)))
                    append(" relayed")
                    if (!hub?.lastSync.isNullOrBlank()) {
                        append(" · Last sync ")
                        append(relayRelativeAge(hub.lastSync))
                    }
                },
                style = MaterialTheme.typography.labelSmall.copy(fontFamily = PanelMono),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
            hub?.lastError?.takeIf { it.isNotBlank() }?.let { message ->
                Text(message, style = MaterialTheme.typography.labelSmall, color = DigitDnsTheme.colors.warning, maxLines = 2)
            }
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedButton(
                    onClick = {
                        onSave(name.trim(), enabled, url, key, token)
                        key = ""
                        token = ""
                    },
                    enabled = !saving,
                    contentPadding = PaddingValues(horizontal = 12.dp, vertical = 0.dp),
                    modifier = Modifier.height(34.dp),
                ) {
                    Text(if (saving) "Saving…" else if (isNew) "Add hub" else "Save hub")
                }
                if (!isNew) {
                    OutlinedButton(
                        onClick = onTest,
                        enabled = !testing && !saving,
                        contentPadding = PaddingValues(horizontal = 12.dp, vertical = 0.dp),
                        modifier = Modifier.height(34.dp),
                    ) {
                        if (testing) {
                            CircularProgressIndicator(modifier = Modifier.size(14.dp), strokeWidth = 2.dp)
                            Spacer(Modifier.width(4.dp))
                        } else {
                            Icon(Icons.Filled.Speed, contentDescription = null, modifier = Modifier.size(16.dp))
                            Spacer(Modifier.width(4.dp))
                        }
                        Text(if (testing) "Testing…" else "Test connection")
                    }
                    Spacer(Modifier.weight(1f))
                    val ms = testResult?.latencyMs ?: 0.0
                    val ok = testResult?.ok == true
                    RelayTestPingBadge(ms, ok)
                }
            }
        }
    }
}

/** Same badge and thresholds as a relay agent's ping: green fast, yellow over
 * 40 ms, red over 80 ms, and a stable "ping —" before the first test. */
@Composable
private fun RelayTestPingBadge(pingMs: Double, ok: Boolean) {
    val (text, color) = when {
        !ok || pingMs <= 0 -> "ping —" to MaterialTheme.colorScheme.onSurfaceVariant
        pingMs > 80 -> String.format(Locale.US, "ping %.1f ms", pingMs) to DigitDnsTheme.colors.danger
        pingMs > 40 -> String.format(Locale.US, "ping %.1f ms", pingMs) to DigitDnsTheme.colors.warning
        else -> String.format(Locale.US, "ping %.1f ms", pingMs) to DigitDnsTheme.colors.success
    }
    StatusBadge(text, color)
}

@Composable
private fun EgressSummary(egresses: List<EgressInfo>) {
    val enabled = egresses.filter { it.enabled }
    val healthy = enabled.count { it.health == "online" }
    val failed = enabled.count { it.health == "offline" }
    val failClosed = enabled.count { it.failClosed }
    val traffic = egresses.sumOf { it.bytesIn + it.bytesOut }
    val trafficLabel = egressTrafficLabel(traffic)
    PanelCard(contentPadding = 8.dp) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            EgressSummaryItem(stringResource(R.string.egress_summary_healthy, healthy), Modifier.weight(1f))
            EgressSummaryItem(stringResource(R.string.egress_summary_failed, failed), Modifier.weight(1f))
            EgressSummaryItem(stringResource(R.string.egress_summary_traffic, trafficLabel), Modifier.weight(1f))
            EgressSummaryItem(stringResource(R.string.egress_summary_failclosed, failClosed), Modifier.weight(1f))
        }
    }
}

@Composable
private fun EgressSummaryItem(label: String, modifier: Modifier = Modifier) {
    Column(
        modifier = modifier
            .clip(RoundedCornerShape(8.dp))
            .background(MaterialTheme.colorScheme.surface)
            .padding(horizontal = 4.dp, vertical = 5.dp),
    ) {
        Text(
            label,
            style = MaterialTheme.typography.labelSmall.copy(
                fontFamily = PanelMono,
                fontWeight = FontWeight.Bold,
                fontSize = 8.sp,
                lineHeight = 10.sp,
            ),
            color = MaterialTheme.colorScheme.onSurface,
            maxLines = 1,
            softWrap = false,
            overflow = TextOverflow.Ellipsis,
        )
    }
}

@Composable
private fun EgressSectionHeader(title: String, body: String, action: String, onAdd: () -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(start = 12.dp, end = 12.dp, top = 8.dp, bottom = 2.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(title, style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.Bold)
            Text(
                body,
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
        }
        Spacer(Modifier.width(8.dp))
        TextButton(onClick = onAdd, contentPadding = PaddingValues(horizontal = 8.dp, vertical = 0.dp)) {
            Icon(Icons.Filled.Add, contentDescription = null, modifier = Modifier.size(16.dp))
            Spacer(Modifier.width(4.dp))
            Text(action, maxLines = 1)
        }
    }
}

@Composable
private fun EmptyEgress(body: String) {
    Box(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp).heightIn(min = 76.dp),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            body,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(10.dp),
        )
    }
}

@Composable
@Suppress("LongMethod")
private fun EgressCard(
    egress: EgressInfo,
    testing: Boolean,
    onEdit: () -> Unit,
    onToggle: () -> Unit,
    onDelete: () -> Unit,
    onTest: () -> Unit,
    onRefresh: () -> Unit,
) {
    val statusColor = when {
        !egress.enabled -> MaterialTheme.colorScheme.onSurfaceVariant
        egress.health == "online" -> DigitDnsTheme.colors.success
        egress.health == "offline" -> DigitDnsTheme.colors.warning
        else -> MaterialTheme.colorScheme.onSurfaceVariant
    }
    PanelCard(
        modifier = Modifier.fillMaxWidth().alpha(if (egress.enabled) 1f else 0.62f),
        contentPadding = 8.dp,
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Row(verticalAlignment = Alignment.Top) {
                CountryFlag(
                    country = egress.country,
                    width = 24.dp,
                    height = 18.dp,
                    modifier = Modifier.padding(top = 2.dp),
                )
                Spacer(Modifier.width(8.dp))
                Column(modifier = Modifier.weight(1f)) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(
                            egress.displayName,
                            style = MaterialTheme.typography.titleSmall,
                            fontWeight = FontWeight.Bold,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            modifier = Modifier.weight(1f, fill = false),
                        )
                        Spacer(Modifier.width(6.dp))
                        StatusBadge(egress.statusLabel, statusColor)
                        if (egress.failClosed) {
                            Spacer(Modifier.width(4.dp))
                            StatusBadge(
                                stringResource(R.string.egress_fail_closed_badge),
                                DigitDnsTheme.colors.warning,
                            )
                        }
                    }
                    Text(
                        text = "${egress.kindLabel} · ${egress.detail}",
                        style = MaterialTheme.typography.labelSmall.copy(fontFamily = PanelMono),
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
                Row(horizontalArrangement = Arrangement.spacedBy(1.dp)) {
                    AgentActionButton(
                        icon = Icons.Filled.Autorenew,
                        label = stringResource(R.string.egress_refresh),
                        onClick = onRefresh,
                    )
                    AgentActionButton(
                        icon = Icons.Filled.Edit,
                        label = stringResource(R.string.egress_edit),
                        onClick = onEdit,
                    )
                    AgentActionButton(
                        icon = if (egress.enabled) Icons.Filled.Pause else Icons.Filled.PlayArrow,
                        label = stringResource(if (egress.enabled) R.string.agents_disable else R.string.agents_enable),
                        onClick = onToggle,
                    )
                    AgentActionButton(
                        icon = Icons.Filled.Delete,
                        label = stringResource(R.string.egress_delete),
                        onClick = onDelete,
                        tint = DigitDnsTheme.colors.danger,
                    )
                }
            }

            Text(
                text = stringResource(
                    R.string.egress_traffic_meta,
                    egress.trafficLabel,
                    egress.lastSeenLabel,
                ),
                style = MaterialTheme.typography.labelSmall.copy(fontFamily = PanelMono),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )

            if (egress.lastError.isNotBlank()) {
                Text(
                    egress.lastError,
                    style = MaterialTheme.typography.labelSmall.copy(fontFamily = PanelMono),
                    color = DigitDnsTheme.colors.warning,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }

            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                OutlinedButton(
                    onClick = onTest,
                    enabled = !testing,
                    modifier = Modifier.height(34.dp),
                    contentPadding = PaddingValues(horizontal = 10.dp, vertical = 0.dp),
                ) {
                    if (testing) {
                        CircularProgressIndicator(modifier = Modifier.size(14.dp), strokeWidth = 2.dp)
                        Spacer(Modifier.width(6.dp))
                    } else {
                        Icon(Icons.Filled.Speed, contentDescription = null, modifier = Modifier.size(16.dp))
                        Spacer(Modifier.width(4.dp))
                    }
                    Text(stringResource(if (testing) R.string.egress_testing else R.string.egress_test), maxLines = 1)
                }
                Spacer(Modifier.weight(1f))
                EgressLatencyBadge(egress)
            }
        }
    }
}

/** Same colour scheme as relay-agent ping: green fast, yellow over 40 ms,
 * red over 80 ms. The badge replaces the three boxed latency/traffic/age
 * fields, so the card matches an agent card. */
@Composable
private fun EgressLatencyBadge(egress: EgressInfo) {
    val (text, color) = when {
        egress.latencyMs <= 0 -> stringResource(R.string.egress_latency_none) to MaterialTheme.colorScheme.onSurfaceVariant
        egress.latencyMs > 80 -> String.format(Locale.US, "ping %.1f ms", egress.latencyMs) to DigitDnsTheme.colors.danger
        egress.latencyMs > 40 -> String.format(Locale.US, "ping %.1f ms", egress.latencyMs) to DigitDnsTheme.colors.warning
        else -> String.format(Locale.US, "ping %.1f ms", egress.latencyMs) to DigitDnsTheme.colors.success
    }
    StatusBadge(text, color)
}

@Composable
@Suppress("LongMethod", "LongParameterList", "CyclomaticComplexMethod")
private fun EgressFormDialog(
    kind: String,
    egress: EgressInfo?,
    saving: Boolean,
    testing: Boolean = false,
    onTest: (() -> Unit)? = null,
    onDismiss: () -> Unit,
    onSave: (EgressSaveRequest) -> Unit,
) {
    val wireguard = kind == "wireguard"
    var name by remember(egress) { mutableStateOf(egress?.name.orEmpty()) }
    var country by remember(egress) { mutableStateOf(egress?.country.orEmpty().uppercase(Locale.US)) }
    var address by remember(egress) { mutableStateOf(egress?.address.orEmpty()) }
    var username by remember(egress) { mutableStateOf(egress?.username.orEmpty()) }
    var password by remember(egress) { mutableStateOf("") }
    var config by remember(egress) { mutableStateOf("") }
    var resolveLocal by remember(egress) { mutableStateOf(egress?.resolve == "local") }
    var failClosed by remember(egress) { mutableStateOf(egress?.failClosed ?: true) }
    var enabled by remember(egress) { mutableStateOf(egress?.enabled ?: true) }
    val editing = egress != null
    val context = androidx.compose.ui.platform.LocalContext.current
    val configPicker = androidx.activity.compose.rememberLauncherForActivityResult(
        androidx.activity.result.contract.ActivityResultContracts.OpenDocument(),
    ) { uri ->
        if (uri != null) {
            runCatching {
                context.contentResolver.openInputStream(uri)?.bufferedReader()?.use { reader ->
                    config = reader.readText().take(256 * 1024)
                }
            }
        }
    }
    val valid = name.trim().isNotEmpty() &&
        (if (wireguard) config.isNotBlank() || (editing && egress?.configSet == true) else address.trim().isNotEmpty())

    PanelDialog(
        onDismissRequest = onDismiss,
        solid = true,
        title = {
            Text(
                if (editing) {
                    stringResource(R.string.egress_edit_title, if (wireguard) "WireGuard" else "SOCKS5")
                } else {
                    stringResource(
                        if (wireguard) R.string.egress_add_wireguard_title else R.string.egress_add_socks5_title,
                    )
                },
            )
        },
        text = {
            Column(
                modifier = Modifier.heightIn(max = 460.dp).verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                OutlinedTextField(
                    value = name,
                    onValueChange = { name = it },
                    label = { Text(stringResource(R.string.egress_name)) },
                    placeholder = { Text(stringResource(R.string.egress_name_hint)) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                CountryField(selected = country, onSelect = { country = it })
                if (wireguard) {
                    OutlinedTextField(
                        value = config,
                        onValueChange = { config = it },
                        label = { Text(stringResource(R.string.egress_config)) },
                        placeholder = {
                            Text(
                                if (editing && egress?.configSet == true) {
                                    stringResource(R.string.egress_config_saved)
                                } else {
                                    stringResource(R.string.egress_config_hint)
                                },
                            )
                        },
                        minLines = 5,
                        maxLines = 9,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    OutlinedButton(
                        onClick = {
                            configPicker.launch(arrayOf("text/plain", "application/octet-stream", "*/*"))
                        },
                        modifier = Modifier.height(34.dp),
                        contentPadding = PaddingValues(horizontal = 10.dp, vertical = 0.dp),
                    ) {
                        Icon(Icons.Filled.Add, contentDescription = null, modifier = Modifier.size(16.dp))
                        Spacer(Modifier.width(4.dp))
                        Text(stringResource(R.string.egress_upload_conf), maxLines = 1)
                    }
                } else {
                    OutlinedTextField(
                        value = address,
                        onValueChange = { address = it },
                        label = { Text(stringResource(R.string.egress_address)) },
                        placeholder = { Text(stringResource(R.string.egress_address_hint)) },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
                        modifier = Modifier.fillMaxWidth(),
                    )
                    OutlinedTextField(
                        value = username,
                        onValueChange = { username = it },
                        label = { Text(stringResource(R.string.egress_username)) },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    OutlinedTextField(
                        value = password,
                        onValueChange = { password = it },
                        label = { Text(stringResource(R.string.egress_password)) },
                        placeholder = {
                            Text(
                                if (editing && egress?.passwordSet == true) {
                                    stringResource(R.string.egress_password_saved)
                                } else {
                                    ""
                                },
                            )
                        },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    ToggleRow(
                        title = stringResource(R.string.egress_resolve),
                        subtitle = stringResource(
                            if (resolveLocal) R.string.egress_resolve_local else R.string.egress_resolve_remote,
                        ),
                        checked = resolveLocal,
                        onCheckedChange = { resolveLocal = it },
                    )
                }
                ToggleRow(
                    title = stringResource(R.string.egress_fail_closed),
                    subtitle = stringResource(R.string.egress_fail_closed_hint),
                    checked = failClosed,
                    onCheckedChange = { failClosed = it },
                )
                ToggleRow(
                    title = stringResource(R.string.egress_enabled),
                    subtitle = stringResource(R.string.egress_enabled_hint),
                    checked = enabled,
                    onCheckedChange = { enabled = it },
                )
            }
        },
        confirmButton = {
            Row {
                if (onTest != null) {
                    TextButton(onClick = onTest, enabled = !testing) {
                        Text(stringResource(if (testing) R.string.egress_testing else R.string.egress_test))
                    }
                }
                TextButton(
                    onClick = {
                        onSave(
                            EgressSaveRequest(
                                kind = kind,
                                name = name.trim(),
                                country = country.trim().uppercase(Locale.US),
                                address = if (wireguard) null else address.trim(),
                                username = if (wireguard) null else username.trim(),
                                password = if (wireguard) null else password.ifBlank { null },
                                config = if (wireguard) config.trim().ifBlank { null } else null,
                                resolve = if (wireguard) null else if (resolveLocal) "local" else "remote",
                                failClosed = failClosed,
                                enabled = enabled,
                            ),
                        )
                    },
                    enabled = valid && !saving,
                ) { Text(stringResource(if (editing) R.string.action_save else R.string.action_add)) }
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}

@Composable
private fun ToggleRow(
    title: String,
    subtitle: String,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
) {
    Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Column(modifier = Modifier.weight(1f)) {
            Text(title, style = MaterialTheme.typography.bodyMedium)
            Text(
                subtitle,
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        Spacer(Modifier.width(8.dp))
        PanelToggle(checked = checked, onCheckedChange = onCheckedChange)
    }
}

@Composable
private fun EmptyAgents() {
    Box(
        modifier = Modifier.fillMaxWidth().heightIn(min = 240.dp),
        contentAlignment = Alignment.Center,
    ) {
        Column(
            horizontalAlignment = Alignment.CenterHorizontally,
            modifier = Modifier.padding(24.dp),
        ) {
            Icon(
                Icons.Filled.Speed,
                contentDescription = null,
                modifier = Modifier.size(28.dp),
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Spacer(Modifier.height(8.dp))
            Text(
                stringResource(R.string.agents_empty_title),
                style = MaterialTheme.typography.titleSmall,
                color = MaterialTheme.colorScheme.onSurface,
            )
            Spacer(Modifier.height(4.dp))
            Text(
                stringResource(R.string.agents_empty_body),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
@Suppress("LongMethod", "LongParameterList")
private fun AgentCard(
    agent: RelayAgentInfo,
    assignedDomains: Int,
    serviceLabels: List<String>,
    testing: Boolean,
    onEdit: () -> Unit,
    onToggle: () -> Unit,
    onDelete: () -> Unit,
    onCopyIp: () -> Unit,
    onCopyKey: () -> Unit,
    onRotateKey: () -> Unit,
    onTest: () -> Unit,
) {
    val online = agent.isOnline()
    val status = when {
        !agent.enabled -> stringResource(R.string.agents_status_disabled)
        online -> stringResource(R.string.agents_status_online)
        else -> stringResource(R.string.agents_status_offline)
    }
    val statusColor = when {
        !agent.enabled -> MaterialTheme.colorScheme.onSurfaceVariant
        online -> DigitDnsTheme.colors.success
        else -> DigitDnsTheme.colors.warning
    }

    // Compact by design: two cards must fit above the nav bar on a short phone
    // without scrolling, so the page only scrolls from the third agent onward.
    PanelCard(
        modifier = Modifier
            .fillMaxWidth()
            .alpha(if (agent.enabled) 1f else 0.62f),
        contentPadding = 8.dp,
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Row(verticalAlignment = Alignment.Top) {
                CountryFlag(
                    country = agent.country,
                    width = 24.dp,
                    height = 18.dp,
                    modifier = Modifier.padding(top = 2.dp),
                )
                Spacer(Modifier.width(8.dp))
                Column(modifier = Modifier.weight(1f)) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(
                            agent.displayName,
                            style = MaterialTheme.typography.titleSmall,
                            fontWeight = FontWeight.Bold,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            modifier = Modifier.weight(1f, fill = false),
                        )
                        Spacer(Modifier.width(6.dp))
                        StatusBadge(status, statusColor)
                    }
                    Text(
                        text = agentMeta(agent),
                        style = MaterialTheme.typography.labelSmall.copy(fontFamily = PanelMono),
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(1.dp),
                ) {
                    AgentActionButton(
                        icon = Icons.Filled.Edit,
                        label = stringResource(R.string.agents_edit),
                        onClick = onEdit,
                    )
                    AgentActionButton(
                        icon = if (agent.enabled) Icons.Filled.Pause else Icons.Filled.PlayArrow,
                        label = stringResource(
                            if (agent.enabled) R.string.agents_disable else R.string.agents_enable,
                        ),
                        onClick = onToggle,
                    )
                    AgentActionButton(
                        icon = Icons.Filled.Delete,
                        label = stringResource(R.string.agents_delete),
                        onClick = onDelete,
                        tint = DigitDnsTheme.colors.danger,
                    )
                }
            }

            AgentValueRow(
                label = stringResource(R.string.agents_public_ip),
                value = agent.address,
                onCopy = onCopyIp,
                copyEnabled = agent.publicIp.isNotBlank(),
            )
            AgentValueRow(
                label = stringResource(R.string.agents_agent_key),
                value = "${agent.id}:••••••",
                onCopy = onCopyKey,
                copyEnabled = true,
                onRotate = onRotateKey,
            )

            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(10.dp))
                    .background(MaterialTheme.colorScheme.surface)
                    .padding(horizontal = 8.dp, vertical = 3.dp),
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                AgentStat(stringResource(R.string.agents_version), agent.version.ifBlank { "—" }, Modifier.weight(1f))
                AgentStat(stringResource(R.string.agents_last_seen), relayRelativeAge(agent.lastSeen), Modifier.weight(1f))
            }
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .clip(RoundedCornerShape(10.dp))
                    .background(MaterialTheme.colorScheme.surface)
                    .padding(horizontal = 8.dp, vertical = 3.dp),
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                AgentStat(stringResource(R.string.agents_applied), agent.syncLabel, Modifier.weight(1f))
                AgentStat(stringResource(R.string.agents_domains), assignedDomains.toString(), Modifier.weight(1f))
            }

            Text(
                text = if (serviceLabels.isEmpty()) {
                    stringResource(R.string.agents_traffic_service_none, agent.trafficLabel)
                } else {
                    stringResource(
                        R.string.agents_traffic_services,
                        agent.trafficLabel,
                        serviceLabels.joinToString(", "),
                    )
                },
                style = MaterialTheme.typography.labelSmall.copy(fontFamily = PanelMono),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )

            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                OutlinedButton(
                    onClick = onTest,
                    enabled = !testing,
                    modifier = Modifier.height(34.dp),
                    contentPadding = PaddingValues(horizontal = 10.dp, vertical = 0.dp),
                ) {
                    if (testing) {
                        CircularProgressIndicator(
                            modifier = Modifier.size(14.dp),
                            strokeWidth = 2.dp,
                        )
                        Spacer(Modifier.width(6.dp))
                    } else {
                        Icon(Icons.Filled.Speed, contentDescription = null, modifier = Modifier.size(16.dp))
                        Spacer(Modifier.width(4.dp))
                    }
                    Text(
                        stringResource(if (testing) R.string.agents_testing else R.string.agents_test),
                        maxLines = 1,
                    )
                }
                Spacer(Modifier.weight(1f))
                PingBadge(agent)
            }
        }
    }
}

private fun agentMeta(agent: RelayAgentInfo): String {
    val country = agent.country.trim().uppercase(Locale.US).takeIf { it.isNotBlank() }
        ?.let { code ->
            val name = Locale("", code).getDisplayCountry(Locale.getDefault())
            if (name.isNotBlank() && name != code) "$code · $name" else code
        }
    return listOfNotNull(country, agent.id.takeIf { it.isNotBlank() }).joinToString(" · ")
}

/** A compact card-header action: 32 dp, so the agent name keeps the space that
 * three stock 48 dp IconButtons used to take. */
@Composable
private fun AgentActionButton(
    icon: ImageVector,
    label: String,
    onClick: () -> Unit,
    tint: Color = MaterialTheme.colorScheme.onSurfaceVariant,
) {
    Box(
        modifier = Modifier
            .size(28.dp)
            .clip(RoundedCornerShape(8.dp))
            .clickable(role = Role.Button, onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        Icon(
            icon,
            contentDescription = label,
            tint = tint,
            modifier = Modifier.size(16.dp),
        )
    }
}

@Composable
private fun AgentValueRow(
    label: String,
    value: String,
    onCopy: () -> Unit,
    copyEnabled: Boolean,
    onRotate: (() -> Unit)? = null,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(8.dp))
            .background(MaterialTheme.colorScheme.surface)
            .padding(start = 8.dp, end = 2.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            label.uppercase(),
            style = MaterialTheme.typography.labelSmall.copy(fontFamily = PanelMono, fontWeight = FontWeight.Bold),
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.width(6.dp))
        Text(
            value,
            style = MaterialTheme.typography.labelSmall.copy(fontFamily = PanelMono),
            color = DigitDnsTheme.colors.info,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f),
        )
        if (onRotate != null) {
            IconButton(onClick = onRotate, modifier = Modifier.size(26.dp)) {
                Icon(
                    Icons.Filled.Autorenew,
                    contentDescription = stringResource(R.string.agents_rotate_key),
                    modifier = Modifier.size(13.dp),
                )
            }
        }
        PanelCopyIconButton(
            contentDescription = stringResource(R.string.agents_copy_value, label),
            onCopy = onCopy,
            enabled = copyEnabled,
            modifier = Modifier.size(26.dp),
            iconSize = 13.dp,
        )
    }
}

@Composable
private fun AgentStat(label: String, value: String, modifier: Modifier = Modifier) {
    Column(modifier = modifier) {
        Text(
            label.uppercase(),
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
        )
        Text(
            value,
            style = MaterialTheme.typography.labelMedium.copy(fontFamily = PanelMono, fontWeight = FontWeight.Bold),
            color = MaterialTheme.colorScheme.onSurface,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
    }
}

/** The upstream-resolver colour scheme: green fast, yellow over 40 ms, red over 80 ms. */
@Composable
private fun PingBadge(agent: RelayAgentInfo) {
    val colors = DigitDnsTheme.colors
    val (text, color) = when {
        !agent.hasPing -> stringResource(R.string.agents_ping_none) to MaterialTheme.colorScheme.onSurfaceVariant
        agent.pingMs > 80 -> String.format(Locale.US, "ping %.1f ms", agent.pingMs) to colors.danger
        agent.pingMs > 40 -> String.format(Locale.US, "ping %.1f ms", agent.pingMs) to colors.warning
        else -> String.format(Locale.US, "ping %.1f ms", agent.pingMs) to colors.success
    }
    StatusBadge(text, color)
}

@Composable
@Suppress("LongParameterList", "LongMethod")
private fun AgentFormDialog(
    agent: RelayAgentInfo?,
    saving: Boolean,
    onDismiss: () -> Unit,
    onSave: (String, String, String, String, Boolean) -> Unit,
) {
    var name by remember(agent) { mutableStateOf(agent?.name.orEmpty()) }
    var country by remember(agent) { mutableStateOf(agent?.country.orEmpty().uppercase(Locale.US)) }
    var publicIp by remember(agent) { mutableStateOf(agent?.publicIp.orEmpty()) }
    var publicIpv6 by remember(agent) { mutableStateOf(agent?.publicIpv6.orEmpty()) }
    var ipv6Enabled by remember(agent) { mutableStateOf(agent?.ipv6Enabled == true) }
    val editing = agent != null
    val valid = country.trim().length == 2

    PanelDialog(
        onDismissRequest = onDismiss,
        solid = true,
        title = {
            Text(stringResource(if (editing) R.string.agents_edit_title else R.string.agents_add_title))
        },
        text = {
            Column(
                modifier = Modifier.heightIn(max = 420.dp).verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Text(
                    stringResource(if (editing) R.string.agents_edit_body else R.string.agents_add_body),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                OutlinedTextField(
                    value = name,
                    onValueChange = { name = it },
                    label = { Text(stringResource(R.string.agents_name)) },
                    placeholder = { Text(stringResource(R.string.agents_name_hint)) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                CountryField(selected = country, onSelect = { country = it })
                if (editing) {
                    OutlinedTextField(
                        value = publicIp,
                        onValueChange = { publicIp = it },
                        label = { Text(stringResource(R.string.agents_public_ip)) },
                        placeholder = { Text("203.0.113.10") },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
                        modifier = Modifier.fillMaxWidth(),
                    )
                    OutlinedTextField(
                        value = publicIpv6,
                        onValueChange = { publicIpv6 = it },
                        label = { Text(stringResource(R.string.agents_public_ipv6)) },
                        placeholder = { Text("2001:db8::10") },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
                        modifier = Modifier.fillMaxWidth(),
                    )
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Column(modifier = Modifier.weight(1f)) {
                            Text(
                                stringResource(R.string.agents_ipv6_use),
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.onSurface,
                            )
                            Text(
                                stringResource(R.string.agents_ipv6_optional),
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                        Spacer(Modifier.width(8.dp))
                        PanelToggle(
                            checked = ipv6Enabled,
                            onCheckedChange = { ipv6Enabled = it },
                        )
                    }
                    Text(
                        stringResource(R.string.agents_ipv6_note),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                } else {
                    Text(
                        stringResource(R.string.agents_public_ip_hint),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        },
        confirmButton = {
            TextButton(
                onClick = { onSave(name, country, publicIp, publicIpv6, ipv6Enabled) },
                enabled = valid && !saving,
            ) { Text(stringResource(if (editing) R.string.action_save else R.string.action_add)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}

/** A searchable-feeling country chooser: ISO codes ordered by display name, flags beside. */
@Composable
private fun CountryField(selected: String, onSelect: (String) -> Unit) {
    var expanded by remember { mutableStateOf(false) }
    val code = selected.trim().uppercase(Locale.US)
    val label = if (code.length == 2) {
        Locale("", code).getDisplayCountry(Locale.getDefault()).ifBlank { code }
    } else {
        stringResource(R.string.agents_country_hint)
    }
    Box {
        OutlinedButton(
            onClick = { expanded = true },
            modifier = Modifier.fillMaxWidth(),
        ) {
            if (code.length == 2) {
                CountryFlag(country = code, width = 20.dp, height = 15.dp)
                Spacer(Modifier.width(8.dp))
            }
            Text(label, modifier = Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis)
            Icon(Icons.Filled.ArrowDropDown, contentDescription = null)
        }
        DigitDnsDropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            Column(
                modifier = Modifier
                    .heightIn(max = 300.dp)
                    .width(260.dp)
                    .verticalScroll(rememberScrollState()),
            ) {
                countryOptions.forEach { (optionCode, optionName) ->
                    DropdownMenuItem(
                        text = {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                CountryFlag(country = optionCode, width = 20.dp, height = 15.dp)
                                Spacer(Modifier.width(8.dp))
                                Text("$optionCode · $optionName", maxLines = 1, overflow = TextOverflow.Ellipsis)
                            }
                        },
                        onClick = {
                            expanded = false
                            onSelect(optionCode)
                        },
                    )
                }
            }
        }
    }
}

private val countryOptions: List<Pair<String, String>> by lazy {
    Locale.getISOCountries()
        .map { code -> code to Locale("", code).getDisplayCountry(Locale.getDefault()) }
        .sortedBy { (_, name) -> name.lowercase(Locale.getDefault()) }
}
