package com.digitrob.digitdns.ui.theme

import androidx.compose.material3.Typography
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp

/**
 * The panel sets three families (web/css/style.css `--font-body`, `--font-heading`,
 * `--font-mono`): Vazirmatn for everything, and JetBrains Mono for tokens, UUIDs and
 * the numeric columns. Neither webfont ships in the app — bundling two variable
 * fonts for a control panel is weight the panel itself only pays because it renders
 * in a browser — so body and heading fall back to the platform default and mono
 * keeps Android's own monospace, which is the same guarantee the panel's fallback
 * stack makes ("the no-webfont path still looks deliberate").
 *
 * The sizes track the panel's scale but sit one step larger: the panel's 12px base
 * body is comfortable on a desktop monitor and cramped on a phone held at arm's
 * length, so the body/label ramps start at 13sp while the headings keep the panel's
 * own sizes. This is the one place the app deliberately diverges from the panel.
 */
internal val PanelMono = FontFamily.Monospace

internal val DigitDnsTypography = Typography(
    displaySmall = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Bold,
        fontSize = 28.sp,
        lineHeight = 34.sp,
    ),
    headlineMedium = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Bold,
        fontSize = 22.sp,
        lineHeight = 28.sp,
    ),
    headlineSmall = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.SemiBold,
        fontSize = 18.sp,
        lineHeight = 24.sp,
    ),
    titleLarge = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.SemiBold,
        fontSize = 17.sp,
        lineHeight = 23.sp,
    ),
    titleMedium = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.SemiBold,
        fontSize = 15.sp,
        lineHeight = 21.sp,
    ),
    bodyLarge = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Normal,
        fontSize = 15.sp,
        lineHeight = 21.sp,
    ),
    bodyMedium = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Normal,
        fontSize = 14.sp,
        lineHeight = 19.sp,
    ),
    bodySmall = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Normal,
        fontSize = 13.sp,
        lineHeight = 17.sp,
    ),
    labelLarge = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.SemiBold,
        fontSize = 14.sp,
        lineHeight = 19.sp,
    ),
    labelMedium = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Medium,
        fontSize = 12.sp,
        lineHeight = 15.sp,
    ),
)

/**
 * The one size every dashboard metric number uses. The grid's tiles are two columns
 * wide on a phone, so a 32sp number left almost no room for the card's supporting
 * lines — the value is the card's headline, not a wall display, and 22sp keeps the
 * four-line tiles inside a compact, equal-height card.
 */
internal val MetricNumber = TextStyle(
    fontFamily = PanelMono,
    fontWeight = FontWeight.SemiBold,
    fontSize = 22.sp,
    lineHeight = 25.sp,
)

/**
 * The one size every metric's supporting line uses. BodySmall sits at 13sp for prose,
 * which is too wide for two stats sharing a 142dp card; the labels here are short
 * and dense, so they get their own tighter step rather than a per-card override.
 */
internal val MetricMeta = TextStyle(
    fontFamily = FontFamily.Default,
    fontWeight = FontWeight.Normal,
    fontSize = 10.sp,
    lineHeight = 12.sp,
)

/**
 * The metric tiles' header label. One step below the shared label ramp: the six
 * compact tiles trade three pixels of header for a shorter card, while the
 * telemetry, resolver and upstream headers keep the larger shared label.
 */
internal val MetricLabel = TextStyle(
    fontFamily = FontFamily.Default,
    fontWeight = FontWeight.Medium,
    fontSize = 10.sp,
    lineHeight = 13.sp,
)
