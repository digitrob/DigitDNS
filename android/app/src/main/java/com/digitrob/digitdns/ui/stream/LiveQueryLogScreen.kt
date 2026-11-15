package com.digitrob.digitdns.ui.stream

import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectTapGestures
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
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowDownward
import androidx.compose.material.icons.filled.ArrowUpward
import androidx.compose.material.icons.filled.Clear
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
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
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.positionInRoot
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.onClick
import androidx.compose.ui.semantics.onLongClick
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.draw.clip
import com.digitrob.digitdns.ui.components.CountryFlag
import com.digitrob.digitdns.ui.components.DnsDetailsDialog as SharedDnsDetailsDialog
import com.digitrob.digitdns.ui.components.formatLatencyMs
import com.digitrob.digitdns.ui.components.toDnsDetails
import com.digitrob.digitdns.ui.components.PanelDialog
import java.util.Locale
import kotlinx.coroutines.launch
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.IntSize
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.R
import com.digitrob.digitdns.core.notifications.QueryEvent
import com.digitrob.digitdns.data.stream.StreamStatus
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
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
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
    val clipboard = LocalClipboardManager.current
    val haptics = LocalHapticFeedback.current
    val scope = rememberCoroutineScope()
    val domainCopied = stringResource(R.string.live_log_domain_copied)
    // Long-pressing a domain is the phone's equivalent of selecting the text:
    // copy it and confirm with the same toast the row menu uses.
    val copyDomain: (String) -> Unit = { domain ->
        clipboard.setText(AnnotatedString(domain))
        haptics.performHapticFeedback(HapticFeedbackType.LongPress)
        scope.launch { snackbar.showPanelToast(domainCopied) }
    }

    LifecycleResumeEffect(Unit) {
        viewModel.onResume()
        onPauseOrDispose { viewModel.onPause() }
    }

    LaunchedEffect(state.refreshTick) {
        if (state.refreshTick > 0) {
            listState.scrollToItem(0)
        }
    }

    // The stream buffer changes up to four times a second. Filtering and sorting it
    // once per relevant state change keeps the lazy list from rebuilding the same
    // sequence on every read during composition.
    val visible = remember(state.source, state.actionFilter, state.search, state.sort) {
        state.visible
    }

    // Live feed: keep the list pinned to the top as new entries arrive, but only
    // when the user is already at the top so reading older rows is not hijacked.
    // scrollToItem rather than animateScrollToItem: a new query every few hundred
    // milliseconds must not start (and supersede) a scroll animation each time.
    val atTop = listState.firstVisibleItemIndex == 0 &&
        listState.firstVisibleItemScrollOffset == 0
    LaunchedEffect(state.buffer.size, atTop) {
        if (!state.paused && state.buffer.isNotEmpty() && atTop) {
            listState.scrollToItem(0)
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
                if (visible.isEmpty()) {
                    EmptyState(state)
                } else {
                    LazyColumn(modifier = Modifier.fillMaxSize(), state = listState) {
                        items(visible, key = { it.id }, contentType = { 0 }) { event ->
                            QueryRow(
                                event = event,
                                onAction = { action, targets ->
                                    viewModel.applyQuickAction(event.domain, action, targets)
                                },
                                onCopyDomain = copyDomain,
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
private fun StreamStatusChip(status: StreamStatus) {
    val colors = DigitDnsTheme.colors
    val (label, tone) = when (status) {
        StreamStatus.LIVE -> "Live" to colors.success
        StreamStatus.CONNECTING -> "Connecting" to colors.warning
        StreamStatus.RETRYING -> "Retrying" to colors.warning
        StreamStatus.AUTH_FAILED -> "Sign-in needed" to colors.danger
        StreamStatus.STOPPED -> "Stopped" to MaterialTheme.colorScheme.onSurfaceVariant
        StreamStatus.IDLE -> "Idle" to MaterialTheme.colorScheme.onSurfaceVariant
    }
    StatusBadge(label, tone, uppercase = false)
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

        Column(
            modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            OutlinedTextField(
                value = state.search,
                onValueChange = viewModel::onSearchChange,
                label = { Text("Search domain") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            ActionFilterChips(state.actionFilter, state.status, viewModel::onActionFilterChange)
        }
    }
}

/**
 * Action filters as status pills rather than a popup. The old dropdown anchored
 * to the right edge and Compose flipped it to the opposite side when it ran out
 * of room; a chip row has no popup to misplace and uses the same colour language
 * as the action badges in the rows below.
 */
@Composable
private fun ActionFilterChips(
    selected: ActionFilter,
    status: StreamStatus,
    onSelect: (ActionFilter) -> Unit,
) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Row(
            modifier = Modifier.weight(1f).horizontalScroll(rememberScrollState()),
            horizontalArrangement = Arrangement.spacedBy(6.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            ActionFilter.entries.forEach { filter ->
                val tone = when (filter) {
                    ActionFilter.All -> MaterialTheme.colorScheme.primary
                    ActionFilter.Proxy -> DigitDnsTheme.colors.purple
                    ActionFilter.Direct -> DigitDnsTheme.colors.success
                    ActionFilter.Block -> MaterialTheme.colorScheme.error
                }
                val active = filter == selected
                val shape = RoundedCornerShape(999.dp)
                Box(
                    modifier = Modifier
                        .clip(shape)
                        .background(if (active) tone.copy(alpha = 0.16f) else MaterialTheme.colorScheme.surface)
                        .border(
                            width = 1.dp,
                            color = if (active) tone.copy(alpha = 0.55f) else MaterialTheme.colorScheme.outlineVariant,
                            shape = shape,
                        )
                        .clickable { onSelect(filter) }
                        .padding(horizontal = 12.dp, vertical = 6.dp),
                ) {
                    Text(
                        text = filter.name,
                        style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.Bold),
                        color = if (active) tone else MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 1,
                    )
                }
            }
        }
        // The connection state belongs at the trailing edge of the filter row.
        // It used to sit at the end of the scrolling button row above, which put
        // it off-screen on a phone exactly when an operator needed it.
        Spacer(Modifier.width(8.dp))
        StreamStatusChip(status)
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
    onCopyDomain: (String) -> Unit,
    patterns: Set<String>,
) {
    var menuOpen by remember { mutableStateOf(false) }
    var relayOpen by remember(event.id) { mutableStateOf(false) }
    // Read-only DNS response detail for this row. Kept separate from relayOpen so
    // the two dialogs never stack, and keyed on the query id so a recycled row
    // never inherits the previous query's open state.
    var infoOpen by remember(event.id) { mutableStateOf(false) }
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
                val copyLabel = stringResource(R.string.live_log_domain_copied)
                val infoLabel = stringResource(R.string.live_log_info_desc)
                Text(
                    text = event.domain.ifBlank { "—" },
                    // Tap opens the DNS-response popup; long-press copies the name.
                    // Tapping the domain replaces the old (i) button, which cost a
                    // 32 dp slot on every row. pointerInput keeps both gestures on
                    // one target; the semantics actions keep them reachable through
                    // TalkBack.
                    modifier = Modifier
                        .pointerInput(event.domain) {
                            detectTapGestures(
                                onTap = { if (event.domain.isNotBlank()) infoOpen = true },
                                onLongPress = {
                                    if (event.domain.isNotBlank()) onCopyDomain(event.domain)
                                },
                            )
                        }
                        .semantics {
                            onClick(label = infoLabel) {
                                if (event.domain.isNotBlank()) infoOpen = true
                                event.domain.isNotBlank()
                            }
                            onLongClick(label = copyLabel) {
                                if (event.domain.isNotBlank()) onCopyDomain(event.domain)
                                event.domain.isNotBlank()
                            }
                        },
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
                        if (event.latencyMs > 0) append(" · ${formatLatencyMs(event.latencyMs)}")
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

            // Relay flag + status badge. The DNS detail opens from the domain text
            // on the left, so no per-row info button is spent on it; the flag
            // explains the exit and the badge opens the routing menu.
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
        if (infoOpen) {
            SharedDnsDetailsDialog(event.toDnsDetails(), onDismiss = { infoOpen = false })
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
        QueryActionTone.Proxy -> colors.proxy
        QueryActionTone.Danger -> colors.danger
        QueryActionTone.Success -> colors.success
        QueryActionTone.Warning -> colors.warning
        QueryActionTone.Purple -> colors.purple
        QueryActionTone.Muted -> colors.textMuted
    }
    Box(modifier = if (enabled) Modifier.clickable(onClick = onClick) else Modifier) {
        StatusBadge(text = style.label, color = color, uppercase = false)
    }
}
