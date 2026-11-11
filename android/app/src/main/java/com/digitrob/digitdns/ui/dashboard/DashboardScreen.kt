@file:Suppress("TooManyFunctions")

package com.digitrob.digitdns.ui.dashboard

import android.os.Build
import androidx.compose.foundation.BorderStroke
import androidx.annotation.StringRes
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectDragGestures
import androidx.compose.foundation.gestures.detectHorizontalDragGestures
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyGridScope
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.rememberLazyGridState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.animation.core.animateDpAsState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Podcasts
import androidx.compose.material.icons.filled.Public
import androidx.compose.material.icons.rounded.SwapHoriz
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.material3.OutlinedIconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.draw.blur
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.R
import com.digitrob.digitdns.core.network.LatencySnapshot
import com.digitrob.digitdns.core.network.LiveStats
import com.digitrob.digitdns.core.network.ResolutionSettings
import com.digitrob.digitdns.core.network.UpstreamPresetProvider
import com.digitrob.digitdns.core.network.UpstreamStat
import com.digitrob.digitdns.data.DashboardCard
import com.digitrob.digitdns.domain.model.HistoryRange
import com.digitrob.digitdns.domain.model.TopDimension
import com.digitrob.digitdns.ui.components.CountryFlag
import com.digitrob.digitdns.ui.components.MetricCard
import com.digitrob.digitdns.ui.components.MetricValue
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.ConnectionStatusBadge
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.icon
import com.digitrob.digitdns.ui.system.ServerSwitcherDialog
import com.digitrob.digitdns.ui.formatBytes
import com.digitrob.digitdns.ui.formatCount
import com.digitrob.digitdns.ui.formatRate
import com.digitrob.digitdns.ui.formatUptime
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.MetricMeta
import com.digitrob.digitdns.ui.theme.MetricNumber
import com.digitrob.digitdns.ui.theme.PanelMono
import kotlinx.coroutines.launch
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale
import kotlin.math.abs

/**
 * The dashboard: the same cards the panel leads with, sized for a phone. It is a
 * read-only view that follows [DashboardViewModel]; the control actions live on their
 * own tabs, because a monitoring glance and a config change are different intents.
 *
 * The cards are drawn in the operator's saved order ({@link DashboardCard}), so a
 * phone can hide the metrics it never watches and put the ones it does at the top.
 */
@Suppress("LongMethod") // Header, refresh shell and server switcher share one screen state.
@Composable
fun DashboardScreen(
    onAddServer: () -> Unit,
    onLoginRequired: () -> Unit,
    onOpenSubscriber: () -> Unit = {},
    viewModel: DashboardViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    var showServerSwitcher by remember { mutableStateOf(false) }

    DashboardLifecycle(viewModel)

    when {
        state.isLoading && state.stats == null -> LoadingState()
        state.error != null && state.stats == null -> ErrorState(state.error!!)
        else -> Column(modifier = Modifier.fillMaxSize()) {
            DashboardTopBar(state.history, viewModel::setHistoryRange)
            // The switcher shares the IP row and is inset by the same 12dp the header's
            // padding gives the connection pill, so its glyph sits directly under the
            // ONLINE badge. It used to be flush with the screen edge, which put it
            // visibly to the right of the pill above it.
            Row(
                // The switcher is a square the height of the IP row, so its top and
                // bottom edges line up with the address box instead of floating short
                // of them. Intrinsic sizing lets the box decide the row height and the
                // button follow it, whatever the font scale.
                modifier = Modifier.fillMaxWidth().height(IntrinsicSize.Min),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                ServerIpBox(
                    serverIp = state.serverIp,
                    country = state.serverCountry,
                    domain = state.serverDomain,
                    modifier = Modifier.weight(1f),
                )
                OutlinedIconButton(
                    onClick = { showServerSwitcher = true },
                    modifier = Modifier
                        // No start padding: the address box already carries a 12 dp
                        // transparent inset on its right, so the visible gap between
                        // the two is exactly that 12 dp — the same gap the dashboard
                        // grid puts between cards. The end inset keeps the column
                        // aligned with the grid's content padding.
                        .padding(end = 12.dp)
                        .fillMaxHeight()
                        // Width stays the square side (the row height); the inner
                        // vertical padding trims the button itself to the visible
                        // address box, which is the row minus its 4 dp outer padding.
                        .aspectRatio(1f)
                        // 48 dp tall: the width still comes from the row height, so
                        // the result is a short rounded rectangle. The button stays
                        // centred in the row's height, and the glyph is centred in
                        // the button by IconButton itself.
                        .padding(vertical = 8.dp),
                    // A rounded square, not a circle: it matches the dashboard's
                    // 12 dp card corners and lines up in width with the ONLINE pill
                    // above it (11 sp label + 10 dp padding either side ≈ 56 dp).
                    shape = RoundedCornerShape(12.dp),
                    border = BorderStroke(
                        width = 1.dp,
                        color = MaterialTheme.colorScheme.primary.copy(alpha = 0.55f),
                    ),
                    colors = IconButtonDefaults.outlinedIconButtonColors(
                        containerColor = MaterialTheme.colorScheme.primary.copy(alpha = 0.08f),
                        contentColor = MaterialTheme.colorScheme.primary,
                    ),
                ) {
                    Icon(
                        imageVector = Icons.Rounded.SwapHoriz,
                        contentDescription = stringResource(R.string.dashboard_switch_server),
                    )
                }
            }
            PanelRefreshBox(
                isRefreshing = state.isRefreshing,
                onRefresh = { viewModel.refresh() },
                modifier = Modifier.fillMaxSize(),
            ) {
                DashboardGrid(
                    state = state,
                    actions = DashboardActions(
                        onBenchmark = viewModel::benchmarkUpstreams,
                        onAddUpstream = viewModel::addUpstream,
                        onRemoveUpstream = viewModel::removeUpstream,
                        onLoadUpstreamPresets = viewModel::loadUpstreamPresets,
                        onResolutionChange = viewModel::saveResolution,
                        onRunAdaptive = viewModel::runAdaptiveNow,
                    ),
                )
            }
        }
    }

    if (showServerSwitcher) {
        ServerSwitcherDialog(
            onDismiss = { showServerSwitcher = false },
            onAddServer = {
                showServerSwitcher = false
                onAddServer()
            },
            onLoginRequired = {
                showServerSwitcher = false
                onLoginRequired()
            },
            onOpenSubscriber = {
                showServerSwitcher = false
                onOpenSubscriber()
            },
            onSwitched = {
                showServerSwitcher = false
                viewModel.refresh()
            },
        )
    }
}

/**
 * Start and stop the dashboard's live work with the screen lifecycle. Kept out of
 * [DashboardScreen] itself so its composition stays under the project's method-length
 * budget as the screen grows.
 */
@Composable
private fun DashboardLifecycle(viewModel: DashboardViewModel) {
    LifecycleResumeEffect(Unit) {
        viewModel.onResume()
        onPauseOrDispose { viewModel.onPause() }
    }
}

/**
 * The dashboard's top strip: history range chips on the left, the aggregate's update
 * clock in the middle and the live-connection pill on the right. The chips used to sit
 * in their own full-width card, which pushed the first metrics below the fold; folding
 * them into the header keeps the range one tap away without spending a grid row.
 *
 * The chip strip is the only weighted child and it scrolls, so an operator on a very
 * narrow phone can still reach `90d` while the clock and the ONLINE badge stay pinned.
 */
@Composable
private fun DashboardTopBar(
    history: HistoryUiState,
    onRangeChange: (HistoryRange) -> Unit,
) {
    val colors = DigitDnsTheme.colors
    // A background history poll flips isLoading for a moment every ~30 s. Do not
    // replace the timestamp with "Loading…" for that: it makes the whole row
    // flash and the range chips jump. Only a first load with no clock yet shows
    // the loading word.
    val updated = when {
        history.isLoading && history.updatedAtEpochMs == null -> stringResource(R.string.history_loading)
        history.unavailable -> stringResource(R.string.history_unavailable_short)
        history.error != null -> stringResource(R.string.history_error_short)
        history.updatedAtEpochMs != null -> stringResource(
            R.string.history_updated,
            HISTORY_CLOCK.format(Instant.ofEpochMilli(history.updatedAtEpochMs)),
        )
        else -> stringResource(R.string.history_waiting_short)
    }
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Row(
            modifier = Modifier.weight(1f).horizontalScroll(rememberScrollState()),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            HistoryRange.entries.forEach { candidate ->
                HistoryRangePill(
                    label = stringResource(candidate.labelRes()),
                    selected = candidate == history.range,
                    onClick = { onRangeChange(candidate) },
                )
            }
        }
        Text(
            text = updated,
            style = MaterialTheme.typography.labelSmall.copy(
                fontFamily = PanelMono,
                fontSize = 8.sp,
                lineHeight = 10.sp,
            ),
            color = if (history.unavailable || history.error != null) {
                colors.warning
            } else {
                MaterialTheme.colorScheme.onSurfaceVariant
            },
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        ConnectionStatusBadge()
    }
}

/** One compact range pill. Smaller than a Material chip so five fit beside the badge. */
@Composable
private fun HistoryRangePill(
    label: String,
    selected: Boolean,
    onClick: () -> Unit,
) {
    val colors = DigitDnsTheme.colors
    val accent = MaterialTheme.colorScheme.primary
    Box(
        modifier = Modifier
            .clip(RoundedCornerShape(6.dp))
            .background(if (selected) accent.copy(alpha = 0.16f) else Color.Transparent)
            .border(
                width = 1.dp,
                color = if (selected) accent.copy(alpha = 0.6f) else colors.borderColor,
                shape = RoundedCornerShape(6.dp),
            )
            .clickable(onClick = onClick)
            .padding(horizontal = 4.dp, vertical = 2.dp),
    ) {
        Text(
            text = label,
            style = MaterialTheme.typography.labelSmall.copy(
                fontSize = 10.sp,
                lineHeight = 12.sp,
                fontWeight = if (selected) FontWeight.SemiBold else FontWeight.Medium,
            ),
            color = if (selected) accent else MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
        )
    }
}

/** Wall-clock stamp for the history header; seconds are enough to show a refresh. */
private val HISTORY_CLOCK = DateTimeFormatter.ofPattern("HH:mm:ss")
    .withZone(ZoneId.systemDefault())

/**
 * The install's public address, pinned above the grid. The panel keeps the same box in
 * its sidebar; here it fills the strip the header would otherwise leave empty and gives
 * the address a one-tap copy, which is what an operator setting a console's DNS needs.
 * Hidden until the address is known, so an empty box never reads as "no address".
 */
@Composable
private fun ServerIpBox(
    serverIp: String,
    country: String,
    domain: String,
    modifier: Modifier = Modifier,
) {
    if (serverIp.isBlank()) return
    val colors = DigitDnsTheme.colors
    val clipboard = LocalClipboardManager.current
    val swipeThreshold = with(LocalDensity.current) { 24.dp.toPx() }
    var hidden by rememberSaveable { mutableStateOf(false) }
    var showDomain by rememberSaveable { mutableStateOf(false) }
    val hasDomain = domain.isNotBlank()
    val displayed = if (showDomain && hasDomain) domain else serverIp
    // Compose can only blur from Android 12. On 10/11 the privacy swipe masks the
    // value instead of leaving it readable behind a no-op blur.
    val canBlur = Build.VERSION.SDK_INT >= Build.VERSION_CODES.S
    val shownText = if (hidden && !canBlur) maskAddress(displayed) else displayed

    Row(
        modifier = modifier
            .padding(horizontal = 12.dp, vertical = 4.dp)
            .clip(RoundedCornerShape(12.dp))
            .background(colors.bgCard)
            .border(1.dp, colors.borderColor, RoundedCornerShape(12.dp))
            .clickable { clipboard.setText(AnnotatedString(displayed)) }
            .padding(horizontal = 12.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            imageVector = Icons.Filled.Public,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.primary,
            modifier = Modifier.size(16.dp),
        )
        Spacer(Modifier.width(8.dp))
        Column(
            // The swipes live on the text column, never on the row: a gesture that
            // starts on the flag or the copy glyph leaves both exactly as they are.
            modifier = Modifier
                .weight(1f)
                .then(
                    if (hasDomain) {
                        Modifier.addressSwipe(
                            threshold = swipeThreshold,
                            onTogglePrivacy = { hidden = !hidden },
                            onToggleView = { showDomain = !showDomain },
                        )
                    } else {
                        Modifier.privacySwipe(swipeThreshold) { hidden = !hidden }
                    },
                ),
        ) {
            Text(
                text = stringResource(R.string.dashboard_server_address),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                // The address row and the switch button are height-matched through
                // IntrinsicSize.Min. A label that wraps makes the measured intrinsic
                // height a single line narrower than the rendered one, and the address
                // then falls outside the row's clip on a 320 dp phone. Keeping the
                // label to one line keeps the intrinsic and actual heights equal.
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            AddressValue(text = shownText, hidden = hidden, canBlur = canBlur)
        }
        if (country.isNotBlank()) {
            Spacer(Modifier.width(10.dp))
            CountryFlag(country, width = 24.dp, height = 18.dp)
            Spacer(Modifier.width(10.dp))
        }
        Icon(
            imageVector = Icons.Filled.ContentCopy,
            contentDescription = stringResource(R.string.dashboard_copy_address),
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(16.dp),
        )
    }
}

/** The address itself — the one thing the privacy swipe is allowed to cover. */
@Composable
private fun AddressValue(text: String, hidden: Boolean, canBlur: Boolean) {
    val radius by animateDpAsState(
        targetValue = if (hidden && canBlur) 8.dp else 0.dp,
        label = "addressBlur",
    )
    Text(
        text = text,
        style = MaterialTheme.typography.titleMedium.copy(fontFamily = PanelMono),
        color = MaterialTheme.colorScheme.onSurface,
        maxLines = 1,
        overflow = TextOverflow.Ellipsis,
        modifier = if (canBlur) Modifier.blur(radius) else Modifier,
    )
}

/** Keeps an address' shape while replacing every character that carries information. */
private fun maskAddress(value: String): String =
    value.map { c -> if (c.isLetterOrDigit()) '•' else c }.joinToString("")

/**
 * One drag detector for the address block. The first axis to cross [threshold] wins and
 * the action fires while the finger is still down, so a short flick is enough — waiting
 * for the lift, and mapping each direction to one fixed outcome, is what made the old
 * pair of gestures feel dead. Only one action is taken per gesture, so a continued drag
 * cannot toggle back.
 */
private fun Modifier.addressSwipe(
    threshold: Float,
    onTogglePrivacy: () -> Unit,
    onToggleView: () -> Unit,
): Modifier = pointerInput(threshold) {
    var horizontal = 0f
    var vertical = 0f
    var fired = false
    detectDragGestures(
        onDragStart = {
            horizontal = 0f
            vertical = 0f
            fired = false
        },
        onDrag = { change, amount ->
            change.consume()
            if (fired) return@detectDragGestures
            horizontal += amount.x
            vertical += amount.y
            if (abs(horizontal) >= threshold && abs(horizontal) > abs(vertical)) {
                fired = true
                onTogglePrivacy()
            } else if (abs(vertical) >= threshold) {
                fired = true
                onToggleView()
            }
        },
    )
}

/** A horizontal swipe across the value toggles the privacy blur (no domain to swap). */
private fun Modifier.privacySwipe(threshold: Float, onToggle: () -> Unit): Modifier =
    pointerInput(threshold) {
        var drag = 0f
        var fired = false
        detectHorizontalDragGestures(
            onDragStart = {
                drag = 0f
                fired = false
            },
            onHorizontalDrag = { change, amount ->
                change.consume()
                if (fired) return@detectHorizontalDragGestures
                drag += amount
                if (abs(drag) >= threshold) {
                    fired = true
                    onToggle()
                }
            },
        )
    }

private data class DashboardActions(
    val onBenchmark: () -> Unit,
    val onAddUpstream: suspend (String) -> String?,
    val onRemoveUpstream: suspend (String) -> String?,
    val onLoadUpstreamPresets: suspend () -> List<UpstreamPresetProvider>?,
    val onResolutionChange: (ResolutionSettings) -> Unit,
    val onRunAdaptive: () -> Unit,
)

@Composable
private fun DashboardGrid(
    state: DashboardUiState,
    actions: DashboardActions,
) {
    // The grid state is hoisted for one reason: the racer card's swipe-to-delete has
    // to know a scroll is in flight, or a diagonal scroll can settle as a swipe and pop
    // the remove-upstream confirmation.
    val gridState = rememberLazyGridState()

    LazyVerticalGrid(
        columns = GridCells.Fixed(2),
        state = gridState,
        modifier = Modifier.fillMaxSize().padding(12.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        state.layout.visible.forEach { card ->
            if (card.name.startsWith("HISTORY_")) {
                historyCard(card, state)
            } else {
                liveCard(
                    card = card,
                    state = state,
                    swipeEnabled = !gridState.isScrollInProgress,
                    actions = actions,
                )
            }
        }

        item(span = { GridItemSpan(maxLineSpan) }) {
            UptimeFooter(state.stats)
        }
    }
}

/** The original live tiles, one branch per card. */
private fun LazyGridScope.liveCard(
    card: DashboardCard,
    state: DashboardUiState,
    swipeEnabled: Boolean,
    actions: DashboardActions,
) {
    when (card) {
        DashboardCard.QUERY_RATE -> item { QueryRateCard(state.stats) }
        DashboardCard.RAM_CACHE -> item { RamCacheCard(state.stats) }
        DashboardCard.SNI_PROXY -> item { SniProxyCard(state.stats) }
        DashboardCard.CPU_USAGE -> item { CpuUsageCard(state.stats) }
        DashboardCard.RAM_USAGE -> item { RamUsageCard(state.stats) }
        DashboardCard.LIVE_TRAFFIC -> item { LiveTrafficCard(state.stats) }
        DashboardCard.TELEMETRY_CHART -> item(span = { GridItemSpan(maxLineSpan) }) {
            TelemetryChartCard(state.qpsHistory, state.stats)
        }
        DashboardCard.UPSTREAM_RACERS -> item(span = { GridItemSpan(maxLineSpan) }) {
            UpstreamRacersCard(
                upstreams = state.upstreams,
                isBenchmarking = state.isBenchmarking,
                onBenchmark = actions.onBenchmark,
                actions = UpstreamActions(
                    onAdd = actions.onAddUpstream,
                    onRemove = actions.onRemoveUpstream,
                    onLoadPresets = actions.onLoadUpstreamPresets,
                ),
                swipeEnabled = swipeEnabled,
                resolutionControls = ResolutionControls(
                    settings = state.resolution,
                    onChange = actions.onResolutionChange,
                    isAdaptiveRunning = state.isAdaptiveRunning,
                    onRunAdaptive = actions.onRunAdaptive,
                    lastSingleUpstream = state.lastSingleUpstream,
                ),
            )
        }
        DashboardCard.RESOLVER_LATENCY -> item(span = { GridItemSpan(maxLineSpan) }) {
            ResolverLatencyCard(state.latency)
        }
        else -> Unit
    }
}

/** The aggregate-history cards; they share the same layout order as the live tiles. */
private fun LazyGridScope.historyCard(
    card: DashboardCard,
    state: DashboardUiState,
) {
    when (card) {
        DashboardCard.HISTORY_SUMMARY -> item(span = { GridItemSpan(maxLineSpan) }) {
            HistorySummaryCard(state.history.summary, state.history.unavailable)
        }
        DashboardCard.HISTORY_CHART -> item(span = { GridItemSpan(maxLineSpan) }) {
            HistoryChartCard(state.history.series, state.history.unavailable)
        }
        DashboardCard.HISTORY_TOP_DOMAINS -> item(span = { GridItemSpan(maxLineSpan) }) {
            HistoryTopCard(
                dimension = TopDimension.DOMAIN,
                top = state.history.tops[TopDimension.DOMAIN],
                unavailable = state.history.unavailable,
                serverCountry = state.serverCountry,
            )
        }
        DashboardCard.HISTORY_TOP_BLOCKED -> item(span = { GridItemSpan(maxLineSpan) }) {
            HistoryTopCard(
                dimension = TopDimension.BLOCKED_DOMAIN,
                top = state.history.tops[TopDimension.BLOCKED_DOMAIN],
                unavailable = state.history.unavailable,
                serverCountry = state.serverCountry,
            )
        }
        DashboardCard.HISTORY_TOP_CLIENTS -> item(span = { GridItemSpan(maxLineSpan) }) {
            HistoryTopCard(
                dimension = TopDimension.CLIENT,
                top = state.history.tops[TopDimension.CLIENT],
                unavailable = state.history.unavailable,
                serverCountry = state.serverCountry,
            )
        }
        DashboardCard.HISTORY_TOP_UPSTREAMS -> item(span = { GridItemSpan(maxLineSpan) }) {
            HistoryTopCard(
                dimension = TopDimension.UPSTREAM,
                top = state.history.tops[TopDimension.UPSTREAM],
                unavailable = state.history.unavailable,
                serverCountry = state.serverCountry,
            )
        }
        DashboardCard.HISTORY_TOP_PROTOCOLS -> item {
            HistoryTopCard(
                dimension = TopDimension.PROTOCOL,
                top = state.history.tops[TopDimension.PROTOCOL],
                unavailable = state.history.unavailable,
                serverCountry = state.serverCountry,
            )
        }
        DashboardCard.HISTORY_TOP_RELAY -> item {
            HistoryTopCard(
                dimension = TopDimension.RELAY_COUNTRY,
                top = state.history.tops[TopDimension.RELAY_COUNTRY],
                unavailable = state.history.unavailable,
                serverCountry = state.serverCountry,
            )
        }
        else -> Unit
    }
}

// The dashboard cards below mirror the panel's own tiles one-for-one

@Composable
private fun QueryRateCard(stats: LiveStats?) {
    val colors = DigitDnsTheme.colors
    MetricCard {
        Column(verticalArrangement = Arrangement.spacedBy(1.dp)) {
            CardLabel(DashboardCard.QUERY_RATE, R.string.dashboard_query_rate)
            MetricValue(
                text = stats?.let { String.format(Locale.US, "%.1f", it.qps) } ?: "—",
                color = MaterialTheme.colorScheme.primary,
            )
            Text(
                text = stringResource(R.string.dashboard_qps),
                style = MetricMeta,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            val total = stats?.totalQueries ?: 0
            val limit = stats?.rateLimitQps ?: 0
            val dropped = stats?.rateLimited ?: 0
            // No colons: at 10sp this line has to stay on one line inside a 122dp
            // inner width on a 320dp phone, and the two separators are the only
            // characters that can give way without losing information.
            val first = if (limit > 0) {
                stringResource(R.string.dashboard_total_limit, formatCount(total), formatCount(limit))
            } else {
                stringResource(R.string.dashboard_total, formatCount(total))
            }
            Text(
                text = first,
                style = MetricMeta,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = stringResource(R.string.dashboard_dropped, formatCount(dropped)),
                style = MetricMeta,
                color = if (dropped > 0) colors.warning else MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun RamCacheCard(stats: LiveStats?) {
    val colors = DigitDnsTheme.colors
    MetricCard {
        Column(verticalArrangement = Arrangement.spacedBy(1.dp)) {
            CardLabel(DashboardCard.RAM_CACHE, R.string.dashboard_ram_cache)
            MetricValue(
                text = stats?.let { String.format(Locale.US, "%.1f", it.cacheHitRate) } ?: "—",
                color = colors.success,
            )
            // The unit sits on its own line now: it is the same four-line shape as
            // Query Rate, so the two cards sharing a row stay level.
            Text(
                text = stringResource(R.string.dashboard_hits_unit),
                style = MetricMeta,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            val items = formatCount(stats?.cacheItems ?: 0)
            val hits = formatCount(stats?.cacheHits ?: 0)
            val misses = formatCount(stats?.cacheMisses ?: 0)
            Text(
                text = stringResource(R.string.dashboard_cache_items, items, hits),
                style = MetricMeta,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = stringResource(R.string.dashboard_cache_missed, misses),
                style = MetricMeta,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun SniProxyCard(stats: LiveStats?) {
    val colors = DigitDnsTheme.colors
    MetricCard {
        Column(verticalArrangement = Arrangement.spacedBy(1.dp)) {
            CardLabel(DashboardCard.SNI_PROXY, R.string.dashboard_sni_proxy)
            MetricValue(
                text = formatCount(stats?.activeRelays ?: 0),
                color = colors.purple,
            )
            Text(
                text = stringResource(R.string.dashboard_relays),
                style = MetricMeta,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            // Flood refusals are the penalty box counting every retry from a
            // malformed-probe source. One scanner can dominate the raw refused
            // counter, so the footer reports only actionable refusals. Unreadable
            // connections are a separate diagnostic: on a public port they are
            // mostly scanners that opened TLS without a usable SNI, and folding
            // them into "drops" made a healthy relay look broken.
            val flood = stats?.relaysRefusedFlood ?: 0
            val actionableRefused = ((stats?.relaysRefused ?: 0) - flood).coerceAtLeast(0)
            val unreadable = stats?.relaysUnreadable ?: 0
            // Same colour as every other card's secondary line. The amber warning tint
            // made this one tile's footer read differently for the same kind of text.
            Text(
                text = if (actionableRefused > 0) {
                    stringResource(R.string.dashboard_relay_total, formatCount(stats?.totalRelays ?: 0), formatCount(actionableRefused))
                } else {
                    stringResource(R.string.dashboard_relay_total_clean, formatCount(stats?.totalRelays ?: 0))
                },
                style = MetricMeta,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (unreadable > 0) {
                Text(
                    text = stringResource(R.string.dashboard_relay_unreadable, formatCount(unreadable)),
                    style = MetricMeta,
                    color = colors.warning,
                )
            }
        }
    }
}

@Composable
private fun CpuUsageCard(stats: LiveStats?) {
    val colors = DigitDnsTheme.colors
    val pct = stats?.cpuUsagePercent ?: 0.0
    MetricCard {
        Column(verticalArrangement = Arrangement.spacedBy(1.dp)) {
            CardLabel(DashboardCard.CPU_USAGE, R.string.dashboard_cpu_usage)
            MetricValue(
                text = stats?.let { String.format(Locale.US, "%.1f%%", it.cpuUsagePercent) } ?: "—",
                color = colorForPercent(pct, colors.danger, colors.warning, colors.success),
            )
            Text(
                text = stringResource(R.string.dashboard_cores, stats?.numCpu ?: 0),
                style = MetricMeta,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun RamUsageCard(stats: LiveStats?) {
    val colors = DigitDnsTheme.colors
    MetricCard {
        Column(verticalArrangement = Arrangement.spacedBy(1.dp)) {
            CardLabel(DashboardCard.RAM_USAGE, R.string.dashboard_ram_usage)
            // The daemon's own allocation is the headline; machine-wide server
            // RAM is useful context and stays underneath it.
            MetricValue(
                text = stats?.let { formatBytes((it.ramUsageMb * 1024 * 1024).toLong()) } ?: "—",
                color = colors.info,
            )
            val serverUsed = stats?.let { formatBytes((it.systemMemUsedMb * 1024 * 1024).toLong()) } ?: "—"
            val serverTotal = stats?.let { formatBytes((it.systemMemTotalMb * 1024 * 1024).toLong()) } ?: "—"
            Text(
                text = stringResource(R.string.dashboard_ram_server, serverUsed, serverTotal),
                style = MetricMeta,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            val sysPct = stats?.systemMemPercent ?: 0.0
            Text(
                text = stringResource(R.string.dashboard_ram_server_usage, String.format(Locale.US, "%.0f%%", sysPct)),
                style = MetricMeta,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun LiveTrafficCard(stats: LiveStats?) {
    MetricCard {
        Column(verticalArrangement = Arrangement.spacedBy(1.dp)) {
            CardLabel(DashboardCard.LIVE_TRAFFIC, R.string.dashboard_live_traffic)
            MetricValue(
                text = stats?.let { formatBytes(it.totalBytesTransferred) } ?: "—",
                color = MaterialTheme.colorScheme.onSurface,
            )
            val inRate = stats?.let { formatRate(it.speedInKbps) } ?: "—"
            val outRate = stats?.let { formatRate(it.speedOutKbps) } ?: "—"
            Text(
                text = stringResource(R.string.dashboard_traffic_total, inRate, outRate),
                style = MetricMeta,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 2,
            )
        }
    }
}

/** A Compose-Canvas sparkline of the recent QPS window — no charting dependency. */
@Composable
private fun TelemetryChartCard(history: List<Double>, stats: LiveStats?) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                DashboardCardTitle(
                    text = stringResource(R.string.dashboard_query_rate),
                    icon = DashboardCard.TELEMETRY_CHART.icon,
                    modifier = Modifier.weight(1f),
                )
                Text(
                    text = stats?.let { stringResource(R.string.dashboard_qps_value, it.qps) } ?: "—",
                    style = MaterialTheme.typography.titleMedium.copy(fontFamily = PanelMono),
                    color = MaterialTheme.colorScheme.primary,
                )
            }
            Sparkline(
                values = history,
                lineColor = MaterialTheme.colorScheme.primary,
                fillColor = MaterialTheme.colorScheme.primary.copy(alpha = 0.12f),
                modifier = Modifier.fillMaxWidth().height(64.dp),
            )
            Text(
                text = stringResource(R.string.dashboard_samples, history.size),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun Sparkline(
    values: List<Double>,
    lineColor: Color,
    fillColor: Color,
    modifier: Modifier = Modifier,
) {
    Canvas(modifier = modifier) {
        if (values.size < 2) return@Canvas
        val maxValue = (values.maxOrNull() ?: 0.0).coerceAtLeast(1.0)
        val stepX = size.width / (values.size - 1)
        val points = values.mapIndexed { i, v ->
            Offset(i * stepX, size.height - (v / maxValue * size.height).toFloat())
        }
        val line = Path().apply {
            moveTo(points.first().x, points.first().y)
            points.drop(1).forEach { lineTo(it.x, it.y) }
        }
        val fill = Path().apply {
            moveTo(points.first().x, size.height)
            points.forEach { lineTo(it.x, it.y) }
            lineTo(points.last().x, size.height)
            close()
        }
        drawPath(fill, color = fillColor)
        drawPath(line, color = lineColor, style = Stroke(width = 2.dp.toPx()))
    }
}

/** The resolver racer: upstreams sorted fastest-first, with add/remove and a benchmark. */
@Composable
private fun ResolverLatencyCard(latency: LatencySnapshot) {
    // A tighter inset than the standard card: four equal tiles plus their own padding
    // have to fit the phone width, and the default 16 dp edge pushed a four-digit
    // millisecond value past the tile and clipped it.
    PanelCard(modifier = Modifier.fillMaxWidth(), contentPadding = 12.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            DashboardCardTitle(
                text = stringResource(R.string.dashboard_resolver_latency),
                icon = DashboardCard.RESOLVER_LATENCY.icon,
            )
            if (!latency.hasData) {
                Text(
                    text = stringResource(R.string.dashboard_no_observations),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            } else {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(4.dp),
                ) {
                    // The panel colours only p50 — the number an operator reads as "how
                    // fast is the resolver right now" — and leaves the tail percentiles
                    // neutral, so the tile does not become a wall of red during a blip.
                    // Each cell takes an equal share so the values sit evenly spread
                    // rather than bunched against each other on a wide screen.
                    LatencyCell(
                        stringResource(R.string.dashboard_p50),
                        latency.p50Ms,
                        color = latencyColor(latency.p50Ms),
                        modifier = Modifier.weight(1f),
                    )
                    LatencyCell(stringResource(R.string.dashboard_p95), latency.p95Ms, modifier = Modifier.weight(1f))
                    LatencyCell(stringResource(R.string.dashboard_p99), latency.p99Ms, modifier = Modifier.weight(1f))
                    LatencyCell(stringResource(R.string.dashboard_peak), latency.maxMs, modifier = Modifier.weight(1f))
                }
            }
        }
    }
}

/**
 * The panel's own resolver-latency thresholds applied to p50: ≥60 ms reads red,
 * ≥25 ms amber, otherwise green. Matches `app.js`, which gates the same two numbers.
 */
@Composable
private fun latencyColor(ms: Double): Color {
    val colors = DigitDnsTheme.colors
    return when {
        ms >= RESOLVER_LATENCY_DANGER_MS -> colors.danger
        ms >= RESOLVER_LATENCY_WARNING_MS -> colors.warning
        else -> colors.success
    }
}

@Composable
private fun LatencyCell(label: String, ms: Double, color: Color? = null, modifier: Modifier = Modifier) {
    val colors = DigitDnsTheme.colors
    Column(
        modifier = modifier
            .clip(RoundedCornerShape(10.dp))
            .background(colors.bgTile)
            .padding(horizontal = 4.dp, vertical = 6.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(2.dp),
    ) {
        Text(
            text = "%.1f ms".format(ms),
            style = MaterialTheme.typography.titleMedium.copy(fontFamily = PanelMono),
            color = color ?: MaterialTheme.colorScheme.onSurface,
            maxLines = 1,
            softWrap = false,
            overflow = TextOverflow.Ellipsis,
        )
        Text(
            text = label,
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 2,
            textAlign = TextAlign.Center,
        )
    }
}

@Composable
private fun UptimeFooter(stats: LiveStats?) {
    Text(
        text = stringResource(R.string.dashboard_uptime, stats?.let { formatUptime(it.uptimeSec) } ?: "—"),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.fillMaxWidth().padding(top = 4.dp),
    )
}

/** A card header: the panel's per-card icon beside the uppercase label. */
@Composable
private fun CardLabel(
    card: DashboardCard,
    @StringRes titleRes: Int,
    modifier: Modifier = Modifier,
) {
    DashboardCardTitle(
        text = stringResource(titleRes),
        icon = card.icon,
        modifier = modifier,
    )
}

/** Whether the live stream is currently feeding the live tiles. */
@Composable
private fun LoadingState() {
    Column(
        modifier = Modifier.fillMaxSize(),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        CircularProgressIndicator(color = MaterialTheme.colorScheme.primary)
    }
}

@Composable
private fun ErrorState(message: String) {
    val colors = DigitDnsTheme.colors
    Column(
        modifier = Modifier.fillMaxSize().padding(24.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(
            text = message,
            color = colors.danger,
            style = MaterialTheme.typography.bodyLarge,
        )
    }
}

/** At/above this CPU percentage the tile reads red. */
private const val CPU_DANGER_PERCENT = 90.0

/** At/above (and below danger) the tile reads amber. */
private const val CPU_WARNING_PERCENT = 70.0

/**
 * The floor height of every single-span metric tile. Cards carry a different number of
 * lines (Query Rate adds a totals line, CPU just a core count), and in a two-column grid
 * unequal cards leave a visible gap beside the shorter one. Pinning a common floor makes
 * the grid read as a grid.
 */
/** The card-icon size every dashboard tile header uses. */

/** Resolver-latency p50 at/above this reads red — the panel's own threshold. */
private const val RESOLVER_LATENCY_DANGER_MS = 60.0

/** Resolver-latency p50 at/above (and below danger) reads amber. */
private const val RESOLVER_LATENCY_WARNING_MS = 25.0

/** A percent-driven colour, so a hot CPU looks hot. */
private fun colorForPercent(
    value: Double,
    danger: Color,
    warning: Color,
    ok: Color,
) = when {
    value >= CPU_DANGER_PERCENT -> danger
    value >= CPU_WARNING_PERCENT -> warning
    else -> ok
}
