package com.digitrob.digitdns.ui.dashboard

import android.os.Build
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectDragGestures
import androidx.compose.foundation.gestures.detectHorizontalDragGestures
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
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.rememberLazyGridState
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
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.core.network.LatencySnapshot
import com.digitrob.digitdns.core.network.LiveStats
import com.digitrob.digitdns.core.network.UpstreamStat
import com.digitrob.digitdns.data.DashboardCard
import com.digitrob.digitdns.ui.components.CountryFlag
import com.digitrob.digitdns.ui.components.MetricCard
import com.digitrob.digitdns.ui.components.MetricValue
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.PanelScreenHeader
import com.digitrob.digitdns.ui.components.icon
import com.digitrob.digitdns.ui.system.ServerSwitcherDialog
import com.digitrob.digitdns.ui.formatBytes
import com.digitrob.digitdns.ui.formatCount
import com.digitrob.digitdns.ui.formatRate
import com.digitrob.digitdns.ui.formatUptime
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.MetricLabel
import com.digitrob.digitdns.ui.theme.MetricMeta
import com.digitrob.digitdns.ui.theme.MetricNumber
import com.digitrob.digitdns.ui.theme.PanelMono
import kotlinx.coroutines.launch
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
@Composable
fun DashboardScreen(
    onAddServer: () -> Unit,
    onLoginRequired: () -> Unit,
    viewModel: DashboardViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    var showServerSwitcher by remember { mutableStateOf(false) }

    when {
        state.isLoading && state.stats == null -> LoadingState()
        state.error != null && state.stats == null -> ErrorState(state.error!!)
        else -> Column(modifier = Modifier.fillMaxSize()) {
            PanelScreenHeader()
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
                        contentDescription = "Switch server",
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
                    onBenchmark = viewModel::benchmarkUpstreams,
                    onAddUpstream = viewModel::addUpstream,
                    onRemoveUpstream = viewModel::removeUpstream,
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
            onSwitched = {
                showServerSwitcher = false
                viewModel.refresh()
            },
        )
    }
}

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
                text = "Server Public IP/Domain",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
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
            contentDescription = "Copy server address",
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

@Composable
private fun DashboardGrid(
    state: DashboardUiState,
    onBenchmark: () -> Unit,
    onAddUpstream: suspend (String) -> String?,
    onRemoveUpstream: suspend (String) -> String?,
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
                        onBenchmark = onBenchmark,
                        onAdd = onAddUpstream,
                        onRemove = onRemoveUpstream,
                        swipeEnabled = !gridState.isScrollInProgress,
                    )
                }
                DashboardCard.RESOLVER_LATENCY -> item(span = { GridItemSpan(maxLineSpan) }) {
                    ResolverLatencyCard(state.latency)
                }
            }
        }

        item(span = { GridItemSpan(maxLineSpan) }) {
            UptimeFooter(state.stats)
        }
    }
}

// The dashboard cards below mirror the panel's own tiles one-for-one

@Composable
private fun QueryRateCard(stats: LiveStats?) {
    val colors = DigitDnsTheme.colors
    MetricCard {
        Column(verticalArrangement = Arrangement.spacedBy(1.dp)) {
            CardLabel(DashboardCard.QUERY_RATE, "Query Rate", compact = true)
            MetricValue(
                text = stats?.let { String.format(Locale.US, "%.1f", it.qps) } ?: "—",
                color = MaterialTheme.colorScheme.primary,
            )
            Text(
                text = "QPS",
                style = MetricMeta,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            val total = stats?.totalQueries ?: 0
            val limit = stats?.rateLimitQps ?: 0
            val dropped = stats?.rateLimited ?: 0
            // No colons: at 10sp this line has to stay on one line inside a 122dp
            // inner width on a 320dp phone, and the two separators are the only
            // characters that can give way without losing information.
            val first = buildString {
                append("Total ").append(formatCount(total))
                if (limit > 0) append(" · Limit ").append(formatCount(limit)).append("/s")
            }
            Text(
                text = first,
                style = MetricMeta,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = "Dropped ${formatCount(dropped)}",
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
            CardLabel(DashboardCard.RAM_CACHE, "RAM Cache", compact = true)
            Row(verticalAlignment = Alignment.Bottom) {
                MetricValue(
                    text = stats?.let { String.format(Locale.US, "%.1f", it.cacheHitRate) } ?: "—",
                    color = colors.success,
                )
                Text(
                    text = "% Hits",
                    style = MetricMeta,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(start = 4.dp, bottom = 3.dp),
                )
            }
            val items = formatCount(stats?.cacheItems ?: 0)
            val hits = formatCount(stats?.cacheHits ?: 0)
            val misses = formatCount(stats?.cacheMisses ?: 0)
            Text(
                text = "Items $items · $hits hits",
                style = MetricMeta,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = "Missed $misses",
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
            CardLabel(DashboardCard.SNI_PROXY, "SNI Proxy", compact = true)
            MetricValue(
                text = formatCount(stats?.activeRelays ?: 0),
                color = colors.purple,
            )
            Text(
                text = "Relays",
                style = MetricMeta,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            val drops = (stats?.relaysRefused ?: 0) + (stats?.relaysUnreadable ?: 0)
            Text(
                text = "Total ${formatCount(stats?.totalRelays ?: 0)} · ${if (drops > 0) "${formatCount(drops)} drops" else "no drops"}",
                style = MetricMeta,
                color = if (drops > 0) colors.warning else MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun CpuUsageCard(stats: LiveStats?) {
    val colors = DigitDnsTheme.colors
    val pct = stats?.cpuUsagePercent ?: 0.0
    MetricCard {
        Column(verticalArrangement = Arrangement.spacedBy(1.dp)) {
            CardLabel(DashboardCard.CPU_USAGE, "CPU Usage", compact = true)
            MetricValue(
                text = stats?.let { String.format(Locale.US, "%.1f%%", it.cpuUsagePercent) } ?: "—",
                color = colorForPercent(pct, colors.danger, colors.warning, colors.success),
            )
            Text(
                text = "${stats?.numCpu ?: 0} cores",
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
            CardLabel(DashboardCard.RAM_USAGE, "RAM Usage", compact = true)
            MetricValue(
                text = stats?.let { formatBytes((it.ramUsageMb * 1024 * 1024).toLong()) } ?: "—",
                color = colors.info,
            )
            val sysPct = stats?.systemMemPercent ?: 0.0
            Text(
                text = "System ${String.format(Locale.US, "%.0f%%", sysPct)}",
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
            CardLabel(DashboardCard.LIVE_TRAFFIC, "Live Traffic", compact = true)
            MetricValue(
                text = stats?.let { formatBytes(it.totalBytesTransferred) } ?: "—",
                color = MaterialTheme.colorScheme.onSurface,
            )
            Text(
                text = "Total",
                style = MetricMeta,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = "↓ ${stats?.let { formatRate(it.speedInKbps) } ?: "—"} · ↑ ${stats?.let { formatRate(it.speedOutKbps) } ?: "—"}",
                style = MetricMeta,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
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
                CardLabel(DashboardCard.TELEMETRY_CHART, "Query Rate", modifier = Modifier.weight(1f))
                Text(
                    text = stats?.let { "%.1f qps".format(it.qps) } ?: "—",
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
                text = "Last ${history.size} samples, every 3s",
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
            CardLabel(DashboardCard.RESOLVER_LATENCY, "Resolver Latency")
            if (!latency.hasData) {
                Text(
                    text = "No observations yet.",
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
                    LatencyCell("p50 median", latency.p50Ms, color = latencyColor(latency.p50Ms), modifier = Modifier.weight(1f))
                    LatencyCell("p95", latency.p95Ms, modifier = Modifier.weight(1f))
                    LatencyCell("p99 worst 1%", latency.p99Ms, modifier = Modifier.weight(1f))
                    LatencyCell("Peak", latency.maxMs, modifier = Modifier.weight(1f))
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
        text = "Uptime ${stats?.let { formatUptime(it.uptimeSec) } ?: "—"}",
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.fillMaxWidth().padding(top = 4.dp),
    )
}

@Composable
internal fun Label(
    text: String,
    modifier: Modifier = Modifier,
    style: TextStyle = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.Medium),
) {
    Text(
        text = text.uppercase(Locale.US),
        style = style,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = modifier,
    )
}

/** A card header: the panel's per-card icon beside the uppercase label. */
@Composable
private fun CardLabel(
    card: DashboardCard,
    title: String,
    modifier: Modifier = Modifier,
    compact: Boolean = false,
) {
    Row(verticalAlignment = Alignment.CenterVertically, modifier = modifier) {
        Icon(
            imageVector = card.icon,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(CARD_ICON_SIZE),
        )
        Spacer(Modifier.width(6.dp))
        Label(title, style = if (compact) MetricLabel else MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.Medium))
    }
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
private val CARD_ICON_SIZE = 14.dp

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
