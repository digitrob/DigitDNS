package com.digitrob.digitdns.ui.theme

import androidx.compose.ui.graphics.Color

/**
 * The six pills used only by the AdGuard upstream-list import review screen.
 *
 * These are deliberately **not** the panel's action/status tokens. A green pill
 * here means "already covered by DigitDNS", not "a cached DNS answer"; a blue one
 * means "add to DigitDNS", not "proxied". Reusing [ExtendedColors] (and therefore
 * [actionToneColor]) made the review list read as if it were describing query
 * outcomes, so the importer keeps its own small ramp. The values are also distinct
 * from every action tone in every theme, which [AdGuardImportPaletteTest] pins.
 */
internal object AdGuardImportPalette {
    /** Covered: the domain is already in the DigitDNS rule store. */
    val already: Color = Color(0xFF2A8C77)

    /** Add: an AdGuard-only domain proposed for custom_proxied. */
    val add: Color = Color(0xFF3D6FC4)

    /** Conflict: another DigitDNS rule already claims the domain. */
    val conflict: Color = Color(0xFFC23D5B)

    /** Send back: a DigitDNS-only domain proposed for the AdGuard block. */
    val sendBack: Color = Color(0xFF9257B8)

    /** Broad: one of the high-traffic domains that needs a second look. */
    val broad: Color = Color(0xFFB77A18)

    /** External upstream: AdGuard routes to an address other than this server. */
    val externalUpstream: Color = Color(0xFF5E7285)

    /** All six roles, for distinctness checks and the summary legend. */
    fun all(): List<Color> = listOf(already, add, conflict, sendBack, broad, externalUpstream)

    /** The bucket pill colour. Extra flags (broad, external) get their own pills. */
    fun forBucket(bucket: String): Color = when (bucket) {
        "covered" -> already
        "adguardOnly" -> add
        "digitdnsOnly" -> sendBack
        "conflict" -> conflict
        else -> externalUpstream
    }
}
