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
    // Category colours for the protocol ramp. They exist as their own tokens because
    // the live log and the Requests/Browsers card read them by role: DoH3 is orange
    // and DNSCrypt is fuchsia in the panel's own protocol table, and both need a
    // light-mode step that is not the dark one (230 121 249 on white is 1.9:1).
    val Orange400Dark = Color(0xFFFB923C) // 251 146 60
    val Fuchsia400Dark = Color(0xFFE879F9) // 232 121 249

    // Page chrome.
    val BgCardDark = Color(0xB8121C38) // rgba(13, 20, 38, 0.72)
    val BgCardHoverDark = Color(0xE6141E3A) // rgba(20, 30, 58, 0.9)
    val BgTileDark = Color(0xB3030712) // slate-950/70 — the inset inner tile
    // The switch knob. The web panel does not paint it pure white: --knob-bg is
    // slate-300 in dark, zinc-300 in graphite and only white on the light page, so a
    // filled green/red track keeps a quiet knob instead of a paper-white dot.
    val KnobDark = Color(0xFFCBD5E1)
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
    // The light-mode step of the two protocol category colours, matching the
    // [data-theme="light"] .proto-* block in web/css/style.css: orange-700 and
    // fuchsia-800, both of which clear 4.5:1 on the light card.
    val Orange400Light = Color(0xFFC2410C) // 194 65 12
    val Fuchsia400Light = Color(0xFF86198F) // 134 25 143

    val BgCardLight = Color(0xEBFFFFFF) // rgba(255, 255, 255, 0.92)
    val BgCardHoverLight = Color(0xFFFFFFFF)
    val BgTileLight = Color(0x0F0F172A) // slate-900/6 — the inset inner tile
    val KnobLight = Color(0xFFFFFFFF) // --knob-bg on the light page
    val BorderColorLight = Color(0x1A0F172A) // rgba(15, 23, 42, 0.1)
    val BorderGlowLight = Color(0x730891B6) // rgba(8, 145, 178, 0.45)
    val AccentGreenLight = Color(0xFF065840)
    val AccentPurpleLight = Color(0xFF621F98)
    val AccentRedLight = Color(0xFF8C1C1C)
    val AccentAmberLight = Color(0xFF92400E)

    // --- Grey theme (web/css/style.css [data-theme="grey"]) ---

    val Slate950Grey = Color(0xFF09090B)
    val Slate900Grey = Color(0xFF18181B)
    val Slate800Grey = Color(0xFF27272A)
    val Slate700Grey = Color(0xFF3F3F46)
    val Slate600Grey = Color(0xFF52525B)
    val Slate500Grey = Color(0xFF91919A)
    val Slate400Grey = Color(0xFFA1A1AA)
    val Slate300Grey = Color(0xFFD4D4D8)
    val Slate200Grey = Color(0xFFE4E4E7)
    val Slate100Grey = Color(0xFFF4F4F5)
    val Slate50Grey = Color(0xFFFAFAFA)

    // The cyan ramp becomes neutral, matching the web theme. 500 is the control
    // fill (switches/checkboxes); 400 is the former info accent.
    val Cyan500Grey = Color(0xFF71717A)
    val Cyan400Grey = Color(0xFFA1A1AA)
    val Cyan300Grey = Color(0xFFD4D4D8)
    val Cyan200Grey = Color(0xFFE4E4E7)
    val Cyan950Grey = Color(0xFF27272A)

    val BgCardGrey = Color(0xC727272A) // rgba(39, 39, 42, 0.78)
    val BgCardHoverGrey = Color(0xEB2D2D30) // rgba(45, 45, 48, 0.92)
    val BgTileGrey = Color(0xB309090B) // slate-950/70
    val KnobGrey = Color(0xFFD4D4D8) // --knob-bg in the graphite theme
    val BorderColorGrey = Color(0x2EA1A1AA) // rgba(161, 161, 170, 0.18)
    val BorderGlowGrey = Color(0x38FFFFFF) // rgba(255, 255, 255, 0.22)
    val AccentGreenGrey = Color(0xFF10B981)
    // DoH still needs a distinct protocol colour. Lavender reads well on graphite
    // without bringing cyan back.
    val AccentPurpleGrey = Color(0xFFC4B5FD)
    // Proxy is the one route that keeps a colour in Graphite. Sky blue separates
    // it from green Direct, red Blocked and grey Paused without neon.
    val AccentProxyGrey = Color(0xFF7DD3FC)
    val AccentRedGrey = Color(0xFFFF4655)
    val AccentAmberGrey = Color(0xFFF59E0B)
}
