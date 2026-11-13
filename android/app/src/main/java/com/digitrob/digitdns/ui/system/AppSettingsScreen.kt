package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.gestures.detectDragGesturesAfterLongPress
import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
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
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
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
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.compose.ui.zIndex
import kotlin.math.roundToInt
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.R
import com.digitrob.digitdns.core.security.BiometricAvailability
import com.digitrob.digitdns.data.DashboardCard
import com.digitrob.digitdns.data.ThemeMode
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelToggle
import com.digitrob.digitdns.ui.security.findFragmentActivity

/**
 * App-level settings: the theme choice, the dashboard card layout, and a pointer to the
 * per-server actions.
 *
 * The theme is a four-way choice (System / Dark / Grey / Light) rather than a switch:
 * the panel is dark by default, but a phone user expects the app to follow the system
 * until told otherwise, and a plain on/off switch cannot express "follow the system".
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun AppSettingsScreen(viewModel: AppSettingsViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val activity = remember(context) { context.findFragmentActivity() }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        PanelCard(modifier = Modifier.fillMaxWidth()) {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(stringResource(R.string.settings_theme), style = MaterialTheme.typography.titleMedium)
                FlowRow(
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    ThemeMode.entries.forEach { mode ->
                        FilterChip(
                            selected = state.themeMode == mode,
                            onClick = { viewModel.setThemeMode(mode) },
                            label = { Text(stringResource(mode.labelRes())) },
                        )
                    }
                }
                Text(
                    text = stringResource(R.string.settings_theme_help),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )

                BiometricLoginRow(
                    enabled = state.biometricEnabled,
                    availability = state.biometricAvailability,
                    message = state.biometricMessage,
                    onToggle = { viewModel.setBiometricEnabled(activity, it) },
                )
            }
        }

        DashboardLayoutCard(
            order = state.dashboardOrder,
            disabled = state.dashboardDisabled,
            onToggle = viewModel::setCardEnabled,
            onMoveTo = viewModel::moveCardTo,
            onReset = viewModel::resetDashboardLayout,
        )
    }
}

/**
 * The app-lock row. It sits directly below the theme choice; the switch is disabled
 * when the device has no usable biometric or device credential, but remains usable
 * to turn an already-enabled lock off if the hardware changed later.
 */
@Composable
private fun BiometricLoginRow(
    enabled: Boolean,
    availability: BiometricAvailability,
    message: String?,
    onToggle: (Boolean) -> Unit,
) {
    HorizontalDivider(modifier = Modifier.padding(vertical = 4.dp))
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = stringResource(R.string.settings_biometric),
                style = MaterialTheme.typography.titleSmall,
            )
            Text(
                text = stringResource(R.string.settings_biometric_help),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (!enabled && availability != BiometricAvailability.AVAILABLE) {
                Text(
                    text = stringResource(R.string.settings_biometric_unavailable),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error,
                )
            }
            message?.let {
                Text(
                    text = it,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error,
                )
            }
        }
        PanelToggle(
            checked = enabled,
            onCheckedChange = onToggle,
            enabled = availability == BiometricAvailability.AVAILABLE || enabled,
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
    onReset: () -> Unit,
) {
    var rowHeightPx by remember { mutableFloatStateOf(0f) }
    var draggingIndex by remember { mutableIntStateOf(NO_DRAG) }
    var dragOffset by remember { mutableFloatStateOf(0f) }
    val currentOrder by rememberUpdatedState(order)
    val currentOnMoveTo by rememberUpdatedState(onMoveTo)

    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(stringResource(R.string.settings_dashboard_cards), style = MaterialTheme.typography.titleMedium)
            Row(horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
                Text(
                    text = stringResource(R.string.settings_dashboard_cards_help),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.weight(1f),
                )
                TextButton(onClick = onReset) { Text(stringResource(R.string.settings_reset_layout)) }
            }
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
            contentDescription = stringResource(R.string.card_drag_reorder, stringResource(card.labelRes())),
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(24.dp).then(dragHandle()),
        )
        Spacer(Modifier.width(8.dp))
        Text(
            text = stringResource(card.labelRes()),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurface,
            modifier = Modifier.weight(1f),
        )
        PanelToggle(checked = enabled, onCheckedChange = onToggle)
    }
}

private val dashboardCardLabels = mapOf(
    DashboardCard.QUERY_RATE to R.string.card_query_rate,
    DashboardCard.RAM_CACHE to R.string.card_ram_cache,
    DashboardCard.SNI_PROXY to R.string.card_sni_proxy,
    DashboardCard.CPU_USAGE to R.string.card_cpu_usage,
    DashboardCard.RAM_USAGE to R.string.card_ram_usage,
    DashboardCard.LIVE_TRAFFIC to R.string.card_live_traffic,
    DashboardCard.TELEMETRY_CHART to R.string.card_telemetry_chart,
    DashboardCard.UPSTREAM_RACERS to R.string.card_upstream_racers,
    DashboardCard.RESOLVER_LATENCY to R.string.card_resolver_latency,
    DashboardCard.HISTORY_SUMMARY to R.string.card_history_summary,
    DashboardCard.HISTORY_CHART to R.string.card_history_chart,
    DashboardCard.HISTORY_TOP_DOMAINS to R.string.history_top_domains,
    DashboardCard.HISTORY_TOP_BLOCKED to R.string.history_top_blocked,
    DashboardCard.HISTORY_TOP_CLIENTS to R.string.history_top_clients,
    DashboardCard.HISTORY_TOP_UPSTREAMS to R.string.history_top_upstreams,
    DashboardCard.HISTORY_TOP_PROTOCOLS to R.string.history_top_protocols,
    DashboardCard.HISTORY_TOP_RELAY to R.string.history_top_relay,
)

@StringRes
private fun DashboardCard.labelRes(): Int = dashboardCardLabels.getValue(this)

@StringRes
private fun ThemeMode.labelRes(): Int = when (this) {
    ThemeMode.SYSTEM -> R.string.theme_system
    ThemeMode.DARK -> R.string.theme_dark
    ThemeMode.GREY -> R.string.theme_grey
    ThemeMode.LIGHT -> R.string.theme_light
}
