package com.digitrob.digitdns.ui.dashboard

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Podcasts
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import com.digitrob.digitdns.core.network.UpstreamStat
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelRowDefaults
import com.digitrob.digitdns.ui.components.SwipeToDeleteRow
import com.digitrob.digitdns.ui.formatCount
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.ExtendedColors
import com.digitrob.digitdns.ui.theme.PanelMono
import kotlinx.coroutines.launch

/**
 * The Upstream Racers tile: the resolver pool ranked by latency, with the panel's two
 * mutating actions inline — add an upstream, and remove one behind the same
 * confirmation every other swipe-to-delete row in the app uses.
 *
 * Kept in its own file rather than alongside the other dashboard cards because it is the
 * only tile with write actions: it owns transient form state and a dialog, which the
 * read-only stat tiles do not.
 */
@Composable
internal fun UpstreamRacersCard(
    upstreams: List<UpstreamStat>,
    isBenchmarking: Boolean,
    onBenchmark: () -> Unit,
    onAdd: suspend (String) -> String?,
    onRemove: suspend (String) -> String?,
    /** False while the dashboard grid is scrolling: reading the page must not arm a delete. */
    swipeEnabled: Boolean = true,
) {
    val colors = DigitDnsTheme.colors
    var newAddress by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            RacerHeader(isBenchmarking = isBenchmarking, onBenchmark = onBenchmark)

            // Inline add, mirroring the panel's racer input.
            UpstreamAddRow(
                value = newAddress,
                onValueChange = { newAddress = it; error = null },
                busy = busy,
                onSubmit = {
                    val addr = newAddress.trim()
                    if (addr.isNotBlank()) {
                        scope.launch {
                            busy = true
                            error = onAdd(addr)
                            busy = false
                            if (error == null) newAddress = ""
                        }
                    }
                },
            )
            error?.let {
                Text(
                    text = it,
                    style = MaterialTheme.typography.bodySmall,
                    color = colors.danger,
                )
            }

            if (upstreams.isEmpty()) {
                Text(
                    text = "No upstreams reported yet.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            } else {
                upstreams.forEachIndexed { index, stat ->
                    UpstreamRow(
                        rank = index + 1,
                        stat = stat,
                        swipeEnabled = swipeEnabled,
                        onRemove = {
                            scope.launch {
                                busy = true
                                error = onRemove(stat.address)
                                busy = false
                            }
                        },
                    )
                }
            }
        }
    }
}

/** The racer card's title row: icon, label and Benchmark action (or its spinner). */
@Composable
private fun RacerHeader(isBenchmarking: Boolean, onBenchmark: () -> Unit) {
    val colors = DigitDnsTheme.colors
    Row(verticalAlignment = Alignment.CenterVertically) {
        Icon(
            Icons.Filled.Podcasts,
            contentDescription = null,
            tint = colors.warning,
            modifier = Modifier.size(18.dp),
        )
        Spacer(Modifier.width(6.dp))
        Label("Upstream Servers", modifier = Modifier.weight(1f))
        // Fixed-height slot so swapping the Benchmark button for its spinner does not
        // change the header row height and make the card jump while a run is in flight.
        Box(
            modifier = Modifier.height(40.dp),
            contentAlignment = Alignment.Center,
        ) {
            if (isBenchmarking) {
                CircularProgressIndicator(
                    modifier = Modifier.size(16.dp),
                    strokeWidth = 2.dp,
                    color = MaterialTheme.colorScheme.primary,
                )
            } else {
                TextButton(onClick = onBenchmark) {
                    Icon(
                        Icons.Filled.PlayArrow,
                        contentDescription = null,
                        modifier = Modifier.size(16.dp),
                    )
                    Spacer(Modifier.width(4.dp))
                    Text("Benchmark")
                }
            }
        }
    }
}

/** The racer card's inline add row: an address field and an Add button. */
@Composable
private fun UpstreamAddRow(
    value: String,
    onValueChange: (String) -> Unit,
    busy: Boolean,
    onSubmit: () -> Unit,
) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        OutlinedTextField(
            value = value,
            onValueChange = onValueChange,
            placeholder = { Text("8.8.8.8, tls://dns.google") },
            singleLine = true,
            enabled = !busy,
            modifier = Modifier.weight(1f),
        )
        Spacer(Modifier.width(8.dp))
        IconButton(onClick = onSubmit, enabled = !busy && value.isNotBlank()) {
            Icon(
                Icons.Filled.Add,
                contentDescription = "Add upstream",
                tint = MaterialTheme.colorScheme.primary,
            )
        }
    }
}

@Composable
private fun UpstreamRow(
    rank: Int,
    stat: UpstreamStat,
    swipeEnabled: Boolean,
    onRemove: () -> Unit,
) {
    val colors = DigitDnsTheme.colors
    SwipeToDeleteRow(
        onDelete = onRemove,
        confirmTitle = "Remove upstream?",
        confirmText = "${stat.address} will be removed from the resolver pool.",
        confirmLabel = "Remove",
        // 0.85 of the row, not the panel's 0.45: the card sits inside the scrolling
        // dashboard grid, and a diagonal scroll used to settle as a swipe and ask to
        // delete an upstream nobody meant to touch.
        threshold = 0.85f,
        enabled = swipeEnabled,
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .heightIn(min = PanelRowDefaults.height)
                .background(MaterialTheme.colorScheme.surface)
                .padding(vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                text = "$rank",
                style = MaterialTheme.typography.bodyMedium.copy(fontFamily = PanelMono),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.width(20.dp),
            )
            // The status dot mirrors the panel: green when measured and fast, amber above
            // 40 ms, red above 80 ms, grey when nothing has timed it yet.
            Box(
                modifier = Modifier
                    .size(8.dp)
                    .clip(CircleShape)
                    .background(racerDotColor(stat, colors)),
            )
            Spacer(Modifier.width(8.dp))
            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text = stat.address,
                    style = MaterialTheme.typography.bodyMedium.copy(fontFamily = PanelMono),
                    color = MaterialTheme.colorScheme.onSurface,
                    maxLines = 1,
                )
                Text(
                    text = "${formatCount(stat.hits)} hits · ${formatCount(stat.errors)} errors",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Text(
                text = if (stat.isMeasured) "%.1f ms".format(stat.displayLatencyMs) else "—",
                style = MaterialTheme.typography.bodyMedium.copy(fontFamily = PanelMono),
                color = racerDotColor(stat, colors),
            )
        }
    }
}

/** The racer status dot, by the panel's measured-latency thresholds. */
private fun racerDotColor(stat: UpstreamStat, colors: ExtendedColors): Color =
    when {
        !stat.isMeasured -> colors.textMuted
        stat.displayLatencyMs > RACER_DANGER_MS -> colors.danger
        stat.displayLatencyMs > RACER_WARNING_MS -> colors.warning
        else -> colors.success
    }

/** Racer dot: at/above this measured latency reads amber, matching the panel. */
private const val RACER_WARNING_MS = 40.0

/** Racer dot: at/above this measured latency reads red, matching the panel. */
private const val RACER_DANGER_MS = 80.0
