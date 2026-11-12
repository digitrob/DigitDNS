@file:Suppress("TooManyFunctions")

package com.digitrob.digitdns.ui.dashboard

import androidx.annotation.StringRes
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Block
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.People
import androidx.compose.material.icons.filled.Podcasts
import androidx.compose.material.icons.filled.Public
import androidx.compose.material.icons.filled.Share
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.digitrob.digitdns.R
import com.digitrob.digitdns.core.network.StatsSeries
import com.digitrob.digitdns.core.network.StatsSeriesPoint
import com.digitrob.digitdns.core.network.StatsSummary
import com.digitrob.digitdns.data.DashboardCard
import com.digitrob.digitdns.core.network.StatsTop
import com.digitrob.digitdns.core.network.StatsTopItem
import com.digitrob.digitdns.domain.model.HistoryRange
import com.digitrob.digitdns.domain.model.TopDimension
import com.digitrob.digitdns.ui.components.CountryFlag
import com.digitrob.digitdns.ui.components.icon
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.formatCount
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.ExtendedColors
import com.digitrob.digitdns.ui.theme.PanelMono
import com.digitrob.digitdns.ui.theme.protocolColor
import com.digitrob.digitdns.ui.theme.protocolLabel
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale

/**
 * The aggregate-history cards. They deliberately live outside DashboardScreen's
 * metric tiles: each one reads the HistoryViewModel slice, draws its own empty /
 * unavailable state, and is placed by the same DashboardCard layout editor.
 */
@Composable
internal fun HistorySummaryCard(summary: StatsSummary?, unavailable: Boolean) {
    PanelCard(modifier = Modifier.fillMaxWidth(), bottomPadding = 8.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            DashboardCardTitle(stringResource(R.string.history_summary), DashboardCard.HISTORY_SUMMARY.icon)
            when {
                unavailable -> HistoryUnavailable()
                summary == null -> HistoryWaiting()
                else -> {
                    // Six metrics in two rows of three: the card stays half the
                    // height of the old two-column stack and every value gets a
                    // full third of the width on a 320 dp phone.
                    val metrics = listOf(
                        SummaryMetric(
                            label = R.string.history_total_queries,
                            value = formatCount(summary.queries),
                            meta = stringResource(R.string.history_buckets, summary.buckets),
                        ),
                        SummaryMetric(
                            label = R.string.history_blocked,
                            value = percent(summary.blockedPct),
                            meta = stringResource(R.string.history_queries_value, formatCount(summary.blocked)),
                            color = MaterialTheme.colorScheme.error,
                        ),
                        SummaryMetric(
                            label = R.string.history_proxied,
                            value = percent(summary.proxiedPct),
                            meta = stringResource(R.string.history_queries_value, formatCount(summary.proxied)),
                            color = DigitDnsTheme.colors.purple,
                        ),
                        SummaryMetric(
                            label = R.string.history_cache_hit,
                            value = percent(summary.cacheHitPct),
                            meta = stringResource(R.string.history_hits_value, formatCount(summary.cached)),
                            color = DigitDnsTheme.colors.success,
                        ),
                        SummaryMetric(
                            label = R.string.history_active_clients,
                            value = formatCount(summary.activeClients),
                            meta = stringResource(
                                if (summary.activeClientsApprox) {
                                    R.string.history_clients_approx
                                } else {
                                    R.string.history_clients_hll
                                },
                            ),
                            color = MaterialTheme.colorScheme.primary,
                        ),
                        SummaryMetric(
                            label = R.string.history_avg_latency,
                            value = String.format(Locale.US, "%.1f", summary.latencyAvgMs),
                            meta = stringResource(R.string.history_latency_unit),
                        ),
                    )
                    metrics.chunked(3).forEach { row ->
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            row.forEach { metric ->
                                SummaryCell(
                                    label = metric.label,
                                    value = metric.value,
                                    meta = metric.meta,
                                    color = metric.color,
                                    modifier = Modifier.weight(1f),
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}

private data class SummaryMetric(
    @StringRes val label: Int,
    val value: String,
    val meta: String,
    val color: Color = Color.Unspecified,
)

@Composable
internal fun HistoryChartCard(series: StatsSeries?, unavailable: Boolean) {
    val colors = DigitDnsTheme.colors
    PanelCard(modifier = Modifier.fillMaxWidth(), bottomPadding = 8.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            DashboardCardTitle(stringResource(R.string.history_chart_title), DashboardCard.HISTORY_CHART.icon)
            when {
                unavailable -> HistoryUnavailable()
                series == null -> HistoryWaiting()
                series.points.isEmpty() -> Text(
                    text = stringResource(R.string.history_no_data),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                !series.points.any { it.queries > 0 } -> Text(
                    text = stringResource(R.string.history_no_data),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                else -> {
                    Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                        HistoryLegend(stringResource(R.string.history_allowed), MaterialTheme.colorScheme.primary)
                        HistoryLegend(stringResource(R.string.history_blocked), MaterialTheme.colorScheme.error)
                        HistoryLegend(stringResource(R.string.history_proxied), colors.purple)
                    }
                    HistoryChart(series, colors = colors)
                    HistoryTimeAxis(series)
                }
            }
        }
    }
}

/**
 * Ranked cards share one row height so neighbours stay level. Protocols and
 * relay countries normally have very few values, so they show three rows while
 * the other ranked tables show five. Extra entries scroll inside the viewport;
 * the shell, header and "Other N" footer never move.
 */
private val HistoryTopCardBaseHeight = 76.dp
private val HistoryTopRowHeight = 20.dp
private val HistoryTopRowGap = 2.dp
private const val COMPACT_VISIBLE_ROWS = 3
private const val DEFAULT_VISIBLE_ROWS = 5

private fun TopDimension.visibleRows(): Int = when (this) {
    TopDimension.CLIENT, TopDimension.PROTOCOL, TopDimension.RELAY_COUNTRY -> COMPACT_VISIBLE_ROWS
    else -> DEFAULT_VISIBLE_ROWS
}

private fun TopDimension.listHeight(): Dp {
    val rows = visibleRows()
    return HistoryTopRowHeight * rows + HistoryTopRowGap * (rows - 1)
}

@Composable
internal fun HistoryTopCard(
    dimension: TopDimension,
    top: StatsTop?,
    unavailable: Boolean,
    serverCountry: String = "",
) {
    val listHeight = dimension.listHeight()
    // The relay title is long enough to need two lines on a 320 dp phone; give
    // that row (and Protocols beside it) a matching header slot so the two cards
    // stay level instead of one growing past the other.
    val wrappedHeader = dimension == TopDimension.PROTOCOL || dimension == TopDimension.RELAY_COUNTRY
    var byProvider by rememberSaveable(dimension) { mutableStateOf(false) }
    val displayTop = if (dimension == TopDimension.UPSTREAM && byProvider && top != null) {
        top.copy(items = groupUpstreamsByProvider(top.items))
    } else {
        top
    }
    PanelCard(
        modifier = Modifier
            .fillMaxWidth()
            .height(HistoryTopCardBaseHeight + listHeight + if (wrappedHeader) 16.dp else 0.dp),
        bottomPadding = 8.dp,
    ) {
        Column(
            modifier = Modifier.fillMaxSize(),
            verticalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            if (dimension == TopDimension.RELAY_COUNTRY) {
                RelayTitle(dimension.icon, dimension.tint())
            } else if (dimension == TopDimension.UPSTREAM) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    DashboardCardTitle(
                        text = stringResource(dimension.titleRes()),
                        icon = dimension.icon,
                        tint = dimension.tint(),
                        modifier = Modifier.weight(1f),
                    )
                    ProviderGroupChip(
                        active = byProvider,
                        onClick = { byProvider = !byProvider },
                    )
                }
            } else {
                DashboardCardTitle(
                    text = stringResource(dimension.titleRes()),
                    icon = dimension.icon,
                    tint = dimension.tint(),
                    modifier = if (wrappedHeader) Modifier.height(32.dp) else Modifier,
                )
            }
            when {
                unavailable -> HistoryUnavailable()
                displayTop == null || displayTop.items.isEmpty() -> Text(
                    text = stringResource(R.string.history_no_data),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                else -> {
                    HistoryTopEntries(dimension, displayTop, listHeight, serverCountry)
                    if (displayTop.otherCount > 0) {
                        Text(
                            text = stringResource(R.string.history_other, formatCount(displayTop.otherCount)),
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }
        }
    }
}

/** Two lines, because "Queries by Relay" does not fit beside the icon at 320 dp. */
@Composable
private fun RelayTitle(icon: androidx.compose.ui.graphics.vector.ImageVector, tint: androidx.compose.ui.graphics.Color) {
    Row(
        modifier = Modifier.height(32.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            imageVector = icon,
            contentDescription = null,
            tint = tint,
            modifier = Modifier.size(16.dp),
        )
        Box(Modifier.width(6.dp))
        Column {
            listOf(R.string.history_top_relay_line1, R.string.history_top_relay_line2).forEach { line ->
                Text(
                    text = stringResource(line).uppercase(Locale.US),
                    style = MaterialTheme.typography.titleSmall.copy(
                        fontWeight = FontWeight.Medium,
                        lineHeight = 16.sp,
                    ),
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                )
            }
        }
    }
}

@Composable
private fun HistoryTopEntries(
    dimension: TopDimension,
    top: StatsTop,
    listHeight: Dp,
    serverCountry: String,
) {
    // Only the entries move. The header above and the "Other N" footer below
    // stay pinned while this column is swiped.
    Column(
        modifier = Modifier
            .height(listHeight)
            .verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(HistoryTopRowGap),
    ) {
        top.items.forEachIndexed { index, item ->
            HistoryTopRow(dimension, index, item, serverCountry)
        }
    }
}

@Composable
private fun HistoryTopRow(
    dimension: TopDimension,
    index: Int,
    item: StatsTopItem,
    serverCountry: String,
) {
    val rowColor = if (index % 2 == 0) {
        MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.45f)
    } else {
        MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.18f)
    }
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .height(HistoryTopRowHeight)
            .clip(RoundedCornerShape(6.dp))
            .background(rowColor)
            .padding(horizontal = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Row(
            modifier = Modifier.weight(1f),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            when (dimension) {
                TopDimension.RELAY_COUNTRY -> RelayCountryValue(item.value, serverCountry)
                TopDimension.PROTOCOL -> Text(
                    text = protocolLabel(item.value),
                    style = MaterialTheme.typography.bodySmall,
                    color = protocolColor(item.value),
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                else -> {
                    if (dimension == TopDimension.UPSTREAM) {
                        providerDrawable(item.value)?.let { drawable ->
                            Image(
                                painter = painterResource(drawable),
                                contentDescription = null,
                                modifier = Modifier.size(14.dp),
                            )
                            Box(Modifier.width(5.dp))
                        }
                    }
                    Text(
                        text = item.value,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurface,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
            }
        }
        Box(Modifier.width(6.dp))
        Text(
            text = formatCount(item.count),
            style = MaterialTheme.typography.labelSmall.copy(fontFamily = PanelMono),
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun RelayCountryValue(value: String, serverCountry: String) {
    val code = value.trim().uppercase(Locale.ROOT)
    val flagCode = if (code == "LOCAL") serverCountry.trim().uppercase(Locale.ROOT) else code
    if (COUNTRY_CODE.matches(flagCode)) {
        CountryFlag(country = flagCode, width = 16.dp, height = 12.dp)
        Box(Modifier.width(6.dp))
    }
    Text(
        text = relayCountryLabel(value),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurface,
        maxLines = 1,
        overflow = TextOverflow.Ellipsis,
    )
}

/** Aggregate per-server history into provider totals for the By Provider view. */
private fun groupUpstreamsByProvider(items: List<StatsTopItem>): List<StatsTopItem> {
    val totals = linkedMapOf<String, Pair<String, Long>>()
    for (item in items) {
        val name = providerName(item.value) ?: "Other"
        val previous = totals[name]?.second ?: 0L
        totals[name] = name to (previous + item.count)
    }
    return totals.values
        .sortedByDescending { it.second }
        .map { (name, count) -> StatsTopItem(value = name, count = count) }
}

@Composable
private fun ProviderGroupChip(active: Boolean, onClick: () -> Unit) {
    val shape = RoundedCornerShape(999.dp)
    Box(
        modifier = Modifier
            .clip(shape)
            .background(
                if (active) DigitDnsTheme.colors.info.copy(alpha = 0.14f)
                else MaterialTheme.colorScheme.surface,
            )
            .border(
                width = 1.dp,
                color = if (active) DigitDnsTheme.colors.info.copy(alpha = 0.55f)
                else MaterialTheme.colorScheme.outlineVariant,
                shape = shape,
            )
            .clickable { onClick() }
            .padding(horizontal = 8.dp, vertical = 3.dp),
    ) {
        Text(
            text = "By Provider",
            style = MaterialTheme.typography.labelSmall.copy(fontSize = 9.sp, fontWeight = FontWeight.Bold),
            color = if (active) DigitDnsTheme.colors.info else MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
        )
    }
}

@Composable
private fun SummaryCell(
    @StringRes label: Int,
    value: String,
    meta: String,
    modifier: Modifier = Modifier,
    color: Color = Color.Unspecified,
) {
    val valueColor = if (color == Color.Unspecified) MaterialTheme.colorScheme.onSurface else color
    Column(modifier = modifier, verticalArrangement = Arrangement.spacedBy(1.dp)) {
        Text(
            text = stringResource(label).uppercase(Locale.US),
            style = MaterialTheme.typography.labelSmall.copy(fontSize = 9.sp, lineHeight = 11.sp),
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        Text(
            text = value,
            style = MaterialTheme.typography.titleMedium.copy(
                fontFamily = PanelMono,
                fontWeight = FontWeight.Bold,
                fontSize = 18.sp,
                lineHeight = 20.sp,
            ),
            color = valueColor,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        Text(
            text = meta,
            style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.sp, lineHeight = 12.sp),
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
    }
}

@Composable
private fun HistoryWaiting() {
    Text(
        text = stringResource(R.string.history_waiting),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

@Composable
private fun HistoryUnavailable() {
    Text(
        text = stringResource(R.string.history_unavailable),
        style = MaterialTheme.typography.bodySmall,
        color = DigitDnsTheme.colors.warning,
    )
}

@Composable
private fun HistoryLegend(label: String, color: Color) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Box(
            modifier = Modifier
                .size(8.dp)
                .clip(CircleShape)
                .background(color),
        )
        Box(Modifier.width(4.dp))
        Text(
            text = label,
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun HistoryChart(series: StatsSeries, colors: ExtendedColors) {
    val points = series.points
    // The 1h axis always spans the full hour even though the API only returns the
    // slots that have started. Future slots stay undrawn, so the line stops at
    // "now" instead of falling to zero.
    val slotCount = if (series.range == "1h") HOUR_SLOTS else points.size
    val totals = points.map { (it.allowed + it.blocked + it.proxied).toDouble() }
    val peak = totals.maxOrNull() ?: 0.0
    if (peak <= 0.0) {
        Text(
            text = stringResource(R.string.history_no_data),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        return
    }
    val primary = MaterialTheme.colorScheme.primary
    val danger = colors.danger
    val purple = colors.purple
    val border = colors.borderColor
    // Peak, midpoint and zero: three labels fit the 150 dp plot without turning
    // the axis into a wall of numbers, while still giving every value a scale.
    val ticks = listOf(peak, peak / 2, 0.0)
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .height(150.dp)
            .padding(top = 4.dp),
        horizontalArrangement = Arrangement.spacedBy(ChartAxisGap),
    ) {
        Column(
            modifier = Modifier.width(ChartAxisWidth).fillMaxHeight(),
            verticalArrangement = Arrangement.SpaceBetween,
            horizontalAlignment = Alignment.End,
        ) {
            ticks.forEach { value ->
                Text(
                    text = chartAxisLabel(value),
                    style = MaterialTheme.typography.labelSmall.copy(
                        fontFamily = PanelMono,
                        fontSize = 9.sp,
                        lineHeight = 10.sp,
                    ),
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                )
            }
        }
        Canvas(modifier = Modifier.weight(1f).fillMaxHeight()) {
            if (points.isEmpty()) return@Canvas
            drawChartGrid(ticks, peak, border)
            if (points.size == 1) {
                drawBucketBar(points.first(), peak, primary, danger, purple, slotCount)
            } else {
                drawStackedSeries(points, peak, primary, danger, purple, slotCount)
            }
        }
    }
}

/** Horizontal scale lines at the axis ticks, drawn under the fills. */
private fun DrawScope.drawChartGrid(ticks: List<Double>, peak: Double, color: Color) {
    ticks.forEach { value ->
        val y = chartY(value, peak, size.height)
        drawLine(
            color = color.copy(alpha = 0.45f),
            start = Offset(0f, y),
            end = Offset(size.width, y),
            strokeWidth = 1.dp.toPx(),
        )
    }
}

/** The 1h range is one bucket, so a centered stacked bar replaces the line. */
private fun DrawScope.drawBucketBar(
    point: StatsSeriesPoint,
    peak: Double,
    primary: Color,
    danger: Color,
    purple: Color,
    slotCount: Int,
) {
    // With a full-hour axis the single bar belongs at the first slot, not in the
    // middle of the empty remainder.
    val slotWidth = size.width / slotCount.coerceAtLeast(1)
    val barWidth = if (slotCount > 1) slotWidth * BUCKET_BAR_SLOT_FRACTION else size.width * BUCKET_BAR_WIDTH_FRACTION
    val left = if (slotCount > 1) (slotWidth - barWidth) / 2f else (size.width - barWidth) / 2f
    val allowedTop = point.allowed.toDouble()
    val blockedTop = allowedTop + point.blocked
    val proxiedTop = blockedTop + point.proxied
    fun band(lower: Double, upper: Double, color: Color) {
        val top = chartY(upper, peak, size.height)
        val bottom = chartY(lower, peak, size.height)
        drawRect(
            color = color,
            topLeft = Offset(left, top),
            size = Size(barWidth, bottom - top),
        )
    }
    band(0.0, allowedTop, primary.copy(alpha = 0.55f))
    if (point.blocked > 0) band(allowedTop, blockedTop, danger.copy(alpha = 0.65f))
    if (point.proxied > 0) band(blockedTop, proxiedTop, purple.copy(alpha = 0.65f))
}

/** Multi-point stacked bands and their top strokes. */
private fun DrawScope.drawStackedSeries(
    points: List<StatsSeriesPoint>,
    peak: Double,
    primary: Color,
    danger: Color,
    purple: Color,
    slotCount: Int,
) {
    val count = points.size
    val stepX = if (slotCount > 1) size.width / (slotCount - 1).toFloat() else size.width
    val allowed = points.map { it.allowed.toDouble() }
    val blocked = points.map { it.blocked.toDouble() }
    val proxied = points.map { it.proxied.toDouble() }
    val allowedTop = allowed
    val blockedTop = allowed.indices.map { allowed[it] + blocked[it] }
    val proxiedTop = blockedTop.indices.map { blockedTop[it] + proxied[it] }
    val zero = List(count) { 0.0 }

    fun yFor(value: Double): Float = chartY(value, peak, size.height)

    fun layer(lower: List<Double>, upper: List<Double>): Path {
        val path = Path()
        path.moveTo(0f, yFor(lower[0]))
        for (i in 1 until count) path.lineTo(i * stepX, yFor(lower[i]))
        for (i in count - 1 downTo 0) path.lineTo(i * stepX, yFor(upper[i]))
        path.close()
        return path
    }

    drawPath(layer(zero, allowedTop), color = primary.copy(alpha = 0.25f))
    drawPath(layer(allowedTop, blockedTop), color = danger.copy(alpha = 0.28f))
    drawPath(layer(blockedTop, proxiedTop), color = purple.copy(alpha = 0.28f))

    // Thin top strokes make each layer readable without a legend lookup.
    fun stroke(values: List<Double>, color: Color) {
        val path = Path()
        path.moveTo(0f, yFor(values[0]))
        for (i in 1 until count) path.lineTo(i * stepX, yFor(values[i]))
        drawPath(path, color = color, style = Stroke(width = 1.5.dp.toPx()))
    }
    stroke(allowedTop, primary)
    // A zero series would otherwise draw its stroke on top of the previous
    // layer's line and make an all-allowed window look proxied.
    if (blocked.any { it > 0 }) stroke(blockedTop, danger)
    if (proxied.any { it > 0 }) stroke(proxiedTop, purple)
}

/** Map a value to the plot's vertical coordinate. */
private fun chartY(value: Double, peak: Double, height: Float): Float =
    height - (value / peak * height).toFloat()

private val ChartAxisWidth = 34.dp
private val ChartAxisGap = 6.dp

private const val BUCKET_BAR_WIDTH_FRACTION = 0.18f
private const val BUCKET_BAR_SLOT_FRACTION = 0.55f
private const val AXIS_KILO = 1_000.0
private const val AXIS_MEGA = 1_000_000.0

/** Compact axis numbers: full counts to 999, then k/M suffixes. */
private fun chartAxisLabel(value: Double): String = when {
    value >= AXIS_MEGA -> String.format(Locale.US, "%.1fM", value / AXIS_MEGA)
    value >= AXIS_KILO -> String.format(Locale.US, "%.1fk", value / AXIS_KILO)
    else -> value.toLong().toString()
}

private val COUNTRY_CODE = Regex("^[A-Z]{2}$")

private fun relayCountryLabel(value: String): String {
    val code = value.trim().uppercase(Locale.ROOT)
    return when {
        code == "LOCAL" -> "Local"
        !COUNTRY_CODE.matches(code) -> value
        else -> Locale.Builder().setRegion(code).build()
            .getDisplayCountry(Locale.getDefault())
            .ifBlank { code }
    }
}

private fun percent(value: Double): String = String.format(Locale.US, "%.1f%%", value)

private fun historyLabel(iso: String, range: String, interval: String): String {
    val instant = runCatching { Instant.parse(iso) }.getOrNull() ?: return ""
    val pattern = when {
        range == "7d" -> "EEE d"
        range == "30d" || range == "90d" || interval == "day" -> "MMM d"
        else -> "HH:mm"
    }
    return DateTimeFormatter.ofPattern(pattern, Locale.getDefault())
        .withZone(ZoneId.systemDefault())
        .format(instant)
}

/** Four evenly spaced labels spanning the selected window, under the plot. */
@Composable
private fun HistoryTimeAxis(series: StatsSeries) {
    val first = series.points.firstOrNull()?.start
    val start = first?.let { runCatching { Instant.parse(it) }.getOrNull() }
    val slots = if (series.range == "1h") HOUR_SLOTS else series.points.size
    if (start == null || slots <= 0) return
    val step = when {
        series.range == "1h" -> 5L
        series.interval == "day" -> 24L * 60L
        else -> 60L
    }
    val tickCount = minOf(4, slots)
    val indices = (0 until tickCount).map { i -> i * (slots - 1) / (tickCount - 1).coerceAtLeast(1) }.distinct()
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(start = ChartAxisWidth + ChartAxisGap),
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        indices.forEach { index ->
            Text(
                text = historyLabel(
                    start.plusSeconds(index * step * 60).toString(),
                    series.range,
                    series.interval,
                ),
                style = MaterialTheme.typography.labelSmall.copy(
                    fontFamily = PanelMono,
                    fontSize = 9.sp,
                    lineHeight = 10.sp,
                ),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
            )
        }
    }
}

private const val HOUR_SLOTS = 12

internal fun HistoryRange.labelRes(): Int = when (this) {
    HistoryRange.LAST_1H -> R.string.history_range_1h
    HistoryRange.LAST_24H -> R.string.history_range_24h
    HistoryRange.LAST_7D -> R.string.history_range_7d
    HistoryRange.LAST_30D -> R.string.history_range_30d
    HistoryRange.LAST_90D -> R.string.history_range_90d
}

private fun TopDimension.titleRes(): Int = when (this) {
    TopDimension.DOMAIN -> R.string.history_top_domains
    TopDimension.BLOCKED_DOMAIN -> R.string.history_top_blocked
    TopDimension.CLIENT -> R.string.history_top_clients
    TopDimension.UPSTREAM -> R.string.history_top_upstreams
    TopDimension.PROTOCOL -> R.string.history_top_protocols
    TopDimension.RELAY_COUNTRY -> R.string.history_top_relay
}

private val TopDimension.icon
    get() = when (this) {
        TopDimension.DOMAIN -> Icons.Filled.Public
        TopDimension.BLOCKED_DOMAIN -> Icons.Filled.Block
        TopDimension.CLIENT -> Icons.Filled.People
        TopDimension.UPSTREAM -> Icons.Filled.Podcasts
        TopDimension.PROTOCOL -> Icons.Filled.Share
        TopDimension.RELAY_COUNTRY -> Icons.Filled.Dns
    }

@Composable
private fun TopDimension.tint(): Color = when (this) {
    TopDimension.DOMAIN -> MaterialTheme.colorScheme.primary
    TopDimension.BLOCKED_DOMAIN -> MaterialTheme.colorScheme.error
    TopDimension.CLIENT -> DigitDnsTheme.colors.success
    TopDimension.UPSTREAM -> DigitDnsTheme.colors.info
    TopDimension.PROTOCOL -> DigitDnsTheme.colors.purple
    TopDimension.RELAY_COUNTRY -> DigitDnsTheme.colors.warning
}
