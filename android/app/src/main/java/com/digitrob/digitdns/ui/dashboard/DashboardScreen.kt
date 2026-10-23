package com.digitrob.digitdns.ui.dashboard

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
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
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
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
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
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
import com.digitrob.digitdns.ui.theme.PanelMono
import kotlinx.coroutines.launch
import java.util.Locale

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
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                ServerIpBox(state.serverIp, modifier = Modifier.weight(1f))
                IconButton(
                    onClick = { showServerSwitcher = true },
                    modifier = Modifier
                        .padding(start = 8.dp)
                        .size(48.dp),
                ) {
                    Icon(
                        imageVector = Icons.Rounded.SwapHoriz,
                        contentDescription = "Switch server",
                        tint = MaterialTheme.colorScheme.primary,
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
private fun ServerIpBox(serverIp: String, modifier: Modifier = Modifier) {
    if (serverIp.isBlank()) return
    val colors = DigitDnsTheme.colors
    val clipboard = LocalClipboardManager.current
    Row(
        modifier = modifier
            .padding(horizontal = 12.dp, vertical = 4.dp)
            .clip(RoundedCornerShape(12.dp))
            .background(colors.bgCard)
            .border(1.dp, colors.borderColor, RoundedCornerShape(12.dp))
            .clickable { clipboard.setText(AnnotatedString(serverIp)) }
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
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = "Server Public IP",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = serverIp,
                style = MaterialTheme.typography.titleMedium.copy(fontFamily = PanelMono),
                color = MaterialTheme.colorScheme.onSurface,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
        Icon(
            imageVector = Icons.Filled.ContentCopy,
            contentDescription = "Copy server IP",
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(16.dp),
        )
    }
}

@Composable
private fun DashboardGrid(
    state: DashboardUiState,
    onBenchmark: () -> Unit,
    onAddUpstream: suspend (String) -> String?,
    onRemoveUpstream: suspend (String) -> String?,
) {
    LazyVerticalGrid(
        columns = GridCells.Fixed(2),
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
    PanelCard(modifier = Modifier.fillMaxWidth().heightIn(min = METRIC_CARD_MIN_HEIGHT)) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            CardLabel(DashboardCard.QUERY_RATE, "Query Rate")
            Text(
                text = stats?.let { String.format(Locale.US, "%.1f", it.qps) } ?: "—",
                style = MaterialTheme.typography.headlineLarge.copy(fontFamily = PanelMono),
                color = MaterialTheme.colorScheme.primary,
            )
            Text(
                text = "QPS",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Spacer(Modifier.height(4.dp))
            val total = stats?.totalQueries ?: 0
            val limit = stats?.rateLimitQps ?: 0
            val dropped = stats?.rateLimited ?: 0
            val first = buildString {
                append("Total: ").append(formatCount(total))
                if (limit > 0) append(" · Limit:").append(formatCount(limit)).append("/s")
            }
            Text(
                text = first,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = "Dropped: ${formatCount(dropped)}",
                style = MaterialTheme.typography.bodySmall,
                color = if (dropped > 0) colors.warning else MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun RamCacheCard(stats: LiveStats?) {
    val colors = DigitDnsTheme.colors
    PanelCard(modifier = Modifier.fillMaxWidth().heightIn(min = METRIC_CARD_MIN_HEIGHT)) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            CardLabel(DashboardCard.RAM_CACHE, "RAM Cache")
            Row(verticalAlignment = Alignment.Bottom) {
                Text(
                    text = stats?.let { String.format(Locale.US, "%.1f", it.cacheHitRate) } ?: "—",
                    style = MaterialTheme.typography.headlineLarge.copy(fontFamily = PanelMono),
                    color = colors.success,
                )
                Text(
                    text = "% Hits",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(start = 4.dp, bottom = 6.dp),
                )
            }
            Spacer(Modifier.height(4.dp))
            val items = formatCount(stats?.cacheItems ?: 0)
            val hits = formatCount(stats?.cacheHits ?: 0)
            val misses = formatCount(stats?.cacheMisses ?: 0)
            Text(
                text = "Items: $items · $hits hits",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = "Missed: $misses",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun SniProxyCard(stats: LiveStats?) {
    val colors = DigitDnsTheme.colors
    PanelCard(modifier = Modifier.fillMaxWidth().heightIn(min = METRIC_CARD_MIN_HEIGHT)) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            CardLabel(DashboardCard.SNI_PROXY, "SNI Proxy")
            Text(
                text = formatCount(stats?.activeRelays ?: 0),
                style = MaterialTheme.typography.headlineLarge.copy(fontFamily = PanelMono),
                color = colors.purple,
            )
            Text(
                text = "Relays",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Spacer(Modifier.height(4.dp))
            val drops = (stats?.relaysRefused ?: 0) + (stats?.relaysUnreadable ?: 0)
            Text(
                text = "Total: ${formatCount(stats?.totalRelays ?: 0)} · ${if (drops > 0) "${formatCount(drops)} drops" else "no drops"}",
                style = MaterialTheme.typography.bodySmall,
                color = if (drops > 0) colors.warning else MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun CpuUsageCard(stats: LiveStats?) {
    val colors = DigitDnsTheme.colors
    val pct = stats?.cpuUsagePercent ?: 0.0
    PanelCard(modifier = Modifier.fillMaxWidth().heightIn(min = METRIC_CARD_MIN_HEIGHT)) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            CardLabel(DashboardCard.CPU_USAGE, "CPU Usage")
            Text(
                text = stats?.let { String.format(Locale.US, "%.1f%%", it.cpuUsagePercent) } ?: "—",
                style = MaterialTheme.typography.headlineLarge.copy(fontFamily = PanelMono),
                color = colorForPercent(pct, colors.danger, colors.warning, colors.success),
            )
            Text(
                text = "${stats?.numCpu ?: 0} cores",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun RamUsageCard(stats: LiveStats?) {
    val colors = DigitDnsTheme.colors
    PanelCard(modifier = Modifier.fillMaxWidth().heightIn(min = METRIC_CARD_MIN_HEIGHT)) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            CardLabel(DashboardCard.RAM_USAGE, "RAM Usage")
            Text(
                text = stats?.let { formatBytes((it.ramUsageMb * 1024 * 1024).toLong()) } ?: "—",
                style = MaterialTheme.typography.headlineLarge.copy(fontFamily = PanelMono),
                color = colors.info,
            )
            Spacer(Modifier.height(4.dp))
            val sysPct = stats?.systemMemPercent ?: 0.0
            Text(
                text = "System: ${String.format(Locale.US, "%.0f%%", sysPct)}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun LiveTrafficCard(stats: LiveStats?) {
    PanelCard(modifier = Modifier.fillMaxWidth().heightIn(min = METRIC_CARD_MIN_HEIGHT)) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            CardLabel(DashboardCard.LIVE_TRAFFIC, "Live Traffic")
            Text(
                text = stats?.let { formatBytes(it.totalBytesTransferred) } ?: "—",
                style = MaterialTheme.typography.headlineLarge.copy(fontFamily = PanelMono),
                color = MaterialTheme.colorScheme.onSurface,
            )
            Text(
                text = "Total",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Spacer(Modifier.height(4.dp))
            Text(
                text = "↓ ${stats?.let { formatRate(it.speedInKbps) } ?: "—"} · ↑ ${stats?.let { formatRate(it.speedOutKbps) } ?: "—"}",
                style = MaterialTheme.typography.bodySmall,
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
internal fun Label(text: String, modifier: Modifier = Modifier) {
    Text(
        text = text.uppercase(Locale.US),
        style = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.Medium),
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = modifier,
    )
}

/** A card header: the panel's per-card icon beside the uppercase label. */
@Composable
private fun CardLabel(card: DashboardCard, title: String, modifier: Modifier = Modifier) {
    Row(verticalAlignment = Alignment.CenterVertically, modifier = modifier) {
        Icon(
            imageVector = card.icon,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(16.dp),
        )
        Spacer(Modifier.width(6.dp))
        Label(title)
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
private val METRIC_CARD_MIN_HEIGHT = 128.dp

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
