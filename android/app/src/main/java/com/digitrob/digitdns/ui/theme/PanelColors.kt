package com.digitrob.digitdns.ui.theme

import androidx.compose.ui.graphics.Color

/**
 * The panel's colour tokens, ported from web/css/style.css.
 *
 * The panel defines its palette twice — a `[data-theme="dark"]` block and a
 * `[data-theme="light"]` one — because a token means different things per theme:
 * a *fill* shade deepens as its number rises while a *text* shade lightens, and the
 * two tiers invert in opposite directions between themes. Rather than collapse that
 * into one ramp (which is exactly the mistake the panel's own comments describe,
 * where near-white text landed on a bright fill at 2.26:1), this file keeps both
 * blocks verbatim. [DigitDnsColorScheme] picks one per theme.
 *
 * The names match the CSS custom properties so a designer can diff the two files.
 */
internal object PanelColors {

    // --- Dark theme (web/css/style.css [data-theme="dark"]) ---

    // Slate ramp. 950 = deepest inset, 900 = the page, 800/700 = raised surfaces
    // and borders, 600-500 = dividers and placeholders, 400-50 = text muted→strong.
    val Slate950Dark = Color(0xFF030712)
    val Slate900Dark = Color(0xFF060913)
    val Slate800Dark = Color(0xFF0D1426)
    val Slate700Dark = Color(0xFF16213E)
    val Slate600Dark = Color(0xFF475569)
    val Slate500Dark = Color(0xFF8090A3)
    val Slate400Dark = Color(0xFF94A3B8)
    val Slate300Dark = Color(0xFFCBD5E1)
    val Slate200Dark = Color(0xFFE2E8F0)
    val Slate100Dark = Color(0xFFF1F5F9)
    val Slate50Dark = Color(0xFFF8FAFC)

    // Accent cyan. 500 is the fill, 400 the text/icon accent, 300/200 quieter text,
    // 950 a tinted background block.
    val Cyan500Dark = Color(0xFF00D2DF)
    val Cyan400Dark = Color(0xFF00F0FF)
    val Cyan300Dark = Color(0xFF67E8F9)
    val Cyan200Dark = Color(0xFFA5F3FC)
    val Cyan950Dark = Color(0xFF083344)

    // Semantic text shades (300/400) and fill shades (500/600/700).
    val Emerald500Dark = Color(0xFF10B981)
    val Emerald400Dark = Color(0xFF34D399)
    val Emerald300Dark = Color(0xFF6EE7B7)
    val Red700Dark = Color(0xFFB91C1C)
    val Red600Dark = Color(0xFFDC2626)
    val Red500Dark = Color(0xFFEF4444)
    val Red400Dark = Color(0xFFF87171)
    val Red300Dark = Color(0xFFFCA5A5)
    val Red950Dark = Color(0xFF450A0A)
    val Red900Dark = Color(0xFF7F1D1D)
    val Amber500Dark = Color(0xFFF59E0B)
    val Amber400Dark = Color(0xFFFBBF24)
    val Amber300Dark = Color(0xFFFCD34D)
    val Purple700Dark = Color(0xFF7E22CE)
    val Purple600Dark = Color(0xFF9333EA)
    val Purple500Dark = Color(0xFFA855F7)
    val Purple400Dark = Color(0xFFC084FC)
    val Purple300Dark = Color(0xFFD8B4FE)
    val Blue600Dark = Color(0xFF2563EB)
    val Blue500Dark = Color(0xFF3B82F6)
    val Blue400Dark = Color(0xFF60A5FA)

    // Page chrome.
    val BgCardDark = Color(0xB8121C38) // rgba(13, 20, 38, 0.72)
    val BgCardHoverDark = Color(0xE6141E3A) // rgba(20, 30, 58, 0.9)
    val BgTileDark = Color(0xB3030712) // slate-950/70 — the inset inner tile
    val BorderColorDark = Color(0x2938BDF8) // rgba(56, 189, 248, 0.16)
    val BorderGlowDark = Color(0x8C00F0FF) // rgba(0, 240, 255, 0.55)
    val AccentGreenDark = Color(0xFF10B981)
    val AccentPurpleDark = Color(0xFFA855F7)
    val AccentRedDark = Color(0xFFFF4655)
    val AccentAmberDark = Color(0xFFF59E0B)
    val AccentDiscord = Color(0xFF5865F2) // brand-fixed in both themes

    // --- Light theme (web/css/style.css [data-theme="light"]) ---

    val Slate950Light = Color(0xFFEEF2F7)
    val Slate900Light = Color(0xFFF7F9FC)
    val Slate800Light = Color(0xFFDDE3EC)
    val Slate700Light = Color(0xFFC3CCDA)
    val Slate600Light = Color(0xFF8D99AB)
    val Slate500Light = Color(0xFF545F70)
    val Slate400Light = Color(0xFF4A5464)
    val Slate300Light = Color(0xFF3C4654)
    val Slate200Light = Color(0xFF2A3340)
    val Slate100Light = Color(0xFF1B2432)
    val Slate50Light = Color(0xFF111823)

    val Cyan500Light = Color(0xFF155E75)
    val Cyan400Light = Color(0xFF0E7490)
    val Cyan300Light = Color(0xFF0B6178)
    val Cyan200Light = Color(0xFF08505F)
    val Cyan950Light = Color(0xFFE0F5FA)

    val Emerald500Light = Color(0xFF065F46)
    val Emerald400Light = Color(0xFF056C4E)
    val Emerald300Light = Color(0xFF065640)
    val Red600Light = Color(0xFF991B1B)
    val Red500Light = Color(0xFF991B1B)
    val Red400Light = Color(0xFFB91C1C)
    val Red300Light = Color(0xFF8C1C1C)
    val Red950Light = Color(0xFFFECACA)
    val Red900Light = Color(0xFFFCA5A5)
    val Amber500Light = Color(0xFF78350F)
    val Amber400Light = Color(0xFF92400E)
    val Amber300Light = Color(0xFF92400E)
    val Purple600Light = Color(0xFF6B21A8)
    val Purple500Light = Color(0xFF6B21A8)
    val Purple400Light = Color(0xFF7E22CE)
    val Purple300Light = Color(0xFF621F98)
    val Blue600Light = Color(0xFF1E40AF)
    val Blue500Light = Color(0xFF1D4ED8)
    val Blue400Light = Color(0xFF1D4ED8)

    val BgCardLight = Color(0xEBFFFFFF) // rgba(255, 255, 255, 0.92)
    val BgCardHoverLight = Color(0xFFFFFFFF)
    val BgTileLight = Color(0x0F0F172A) // slate-900/6 — the inset inner tile
    val BorderColorLight = Color(0x1A0F172A) // rgba(15, 23, 42, 0.1)
    val BorderGlowLight = Color(0x730891B6) // rgba(8, 145, 178, 0.45)
    val AccentGreenLight = Color(0xFF065840)
    val AccentPurpleLight = Color(0xFF621F98)
    val AccentRedLight = Color(0xFF8C1C1C)
    val AccentAmberLight = Color(0xFF92400E)
}
