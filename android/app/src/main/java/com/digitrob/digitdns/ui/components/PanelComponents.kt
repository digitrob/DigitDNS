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
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchColors
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.pulltorefresh.PullToRefreshDefaults
import androidx.compose.material3.pulltorefresh.rememberPullToRefreshState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.digitrob.digitdns.R
import com.digitrob.digitdns.data.HistoryClock
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.MetricNumber
import com.digitrob.digitdns.ui.theme.PanelMono
import kotlinx.coroutines.delay
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

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
 *
 * [verticalPadding] defaults to [contentPadding], so no existing card changes. Lists of
 * single-line rows pass a smaller value to tighten the top and bottom without moving the
 * left/right inset, and [contentAlignment] lets the row sit centred in the height those
 * rows share.
 *
 * [solid] swaps the translucent card colour for the opaque surface. A dialog uses it so
 * the page behind the panel is not visible through the text; ordinary dashboard cards
 * keep the glass treatment.
 */
@Composable
@Suppress("LongParameterList")
fun PanelCard(
    modifier: Modifier = Modifier,
    contentPadding: Dp = 16.dp,
    verticalPadding: Dp = contentPadding,
    bottomPadding: Dp = verticalPadding,
    highlighted: Boolean = false,
    contentAlignment: Alignment = Alignment.TopStart,
    solid: Boolean = false,
    content: @Composable () -> Unit,
) {
    val colors = DigitDnsTheme.colors
    val border = if (highlighted) MaterialTheme.colorScheme.primary else colors.borderColor
    Box(
        modifier = modifier
            .clip(RoundedCornerShape(12.dp))
            .background(if (solid) MaterialTheme.colorScheme.surface else colors.bgCard)
            .border(1.dp, border, RoundedCornerShape(12.dp))
            .padding(start = contentPadding, end = contentPadding, top = verticalPadding, bottom = bottomPadding),
        contentAlignment = contentAlignment,
    ) {
        content()
    }
}

/**
 * The panel's copy affordance: an icon button that turns into a green check for a
 * moment after the value has been handed to the clipboard.
 *
 * A toast alone is easy to miss on a phone — it appears at the bottom of the screen
 * while the operator's eye is on the row they just tapped, and a busy screen can
 * show three of them in a row. The check says "that one" at the point of the press,
 * which is the same reason the web panel swaps the glyph inside the copy box instead
 * of only flashing a corner toast.
 *
 * The revert is keyed on [copied] rather than fired from the click handler so a
 * second tap restarts the window: tapping twice restarts the effect, and the icon
 * stays a check until the last tap is 1.2 s old.
 */
@Composable
fun PanelCopyIconButton(
    contentDescription: String,
    onCopy: () -> Unit,
    modifier: Modifier = Modifier,
    tint: Color = MaterialTheme.colorScheme.onSurfaceVariant,
    enabled: Boolean = true,
    iconSize: Dp? = null,
) {
    var copied by remember { mutableStateOf(false) }
    LaunchedEffect(copied) {
        if (copied) {
            delay(1200)
            copied = false
        }
    }
    IconButton(
        onClick = {
            onCopy()
            copied = true
        },
        modifier = modifier,
        enabled = enabled,
    ) {
        Icon(
            imageVector = if (copied) Icons.Filled.Check else Icons.Filled.ContentCopy,
            contentDescription = contentDescription,
            tint = if (copied) DigitDnsTheme.colors.success else tint,
            // A 26-28 dp row keeps its 13-14 dp glyph; a default IconButton keeps its
            // 24 dp one. Without this the compact agent rows would grow a full-size
            // icon and push their own row height.
            modifier = if (iconSize != null) Modifier.size(iconSize) else Modifier,
        )
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

/**
 * The height every dashboard metric tile settles at. All six share it, so a grid row
 * never shows one card taller than its neighbour; 132 dp fits the tallest content
 * (Query Rate: label, value, unit and two supporting lines) at the default text scale.
 * The 8dp inset leaves the widest support line on one row even at 320dp.
 */
private val MetricCardHeight = 94.dp

/** The shared shell of a dashboard metric tile: compact padding, one height. */
@Composable
fun MetricCard(modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    PanelCard(
        modifier = modifier.fillMaxWidth().heightIn(min = MetricCardHeight),
        contentPadding = 8.dp,
        content = content,
    )
}

/** A metric's headline number. Never wraps: the unit lives on its own line. */
@Composable
fun MetricValue(text: String, color: Color) {
    Text(
        text = text,
        style = MetricNumber,
        color = color,
        maxLines = 1,
        softWrap = false,
        overflow = TextOverflow.Ellipsis,
    )
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
    maxLines: Int = 1,
) {
    // Match the web's .badge chip: 20px radius, 4x8 padding, 10px bold
    // uppercase text with the same 0.5px tracking.
    Row(
        modifier = modifier
            .clip(RoundedCornerShape(20.dp))
            .background(color.copy(alpha = 0.15f))
            .border(1.dp, color.copy(alpha = 0.4f), RoundedCornerShape(20.dp))
            .padding(horizontal = 8.dp, vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.Center,
    ) {
        Text(
            text = text.uppercase(),
            style = MaterialTheme.typography.labelSmall.copy(
                fontWeight = FontWeight.Bold,
                fontSize = 10.sp,
                lineHeight = 12.sp,
                letterSpacing = 0.5.sp,
            ),
            color = color,
            maxLines = maxLines,
            overflow = TextOverflow.Ellipsis,
            textAlign = androidx.compose.ui.text.style.TextAlign.Center,
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
    trailing: @Composable RowScope.() -> Unit = {},
    actions: @Composable RowScope.() -> Unit = {},
) {
    Row(
        modifier = modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 4.dp),
        verticalAlignment = Alignment.Top,
    ) {
        actions()
        Spacer(Modifier.weight(1f))
        if (showBadge) {
            // The dashboard puts the aggregate clock directly beside the pill; the
            // other live-backed screens mirror that so switching tabs does not move
            // the Online badge or its timestamp.
            Row(verticalAlignment = Alignment.CenterVertically) {
                trailing()
                HistoryClockText()
                Spacer(Modifier.width(8.dp))
                ConnectionStatusBadge()
            }
        }
    }
}

/**
 * The last successful aggregate-history refresh, in the same tiny mono style the
 * dashboard uses. It stays hidden until the dashboard has loaded history once.
 */
@Composable
private fun HistoryClockText() {
    val updatedAt by HistoryClock.updatedAtEpochMs.collectAsState()
    val epochMs = updatedAt ?: return
    PanelHeaderMeta(
        text = stringResource(
            R.string.history_updated,
            HEADER_CLOCK.format(Instant.ofEpochMilli(epochMs)),
        ),
    )
}

/** The one 8 sp meta line used by the updated clock and the live-log count. */
@Composable
internal fun PanelHeaderMeta(
    text: String,
    modifier: Modifier = Modifier,
    textAlign: TextAlign = TextAlign.Start,
) {
    Text(
        text = text,
        style = MaterialTheme.typography.labelSmall.copy(
            fontFamily = PanelMono,
            fontSize = 8.sp,
            lineHeight = 10.sp,
        ),
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        maxLines = 1,
        overflow = TextOverflow.Ellipsis,
        textAlign = textAlign,
        modifier = modifier,
    )
}

private val HEADER_CLOCK = DateTimeFormatter.ofPattern("HH:mm:ss")
    .withZone(ZoneId.systemDefault())

/**
 * The panel's compact switch control. Keep the thumb tied to the shared theme token;
 * callers may supply colors when the track itself carries a semantic state (for
 * example, the engine's green-on/red-off control).
 */
@Composable
fun PanelToggle(
    checked: Boolean,
    onCheckedChange: ((Boolean) -> Unit)?,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    colors: SwitchColors? = null,
) {
    val knob = DigitDnsTheme.colors.knob
    val trackOn = DigitDnsTheme.colors.success
    Switch(
        checked = checked,
        onCheckedChange = onCheckedChange,
        modifier = modifier,
        enabled = enabled,
        colors = colors ?: SwitchDefaults.colors(
            checkedThumbColor = knob,
            checkedTrackColor = trackOn,
            checkedBorderColor = trackOn,
            uncheckedThumbColor = knob,
        ),
    )
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
        PanelToggle(checked = checked, onCheckedChange = onCheckedChange, enabled = enabled)
    }
}

/**
 * A small uppercase section title, matching the panel's card headings.
 *
 * [icon] is the admin panel's per-card glyph: the dashboard draws its own copy of
 * this row at the same 14 dp / 6 dp spacing, and every surface that shows a card
 * heading should read as the same kind of header.
 */
@Composable
fun PanelSectionTitle(
    text: String,
    modifier: Modifier = Modifier,
    icon: ImageVector? = null,
) {
    Row(modifier = modifier, verticalAlignment = Alignment.CenterVertically) {
        icon?.let {
            Icon(
                imageVector = it,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.size(14.dp),
            )
            Spacer(Modifier.width(6.dp))
        }
        Text(
            text = text.uppercase(),
            style = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.Medium),
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/**
 * First-paint placeholder for server-backed settings pages. It draws the card
 * shapes the page is about to fill instead of a centred spinner, so the screen
 * never looks empty and the layout does not jump when the response lands.
 */
@Composable
fun SettingsLoadingSkeleton(
    modifier: Modifier = Modifier,
    cards: Int = 3,
) {
    Column(
        modifier = modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        repeat(cards) {
            PanelCard(modifier = Modifier.fillMaxWidth()) {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    SkeletonLine(widthFraction = 0.45f, height = 14.dp)
                    SkeletonLine(widthFraction = 0.92f, height = 10.dp)
                    SkeletonLine(widthFraction = 0.78f, height = 10.dp)
                    SkeletonLine(widthFraction = 0.64f, height = 10.dp)
                }
            }
        }
    }
}

@Composable
private fun SkeletonLine(widthFraction: Float, height: Dp) {
    Box(
        modifier = Modifier
            .fillMaxWidth(widthFraction)
            .height(height)
            .clip(RoundedCornerShape(4.dp))
            .background(MaterialTheme.colorScheme.surfaceVariant),
    )
}
