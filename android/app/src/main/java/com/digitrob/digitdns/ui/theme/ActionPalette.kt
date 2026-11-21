package com.digitrob.digitdns.ui.theme

import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import com.digitrob.digitdns.ui.stream.QueryActionStyle
import com.digitrob.digitdns.ui.stream.QueryActionTone
import com.digitrob.digitdns.ui.stream.queryActionStyle

/**
 * The one tone-to-colour mapping for query action pills.
 *
 * The live log used to inline this `when` and the history screen carried a second
 * local table that disagreed with it: `CUSTOM` and `QUOTA` were amber in history but
 * purple/red in the live log, and the proxy sinks got different colours again. Both
 * screens now read the colour from the same [QueryActionTone] the label comes from,
 * so an action cannot be described one way and coloured another.
 */
fun actionToneColor(tone: QueryActionTone, colors: ExtendedColors): Color = when (tone) {
    QueryActionTone.Info -> colors.info
    QueryActionTone.Proxy -> colors.proxy
    QueryActionTone.Danger -> colors.danger
    QueryActionTone.Success -> colors.success
    QueryActionTone.Warning -> colors.warning
    QueryActionTone.Purple -> colors.purple
    QueryActionTone.Muted -> colors.textMuted
}

@Composable
fun actionToneColor(tone: QueryActionTone): Color = actionToneColor(tone, DigitDnsTheme.colors)

/** Pure form used by unit tests and by callers that already hold a theme snapshot. */
fun actionBadgeColor(action: String, colors: ExtendedColors): Color =
    actionToneColor(queryActionStyle(action).tone, colors)

fun actionStyleColor(style: QueryActionStyle, colors: ExtendedColors): Color =
    actionToneColor(style.tone, colors)

/** The badge colour for one raw server action, resolved through its shared style. */
@Composable
fun actionBadgeColor(action: String): Color = actionBadgeColor(action, DigitDnsTheme.colors)

/** Variant for screens that already parsed the action into a [QueryActionStyle]. */
@Composable
fun actionBadgeColor(style: QueryActionStyle): Color = actionStyleColor(style, DigitDnsTheme.colors)
