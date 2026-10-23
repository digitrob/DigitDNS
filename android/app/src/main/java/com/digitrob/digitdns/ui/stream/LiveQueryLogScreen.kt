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
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
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
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.core.notifications.QueryEvent
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.PanelScreenHeader
import com.digitrob.digitdns.ui.components.PanelSnackbarHost
import com.digitrob.digitdns.ui.components.RouteAction
import com.digitrob.digitdns.ui.components.RouteActionMenu
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.components.showPanelToast
import com.digitrob.digitdns.ui.formatLatency
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

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

    LaunchedEffect(state.message) {
        val message = state.message ?: return@LaunchedEffect
        snackbar.showPanelToast(message)
        viewModel.consumeMessage()
    }

    Scaffold(snackbarHost = { PanelSnackbarHost(snackbar) }, contentWindowInsets = WindowInsets(0)) { padding ->
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            PanelScreenHeader()
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
                            QueryRow(event = event, onAction = viewModel::applyQuickAction)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun LogToolbar(state: LiveQueryLogUiState, viewModel: LiveQueryLogViewModel) {
    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 8.dp)) {
        Text(
            text = "${state.source.size} queries",
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )

        Row(
            modifier = Modifier
                .fillMaxWidth()
                .horizontalScroll(rememberScrollState())
                .padding(top = 4.dp),
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
private fun QueryRow(event: QueryEvent, onAction: (RouteAction, String) -> Unit) {
    var menuOpen by remember { mutableStateOf(false) }
    PanelCard(modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 4.dp), highlighted = menuOpen) {
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
                Text(
                    text = buildString {
                        append(event.accountName.ifBlank { event.clientIp.ifBlank { "unknown" } })
                        if (event.protocol.isNotBlank()) append(" · ${event.protocol}")
                        if (event.latencyMs > 0) append(" · ${formatLatency(event.latencyMs)}")
                        if (event.cached) append(" · cached")
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                if (event.ruleMatched.isNotBlank()) {
                    Text(
                        text = event.ruleMatched,
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        fontFamily = FontFamily.Monospace,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
            }

            // The status badge itself is the menu trigger, matching the panel where the
            // badge opens the quick-action menu.
            ActionBadge(
                action = event.action,
                onClick = { menuOpen = true },
                enabled = event.domain.isNotBlank(),
            )
        }

        RouteActionMenu(
            domain = event.domain,
            expanded = menuOpen,
            onDismiss = { menuOpen = false },
            onAction = onAction,
        )
    }
}

@Composable
private fun ActionBadge(action: String, onClick: () -> Unit, enabled: Boolean) {
    val colors = DigitDnsTheme.colors
    val color: Color = when (action.uppercase()) {
        "BLOCK" -> colors.danger
        "PROXY" -> colors.info
        "DIRECT" -> colors.success
        else -> colors.textMuted
    }
    val label = action.lowercase().replaceFirstChar { it.uppercase() }
    Box(modifier = if (enabled) Modifier.clickable(onClick = onClick) else Modifier) {
        StatusBadge(text = label.ifBlank { "—" }, color = color)
    }
}
