package com.digitrob.digitdns.ui.theme

import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color

/**
 * The panel tokens Material 3's own [androidx.compose.material3.ColorScheme] has no
 * slot for: the glass card surfaces, the cyan border glow, and the semantic accent
 * set the badges and status dots read from. They are surfaced through a
 * [androidx.compose.runtime.CompositionLocal] rather than bolted onto the M3 scheme,
 * because M3 would misread them — `--accent-purple` is not the secondary role, it is
 * "here is the purple chip", and folding it into `secondary` would tint every
 * secondary component with it.
 *
 * Every field is dark/light resolved by [digitDnsExtendedColors] so a composable
 * reads `DigitDnsTheme.colors.success` and gets the right value for the theme in force.
 */
@Immutable
data class ExtendedColors(
    val bgMain: Color,
    val bgCard: Color,
    val bgCardHover: Color,
    val bgTile: Color,
    val borderColor: Color,
    val borderGlow: Color,
    val textMain: Color,
    val textMuted: Color,
    val success: Color,
    val warning: Color,
    val danger: Color,
    val info: Color,
    val purple: Color,
    val discord: Color,
)

internal fun digitDnsExtendedColors(dark: Boolean): ExtendedColors = if (dark) {
    ExtendedColors(
        bgMain = PanelColors.Slate900Dark,
        bgCard = PanelColors.BgCardDark,
        bgCardHover = PanelColors.BgCardHoverDark,
        bgTile = PanelColors.BgTileDark,
        borderColor = PanelColors.BorderColorDark,
        borderGlow = PanelColors.BorderGlowDark,
        textMain = PanelColors.Slate50Dark,
        textMuted = PanelColors.Slate400Dark,
        success = PanelColors.AccentGreenDark,
        warning = PanelColors.AccentAmberDark,
        danger = PanelColors.AccentRedDark,
        info = PanelColors.Cyan400Dark,
        purple = PanelColors.AccentPurpleDark,
        discord = PanelColors.AccentDiscord,
    )
} else {
    ExtendedColors(
        bgMain = PanelColors.Slate900Light,
        bgCard = PanelColors.BgCardLight,
        bgCardHover = PanelColors.BgCardHoverLight,
        bgTile = PanelColors.BgTileLight,
        borderColor = PanelColors.BorderColorLight,
        borderGlow = PanelColors.BorderGlowLight,
        textMain = PanelColors.Slate50Light,
        textMuted = PanelColors.Slate400Light,
        success = PanelColors.AccentGreenLight,
        warning = PanelColors.AccentAmberLight,
        danger = PanelColors.AccentRedLight,
        info = PanelColors.Cyan400Light,
        purple = PanelColors.AccentPurpleLight,
        discord = PanelColors.AccentDiscord,
    )
}

/** The in-force [ExtendedColors]. Read it as `DigitDnsTheme.colors`. */
val LocalExtendedColors = staticCompositionLocalOf { digitDnsExtendedColors(dark = true) }
