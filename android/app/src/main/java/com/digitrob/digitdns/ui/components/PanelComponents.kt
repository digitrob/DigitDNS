package com.digitrob.digitdns.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.pulltorefresh.PullToRefreshDefaults
import androidx.compose.material3.pulltorefresh.rememberPullToRefreshState
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono

/**
 * The pieces the panel's cards are built from — a glass card, a stat tile, a quota
 * bar, a status badge — ported so the app reads as the same product. They live here
 * rather than per-screen because the dashboard, the client list and the client detail
 * all draw the same shapes, and a diverging border treatment between them would be
 * the first sign the app was a separate thing.
 */

/**
 * A panel card: the translucent surface with the cyan hairline border. [contentPadding]
 * is exposed for the rare card whose content is itself a row of equal-width tiles (the
 * resolver-latency block): the default 16 dp edge leaves too little width for four
 * cells once their own padding is subtracted, so those callers pass a tighter inset.
 */
@Composable
fun PanelCard(
    modifier: Modifier = Modifier,
    contentPadding: Dp = 16.dp,
    highlighted: Boolean = false,
    content: @Composable () -> Unit,
) {
    val colors = DigitDnsTheme.colors
    val border = if (highlighted) MaterialTheme.colorScheme.primary else colors.borderColor
    Box(
        modifier = modifier
            .clip(RoundedCornerShape(12.dp))
            .background(colors.bgCard)
            .border(1.dp, border, RoundedCornerShape(12.dp))
            .padding(contentPadding),
    ) {
        content()
    }
}

/**
 * The shared row geometry for swipe-to-delete lists. A comfortable swipe target, tall
 * enough that a finger cannot start on one row and settle on the next; every list that
 * carries a swipe uses the same value so the rows match page to page.
 */
object PanelRowDefaults {
    val height = 56.dp
}

/** A labelled metric, the unit of the dashboard grid. */
@Composable
fun StatTile(
    label: String,
    value: String,
    modifier: Modifier = Modifier,
    valueColor: Color = MaterialTheme.colorScheme.onSurface,
    caption: String? = null,
) {
    PanelCard(modifier = modifier) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(
                text = label.uppercase(),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                text = value,
                style = MaterialTheme.typography.headlineSmall.copy(fontFamily = PanelMono),
                color = valueColor,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            caption?.let {
                Text(
                    text = it,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
    }
}

/**
 * A quota bar. The colour steps with usage (green → amber → red), matching the
 * threshold logic the panel uses so a nearly-exhausted account looks the same in both
 * places.
 */
@Composable
fun QuotaBar(
    fraction: Float,
    modifier: Modifier = Modifier,
) {
    val colors = DigitDnsTheme.colors
    val barColor = when {
        fraction >= 0.9f -> colors.danger
        fraction >= 0.7f -> colors.warning
        else -> colors.success
    }
    Box(
        modifier = modifier
            .fillMaxWidth()
            .height(6.dp)
            .clip(RoundedCornerShape(3.dp))
            .background(colors.borderColor),
    ) {
        Box(
            modifier = Modifier
                .fillMaxWidth(fraction.coerceIn(0f, 1f))
                .height(6.dp)
                .clip(RoundedCornerShape(3.dp))
                .background(barColor),
        )
    }
}

/** A small pill: green "Active", amber "Expiring", red "Disabled"/"Expired". */
@Composable
fun StatusBadge(
    text: String,
    color: Color,
    modifier: Modifier = Modifier,
) {
    Row(
        modifier = modifier
            .clip(RoundedCornerShape(999.dp))
            .background(color.copy(alpha = 0.15f))
            .border(1.dp, color.copy(alpha = 0.4f), RoundedCornerShape(999.dp))
            .padding(horizontal = 10.dp, vertical = 3.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = text,
            style = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.Medium),
            color = color,
        )
    }
}

/**
 * The pull-to-refresh wrapper every server-backed screen uses, so the gesture and its
 * indicator look identical everywhere. The caller keeps the content; this adds the
 * swipe affordance and an indicator tinted to the panel's accent. Material3's box
 * already slides the content down with the pull, so the motion is standard.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PanelRefreshBox(
    isRefreshing: Boolean,
    onRefresh: () -> Unit,
    modifier: Modifier = Modifier,
    content: @Composable () -> Unit,
) {
    // One state instance, driven by the box and read by the indicator. It used to be
    // minted inside the indicator lambda, where nothing ever advanced it, so the pull
    // arc stayed at distanceFraction 0 and the gesture looked dead until release.
    val state = rememberPullToRefreshState()
    PullToRefreshBox(
        isRefreshing = isRefreshing,
        onRefresh = onRefresh,
        modifier = modifier,
        state = state,
        indicator = {
            PullToRefreshDefaults.Indicator(
                state = state,
                isRefreshing = isRefreshing,
                containerColor = MaterialTheme.colorScheme.surface,
                color = MaterialTheme.colorScheme.primary,
                modifier = Modifier.align(Alignment.TopCenter),
            )
        },
    ) {
        content()
    }
}

/**
 * The standard screen top strip: an optional row of screen actions on the left and the
 * live connection pill pinned to the far top-right corner, above the scrolling content.
 *
 * The page carries no title: the bottom tab (or the Settings chip row) already names it,
 * so a second copy only wasted vertical space. The badge is always the outermost
 * element — screen actions sit to its left — so an operator finds the connection state
 * in the exact same corner on every live-backed screen.
 */
@Composable
fun PanelScreenHeader(
    modifier: Modifier = Modifier,
    showBadge: Boolean = true,
    actions: @Composable RowScope.() -> Unit = {},
) {
    Row(
        modifier = modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 4.dp),
        verticalAlignment = Alignment.Top,
    ) {
        actions()
        Spacer(Modifier.weight(1f))
        if (showBadge) {
            ConnectionStatusBadge()
        }
    }
}

/**
 * The panel's switch: a labelled toggle row. Shared so every settings screen turns a
 * boolean on and off the same way, rather than each growing its own label/switch pair.
 */
@Composable
fun PanelSwitch(
    label: String,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    supporting: String? = null,
) {
    Row(
        modifier = modifier.fillMaxWidth().padding(vertical = 2.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = label,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurface,
            )
            supporting?.let {
                Text(
                    text = it,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        Switch(checked = checked, onCheckedChange = onCheckedChange, enabled = enabled)
    }
}

/** A small uppercase section title, matching the panel's card headings. */
@Composable
fun PanelSectionTitle(text: String, modifier: Modifier = Modifier) {
    Text(
        text = text.uppercase(),
        style = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.Medium),
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = modifier,
    )
}
