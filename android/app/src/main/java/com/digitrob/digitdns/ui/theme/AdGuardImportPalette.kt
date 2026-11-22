package com.digitrob.digitdns.ui.theme

import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

/**
 * The importer's review states resolve to the panel's own palette tokens.
 *
 * This used to hold a private copy of the web card's pill colours, which made the
 * review screen the only list in the app whose badges did not come from
 * [StatusBadge]'s theme colours. The route colours now go through
 * [actionBadgeColor], the same mapping Live Log and Domains use, so a PROXY row
 * reads identically wherever it appears. The review states reuse existing tokens
 * (success/purple/warning/muted) rather than inventing a fourth palette.
 */
internal object AdGuardImportPalette {

    /** A parsed row's review state, using the panel's existing semantic tokens. */
    fun bucketColor(bucket: String, colors: ExtendedColors): Color = when (bucket) {
        "covered" -> colors.success
        "adguardOnly" -> colors.purple
        "conflict" -> colors.warning
        else -> colors.textMuted
    }

    @Composable
    fun bucketColor(bucket: String): Color = bucketColor(bucket, DigitDnsTheme.colors)

    /** The card status, mapped with the same vocabulary as the review buckets. */
    fun statusColor(status: String, colors: ExtendedColors): Color = when {
        status.contains("fail", ignoreCase = true) ||
            status.contains("unavailable", ignoreCase = true) -> colors.danger

        // A loaded preview is a completed, usable state: it reads green like Saved
        // and Applied. It used to borrow the purple "to DigitDNS" chip colour, which
        // made a finished preview look like a pending action.
        status.contains("ready", ignoreCase = true) ||
            status.contains("saved", ignoreCase = true) ||
            status.contains("applied", ignoreCase = true) ||
            status.contains("deleted", ignoreCase = true) -> colors.success

        status.contains("previewing", ignoreCase = true) ||
            status.contains("applying", ignoreCase = true) ||
            status.contains("saving", ignoreCase = true) -> colors.info

        else -> colors.textMuted
    }

    @Composable
    fun statusColor(status: String): Color = statusColor(status, DigitDnsTheme.colors)

    /** Route chips deliberately borrow the shared action badge colour mapping. */
    fun routeColor(action: String, colors: ExtendedColors): Color = actionBadgeColor(action, colors)

    @Composable
    fun routeColor(action: String): Color = actionBadgeColor(action)
}
