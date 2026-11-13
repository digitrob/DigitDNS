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
    val proxy: Color,
    val purple: Color,
    val discord: Color,
    /** The protocol ramp's TCP blue. [info] is the cyan accent and must not be reused. */
    val blue: Color,
    /** DoH3's category colour. */
    val orange: Color,
    /** DNSCrypt's category colour. */
    val fuchsia: Color,
    /**
     * The switch knob. Mirrors the web panel's `--knob-bg`, which is not white on the
     * two dark themes: a pure-white dot on a filled track reads as a browser default
     * rather than the panel's control. See [PanelColors.KnobDark].
     */
    val knob: Color,
)

internal fun digitDnsExtendedColors(dark: Boolean, grey: Boolean = false): ExtendedColors = when {
    grey -> ExtendedColors(
        bgMain = PanelColors.Slate900Grey,
        bgCard = PanelColors.BgCardGrey,
        bgCardHover = PanelColors.BgCardHoverGrey,
        bgTile = PanelColors.BgTileGrey,
        borderColor = PanelColors.BorderColorGrey,
        borderGlow = PanelColors.BorderGlowGrey,
        textMain = PanelColors.Slate50Grey,
        textMuted = PanelColors.Slate400Grey,
        success = PanelColors.AccentGreenGrey,
        warning = PanelColors.AccentAmberGrey,
        danger = PanelColors.AccentRedGrey,
        info = PanelColors.Cyan400Grey,
        proxy = PanelColors.AccentProxyGrey,
        purple = PanelColors.AccentPurpleGrey,
        discord = PanelColors.AccentDiscord,
        // Graphite is monochrome for category colour; green/amber/red keep their
        // meaning below, but the three decorative protocol hues go neutral.
        blue = PanelColors.Slate300Grey,
        orange = PanelColors.Slate300Grey,
        fuchsia = PanelColors.Slate300Grey,
        knob = PanelColors.KnobGrey,
    )
    dark -> ExtendedColors(
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
        proxy = PanelColors.Cyan300Dark,
        purple = PanelColors.AccentPurpleDark,
        discord = PanelColors.AccentDiscord,
        blue = PanelColors.Blue400Dark,
        orange = PanelColors.Orange400Dark,
        fuchsia = PanelColors.Fuchsia400Dark,
        knob = PanelColors.KnobDark,
    )
    else -> ExtendedColors(
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
        proxy = PanelColors.Cyan300Light,
        purple = PanelColors.AccentPurpleLight,
        discord = PanelColors.AccentDiscord,
        blue = PanelColors.Blue400Light,
        orange = PanelColors.Orange400Light,
        fuchsia = PanelColors.Fuchsia400Light,
        knob = PanelColors.KnobLight,
    )
}

/** The in-force [ExtendedColors]. Read it as `DigitDnsTheme.colors`. */
val LocalExtendedColors = staticCompositionLocalOf { digitDnsExtendedColors(dark = true) }
