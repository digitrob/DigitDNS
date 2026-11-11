package com.digitrob.digitdns.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import com.digitrob.digitdns.ui.theme.PanelColors as P

/**
 * The panel's theme, expressed as a Material 3 [androidx.compose.material3.ColorScheme].
 *
 * The mapping mirrors what the panel means by each token rather than what M3 usually
 * calls it:
 *  - `primary` is the brand cyan fill (`--c-cyan-500`), and `onPrimary` is the near-black
 *    the seven primary buttons carry (`text-slate-950`), not white — white on cyan-500
 *    measured 1.86:1 in the panel and is the exact failure its comments call out. In the
 *    light theme the same token is *near-white* instead: the light `--c-cyan-500` is a
 *    deep teal fill, so its foreground must be light, not dark. The two themes invert
 *    for the same reason their ramps do (see [PanelColors]).
 *  - `surface`/`surfaceVariant` carry the card colours; the glass translucency of
 *    `--bg-card` is preserved in [ExtendedColors], since M3 tints surfaces by elevation
 *    on its own and would flatten the alpha.
 *  - `error` is the panel's `--accent-red`.
 *
 * The dark scheme is the panel's default (its no-JavaScript fallback is the dark
 * panel), so the app does too: the scheme follows the OS setting but the app opens
 * dark unless light was explicitly chosen.
 */
private val DarkScheme = darkColorScheme(
    primary = P.Cyan500Dark,
    onPrimary = P.Slate950Dark,
    primaryContainer = P.Cyan950Dark,
    onPrimaryContainer = P.Cyan200Dark,
    secondary = P.Purple500Dark,
    onSecondary = P.Slate950Dark,
    secondaryContainer = P.Purple700Dark,
    onSecondaryContainer = P.Purple300Dark,
    tertiary = P.Emerald500Dark,
    onTertiary = P.Slate950Dark,
    error = P.Red500Dark,
    onError = P.Slate950Dark,
    errorContainer = P.Red900Dark,
    onErrorContainer = P.Red300Dark,
    background = P.Slate900Dark,
    onBackground = P.Slate50Dark,
    surface = P.Slate800Dark,
    onSurface = P.Slate50Dark,
    surfaceVariant = P.Slate700Dark,
    onSurfaceVariant = P.Slate400Dark,
    outline = P.Slate600Dark,
    outlineVariant = P.Slate700Dark,
    inverseSurface = P.Slate100Dark,
    inverseOnSurface = P.Slate900Dark,
)

private val LightScheme = lightColorScheme(
    primary = P.Cyan500Light,
    onPrimary = P.Slate950Light,
    primaryContainer = P.Cyan950Light,
    onPrimaryContainer = P.Cyan500Light,
    secondary = P.Purple500Light,
    onSecondary = P.Slate950Light,
    secondaryContainer = P.Purple300Light,
    onSecondaryContainer = Color.White,
    tertiary = P.Emerald500Light,
    onTertiary = P.Slate950Light,
    error = P.Red500Light,
    onError = P.Slate950Light,
    errorContainer = P.Red950Light,
    onErrorContainer = P.Red300Light,
    background = P.Slate900Light,
    onBackground = P.Slate50Light,
    surface = P.Slate950Light,
    onSurface = P.Slate50Light,
    surfaceVariant = P.Slate800Light,
    onSurfaceVariant = P.Slate400Light,
    outline = P.Slate600Light,
    outlineVariant = P.Slate700Light,
    inverseSurface = P.Slate100Light,
    inverseOnSurface = P.Slate900Light,
)

/** Graphite: neutral surfaces, light-grey primary actions, semantic green/amber/red. */
private val GreyScheme = darkColorScheme(
    primary = P.Slate200Grey,
    onPrimary = P.Slate950Grey,
    primaryContainer = P.Slate800Grey,
    onPrimaryContainer = P.Slate100Grey,
    secondary = P.Slate400Grey,
    onSecondary = P.Slate950Grey,
    secondaryContainer = P.Slate700Grey,
    onSecondaryContainer = P.Slate100Grey,
    tertiary = P.AccentGreenGrey,
    onTertiary = P.Slate950Grey,
    error = P.Red500Dark,
    onError = P.Slate950Grey,
    errorContainer = P.Red900Dark,
    onErrorContainer = P.Red300Dark,
    background = P.Slate900Grey,
    onBackground = P.Slate50Grey,
    surface = P.Slate800Grey,
    onSurface = P.Slate50Grey,
    surfaceVariant = P.Slate700Grey,
    onSurfaceVariant = P.Slate400Grey,
    outline = P.Slate500Grey,
    outlineVariant = P.Slate700Grey,
    inverseSurface = P.Slate100Grey,
    inverseOnSurface = P.Slate900Grey,
)

/** Whether the app is currently rendering the light scheme. */
val LocalIsLightTheme = staticCompositionLocalOf { false }

/** Whether the app is currently rendering the Graphite/grey scheme. */
val LocalIsGreyTheme = staticCompositionLocalOf { false }

@Composable
fun DigitDnsTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    greyTheme: Boolean = false,
    content: @Composable () -> Unit,
) {
    val scheme = when {
        greyTheme -> GreyScheme
        darkTheme -> DarkScheme
        else -> LightScheme
    }
    val extended = digitDnsExtendedColors(darkTheme, greyTheme)

    CompositionLocalProvider(
        LocalExtendedColors provides extended,
        LocalIsLightTheme provides !darkTheme,
        LocalIsGreyTheme provides greyTheme,
    ) {
        MaterialTheme(
            colorScheme = scheme,
            typography = DigitDnsTypography,
            content = content,
        )
    }
}

/** The non-M3 panel tokens, resolved for the theme in force: `DigitDnsTheme.colors`. */
object DigitDnsTheme {
    val colors: ExtendedColors
        @Composable
        @ReadOnlyComposable
        get() = LocalExtendedColors.current
}
