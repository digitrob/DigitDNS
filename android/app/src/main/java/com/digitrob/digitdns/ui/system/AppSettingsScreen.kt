package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.gestures.detectDragGesturesAfterLongPress
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.DragHandle
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.unit.dp
import androidx.compose.ui.zIndex
import kotlin.math.roundToInt
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.data.DashboardCard
import com.digitrob.digitdns.data.ThemeMode
import com.digitrob.digitdns.ui.components.PanelCard

/**
 * App-level settings: the theme choice, the dashboard card layout, and a pointer to the
 * per-server actions.
 *
 * The theme is a three-way choice (System / Dark / Light) rather than a switch: the
 * panel is dark by default, but a phone user expects the app to follow the system
 * until told otherwise, and a plain on/off switch cannot express "follow the system".
 */
@Composable
fun AppSettingsScreen(viewModel: AppSettingsViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        PanelCard(modifier = Modifier.fillMaxWidth()) {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text("Theme", style = MaterialTheme.typography.titleMedium)
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    ThemeMode.entries.forEach { mode ->
                        FilterChip(
                            selected = state.themeMode == mode,
                            onClick = { viewModel.setThemeMode(mode) },
                            label = { Text(mode.label()) },
                        )
                    }
                }
                Text(
                    text = "Follow system matches the rest of the phone. Dark and Light " +
                        "pin the app regardless of the OS setting.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }

        DashboardLayoutCard(
            order = state.dashboardOrder,
            disabled = state.dashboardDisabled,
            onToggle = viewModel::setCardEnabled,
            onMoveTo = viewModel::moveCardTo,
        )
    }
}

/**
 * The dashboard card layout editor: toggle visibility, and drag a card by its handle to
 * reorder. The list is fixed-height rows, so a long-press drag is translated into slot
 * changes as the finger crosses row boundaries.
 */
@Composable
private fun DashboardLayoutCard(
    order: List<DashboardCard>,
    disabled: Set<DashboardCard>,
    onToggle: (DashboardCard, Boolean) -> Unit,
    onMoveTo: (Int, Int) -> Unit,
) {
    var rowHeightPx by remember { mutableFloatStateOf(0f) }
    var draggingIndex by remember { mutableIntStateOf(NO_DRAG) }
    var dragOffset by remember { mutableFloatStateOf(0f) }
    val currentOrder by rememberUpdatedState(order)
    val currentOnMoveTo by rememberUpdatedState(onMoveTo)

    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text("Dashboard cards", style = MaterialTheme.typography.titleMedium)
            Text(
                text = "Choose which cards the dashboard shows, and their order. " +
                    "Long-press a handle to drag a card.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                order.forEachIndexed { index, card ->
                    key(card) {
                    val isDragging = index == draggingIndex
                    DashboardCardRow(
                        card = card,
                        enabled = card !in disabled,
                        isDragging = isDragging,
                        dragOffset = if (isDragging) dragOffset else 0f,
                        onToggle = { onToggle(card, it) },
                        onMeasured = { if (rowHeightPx == 0f) rowHeightPx = it.toFloat() },
                        dragHandle = {
                            Modifier.pointerInput(card) {
                                detectDragGesturesAfterLongPress(
                                    onDragStart = {
                                        draggingIndex = currentOrder.indexOf(card)
                                        dragOffset = 0f
                                    },
                                    onDragEnd = {
                                        draggingIndex = NO_DRAG
                                        dragOffset = 0f
                                    },
                                    onDragCancel = {
                                        draggingIndex = NO_DRAG
                                        dragOffset = 0f
                                    },
                                    onDrag = { change, delta ->
                                        change.consume()
                                        if (rowHeightPx <= 0f) return@detectDragGesturesAfterLongPress
                                        dragOffset += delta.y
                                        val steps = (dragOffset / rowHeightPx).roundToInt()
                                        if (steps != 0) {
                                            val current = currentOrder.indexOf(card)
                                            val target = (current + steps).coerceIn(currentOrder.indices)
                                            if (target != current) {
                                                currentOnMoveTo(current, target)
                                                dragOffset -= (target - current) * rowHeightPx
                                                draggingIndex = target
                                            }
                                        }
                                    },
                                )
                            }
                        },
                    )
                }
                    }
            }
        }
    }
}

private const val NO_DRAG = -1

@Composable
private fun DashboardCardRow(
    card: DashboardCard,
    enabled: Boolean,
    isDragging: Boolean,
    dragOffset: Float,
    onToggle: (Boolean) -> Unit,
    onMeasured: (Int) -> Unit,
    dragHandle: () -> Modifier,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .onSizeChanged { onMeasured(it.height) }
            .zIndex(if (isDragging) 1f else 0f)
            .graphicsLayer { translationY = dragOffset }
            .padding(vertical = 2.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            Icons.Filled.DragHandle,
            contentDescription = "Drag to reorder ${card.label()}",
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(24.dp).then(dragHandle()),
        )
        Spacer(Modifier.width(8.dp))
        Text(
            text = card.label(),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurface,
            modifier = Modifier.weight(1f),
        )
        Switch(checked = enabled, onCheckedChange = onToggle)
    }
}

private fun DashboardCard.label(): String = when (this) {
    DashboardCard.QUERY_RATE -> "Query rate"
    DashboardCard.RAM_CACHE -> "RAM cache"
    DashboardCard.SNI_PROXY -> "SNI proxy"
    DashboardCard.CPU_USAGE -> "CPU usage"
    DashboardCard.RAM_USAGE -> "DigitDNS RAM usage"
    DashboardCard.LIVE_TRAFFIC -> "Live traffic"
    DashboardCard.TELEMETRY_CHART -> "Query-rate chart"
    DashboardCard.UPSTREAM_RACERS -> "Upstream racers"
    DashboardCard.RESOLVER_LATENCY -> "Resolver latency"
}

private fun ThemeMode.label(): String = when (this) {
    ThemeMode.SYSTEM -> "Follow system"
    ThemeMode.DARK -> "Dark"
    ThemeMode.LIGHT -> "Light"
}
