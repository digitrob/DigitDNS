package com.digitrob.digitdns.ui.stream

import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowDownward
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.ArrowUpward
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Clear
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
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
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.positionInRoot
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.draw.clip
import com.digitrob.digitdns.ui.components.CountryFlag
import com.digitrob.digitdns.ui.components.PanelDialog
import java.util.Locale
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.unit.IntSize
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.R
import com.digitrob.digitdns.core.notifications.QueryEvent
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.PanelRowDefaults
import com.digitrob.digitdns.ui.components.PanelHeaderMeta
import com.digitrob.digitdns.ui.components.PanelScreenHeader
import com.digitrob.digitdns.ui.components.PanelSnackbarHost
import com.digitrob.digitdns.ui.components.RouteAction
import com.digitrob.digitdns.ui.components.RouteActionMenu
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.components.showPanelToast
import com.digitrob.digitdns.ui.formatLatency
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono
import com.digitrob.digitdns.ui.theme.protocolColor
import com.digitrob.digitdns.ui.theme.protocolLabel

/**
 * The Live Query Log tab, matching the panel's top-level `Live Query Log`.
 *
 * Each row mirrors the panel's columns — TIME · CLIENT/ACCOUNT · PROTO · QUERY DOMAIN
 * · RULE MATCHED · ACTION · LATENCY — collapsed onto a phone's width as two lines: the
 * domain over who asked and how, then the action badge, rule and latency. The full
 * seven-column table would need horizontal scrolling, which on a live, shifting feed is
 * worse than a compact row.
 *
 * The feed is the shared [com.digitrob.digitdns.data.stream.LiveQueryStream], the same
 * socket that feeds the notifications, so opening this tab never opens a second
 * connection. Pause freezes the visible snapshot without stopping the stream, and a
 * per-row menu applies the panel's quick actions (Proxy/Block/Direct/Remove).
 */
@Composable
fun LiveQueryLogScreen(viewModel: LiveQueryLogViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }
    val listState = rememberLazyListState()

    LaunchedEffect(state.refreshTick) {
        if (state.refreshTick > 0) {
            listState.animateScrollToItem(0)
        }
    }

    // Live feed: keep the list pinned to the top as new entries arrive, but only
    // when the user is already at the top so reading older rows is not hijacked.
    LaunchedEffect(state.buffer.size) {
        if (!state.paused && !listState.isScrollInProgress && state.buffer.isNotEmpty()) {
            listState.animateScrollToItem(0)
        }
    }

    LaunchedEffect(state.message) {
        val message = state.message ?: return@LaunchedEffect
        snackbar.showPanelToast(message)
        viewModel.consumeMessage()
    }

    Scaffold(snackbarHost = { PanelSnackbarHost(snackbar) }, contentWindowInsets = WindowInsets(0)) { padding ->
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            PanelScreenHeader(
                actions = { QueryCountHeader(state.source.size) },
            )
            LogToolbar(state, viewModel)
            PanelRefreshBox(
                isRefreshing = state.isRefreshing,
                onRefresh = viewModel::refresh,

                modifier = Modifier.fillMaxSize(),
            ) {
                if (state.visible.isEmpty()) {
                    EmptyState(state)
                } else {
                    LazyColumn(modifier = Modifier.fillMaxSize(), state = listState) {
                        items(state.visible, key = { it.id }, contentType = { 0 }) { event ->
                            QueryRow(
                                event = event,
                                onAction = { action, targets ->
                                    viewModel.applyQuickAction(event.domain, action, targets)
                                },
                                patterns = state.patterns,
                            )
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun QueryCountHeader(count: Int) {
    // Match the 24 dp height of the Updated/Online cluster so the two baselines
    // line up, and pin the width so a growing count cannot move the text.
    Box(
        modifier = Modifier.width(80.dp).height(24.dp),
        contentAlignment = Alignment.CenterStart,
    ) {
        PanelHeaderMeta(
            text = pluralStringResource(R.plurals.stream_query_count, count, count),
            modifier = Modifier.fillMaxWidth(),
        )
    }
}

@Composable
private fun LogToolbar(state: LiveQueryLogUiState, viewModel: LiveQueryLogViewModel) {
    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 4.dp)) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .horizontalScroll(rememberScrollState()),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(2.dp),
        ) {
            TextButton(
                onClick = viewModel::reload,
                modifier = Modifier.width(88.dp),
                contentPadding = PaddingValues(horizontal = 8.dp, vertical = 4.dp),
            ) {
                Icon(Icons.Filled.Refresh, contentDescription = null, modifier = Modifier.size(18.dp))
                Spacer(Modifier.width(4.dp))
                Text("Reload", maxLines = 1)
            }
            TextButton(
                onClick = viewModel::toggleSort,
                modifier = Modifier.width(88.dp),
                contentPadding = PaddingValues(horizontal = 8.dp, vertical = 4.dp),
            ) {
                Icon(
                    if (state.sort == LogSort.Newest) Icons.Filled.ArrowDownward else Icons.Filled.ArrowUpward,
                    contentDescription = null,
                    modifier = Modifier.size(18.dp),
                )
                Spacer(Modifier.width(4.dp))
                Text(if (state.sort == LogSort.Newest) "Newest" else "Oldest", maxLines = 1)
            }
            TextButton(
                onClick = { viewModel.setPaused(!state.paused) },
                modifier = Modifier.width(88.dp),
                contentPadding = PaddingValues(horizontal = 8.dp, vertical = 4.dp),
            ) {
                Icon(
                    if (state.paused) Icons.Filled.PlayArrow else Icons.Filled.Pause,
                    contentDescription = null,
                    modifier = Modifier.size(18.dp),
                )
                Spacer(Modifier.width(4.dp))
                Text(if (state.paused) "Resume" else "Pause", maxLines = 1)
            }
            TextButton(
                onClick = viewModel::clear,
                modifier = Modifier.width(76.dp),
                contentPadding = PaddingValues(horizontal = 8.dp, vertical = 4.dp),
            ) {
                Icon(Icons.Filled.Clear, contentDescription = null, modifier = Modifier.size(18.dp))
                Spacer(Modifier.width(4.dp))
                Text("Clear", maxLines = 1)
            }
        }

        Row(
            modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            OutlinedTextField(
                value = state.search,
                onValueChange = viewModel::onSearchChange,
                label = { Text("Search domain") },
                singleLine = true,
                modifier = Modifier.weight(1f),
            )
            ActionFilterDropdown(state.actionFilter, viewModel::onActionFilterChange)
        }
    }
}

/** The action filter as a dropdown button, placed to the right of the search field. */
@Composable
private fun ActionFilterDropdown(selected: ActionFilter, onSelect: (ActionFilter) -> Unit) {
    val colors = DigitDnsTheme.colors
    var expanded by remember { mutableStateOf(false) }
    OutlinedButton(onClick = { expanded = true }) {
        Text(selected.name, maxLines = 1)
        Icon(Icons.Filled.ArrowDropDown, contentDescription = "Filter")
    }
    DropdownMenu(
        expanded = expanded,
        onDismissRequest = { expanded = false },
        modifier = Modifier
            .background(colors.bgCard)
            .border(1.dp, colors.borderColor, RoundedCornerShape(12.dp)),
    ) {
        ActionFilter.entries.forEach { filter ->
            DropdownMenuItem(
                text = { Text(filter.name) },
                trailingIcon = {
                    if (filter == selected) {
                        Icon(Icons.Filled.Check, contentDescription = null, modifier = Modifier.size(18.dp))
                    }
                },
                onClick = {
                    expanded = false
                    onSelect(filter)
                },
            )
        }
    }
}

@Composable
private fun EmptyState(state: LiveQueryLogUiState) {
    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Text(
            text = if (state.source.isEmpty()) {
                "Waiting for queries…"
            } else {
                "No queries match the filter."
            },
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
@Suppress("LongMethod")
private fun QueryRow(
    event: QueryEvent,
    onAction: (RouteAction, List<String>) -> Unit,
    patterns: Set<String>,
) {
    var menuOpen by remember { mutableStateOf(false) }
    var relayOpen by remember(event.id) { mutableStateOf(false) }
    var rowSize by remember { mutableStateOf(IntSize.Zero) }
    var rowY by remember { mutableStateOf(0f) }
    Box(modifier = Modifier.onGloballyPositioned { coords -> rowSize = coords.size; rowY = coords.positionInRoot().y }) {
        PanelCard(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 12.dp, vertical = 4.dp)
                .heightIn(min = PanelRowDefaults.height),
            verticalPadding = 8.dp,
            contentAlignment = Alignment.CenterStart,
            highlighted = menuOpen,
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
            Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(
                    text = event.domain.ifBlank { "—" },
                    style = MaterialTheme.typography.bodyLarge,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                // The transport that resolved the name; the client transport is only
                // shown when nothing upstream was involved. Only that token is
                // coloured, so the rest of the line stays at the muted body colour.
                val resolvedBy = event.upstreamProtocol.ifBlank { event.protocol }
                val protocolTint = protocolColor(resolvedBy)
                Text(
                    text = buildAnnotatedString {
                        append(event.accountName.ifBlank { event.clientIp.ifBlank { "unknown" } })
                        if (resolvedBy.isNotBlank()) {
                            append(" · ")
                            withStyle(SpanStyle(color = protocolTint, fontWeight = FontWeight.SemiBold)) {
                                append(protocolLabel(resolvedBy))
                            }
                        }
                        if (event.latencyMs > 0) append(" · ${formatLatency(event.latencyMs)}")
                        if (event.upstream.isNotBlank()) append(" · via ${event.upstream}")
                        if (event.timestamp.isNotBlank()) {
                            val t = event.timestamp.substringAfter('T').substringBefore('.').trimEnd('Z')
                            append(" · $t")
                        }
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }

            // Relay flag + status badge. The flag is a second, read-only detail:
            // tapping it explains the exit without opening the routing menu.
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                if (event.action.equals("PROXY", ignoreCase = true) && event.relayCountry.isNotBlank()) {
                    val relayLabel = event.relayAgentName.ifBlank { event.relayCountry }
                    val relayDescription = stringResource(R.string.live_log_relay_desc, relayLabel)
                    Box(
                        modifier = Modifier
                            .size(32.dp)
                            .clip(RoundedCornerShape(4.dp))
                            .clickable { relayOpen = true }
                            .semantics { contentDescription = relayDescription },
                        contentAlignment = Alignment.Center,
                    ) {
                        CountryFlag(
                            country = event.relayCountry,
                            width = 18.dp,
                            height = 13.dp,
                        )
                    }
                }
                ActionBadge(
                    action = event.action,
                    onClick = { menuOpen = true },
                    enabled = event.domain.isNotBlank(),
                )
            }
        }

        }
        if (relayOpen) {
            RelayDetailsDialog(event, onDismiss = { relayOpen = false })
        }
        RouteActionMenu(
            domain = event.domain,
            expanded = menuOpen,
            onDismiss = { menuOpen = false },
            onAction = onAction,
            allowRemove = false,
            patterns = patterns,
            anchorSize = rowSize,
            anchorY = rowY,
        )
    }
}


/** The read-only relay explanation opened by the small flag in a live-log row. */
@Composable
private fun RelayDetailsDialog(event: QueryEvent, onDismiss: () -> Unit) {
    val country = event.relayCountry.trim().uppercase(Locale.US)
    val countryLabel = if (country.length == 2) {
        Locale("", country).getDisplayCountry(Locale.getDefault()).ifBlank { country }
    } else {
        ""
    }
    val name = event.relayAgentName.ifBlank { countryLabel.ifBlank { stringResource(R.string.relay_this_server) } }
    PanelDialog(
        onDismissRequest = onDismiss,
        solid = true,
        title = { Text(stringResource(R.string.live_log_relay_title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(10.dp),
                ) {
                    CountryFlag(country = country, width = 28.dp, height = 21.dp)
                    Column {
                        Text(name, style = MaterialTheme.typography.bodyLarge, fontWeight = FontWeight.SemiBold)
                        if (countryLabel.isNotBlank()) {
                            Text(
                                countryLabel,
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                }
                Text(
                    stringResource(R.string.live_log_relay_body),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        },
        confirmButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_close)) }
        },
    )
}

@Composable
private fun ActionBadge(action: String, onClick: () -> Unit, enabled: Boolean) {
    val colors = DigitDnsTheme.colors
    val style = queryActionStyle(action)
    val color: Color = when (style.tone) {
        QueryActionTone.Info -> colors.info
        QueryActionTone.Danger -> colors.danger
        QueryActionTone.Success -> colors.success
        QueryActionTone.Warning -> colors.warning
        QueryActionTone.Purple -> colors.purple
        QueryActionTone.Muted -> colors.textMuted
    }
    Box(modifier = if (enabled) Modifier.clickable(onClick = onClick) else Modifier) {
        StatusBadge(text = style.label, color = color)
    }
}
