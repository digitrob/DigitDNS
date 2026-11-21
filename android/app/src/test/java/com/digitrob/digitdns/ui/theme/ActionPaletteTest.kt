package com.digitrob.digitdns.ui.theme

import com.digitrob.digitdns.ui.stream.queryActionStyle
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * The history screen used to carry a second action-colour table. These assertions
 * pin every action to the shared style table so the two lists cannot disagree again.
 */
class ActionPaletteTest {

    private val colors = digitDnsExtendedColors(dark = true)

    @Test
    fun everyKnownActionUsesTheSharedToneColour() {
        val cases = listOf(
            "PROXY" to colors.proxy,
            "PROXY_IPV6_SINK" to colors.proxy,
            "PROXY_HTTPS_SINK" to colors.proxy,
            "PROXY_FAILED" to colors.danger,
            "BLOCK" to colors.danger,
            "RATELIMIT" to colors.danger,
            "QUOTA" to colors.danger,
            "SERVFAIL" to colors.danger,
            "CUSTOM" to colors.purple,
            "CACHED" to colors.success,
            "DIRECT" to colors.success,
            "STALE" to colors.warning,
        )
        cases.forEach { (action, expected) ->
            assertEquals("tone colour for $action", expected, actionBadgeColor(action, colors))
            assertEquals(
                "style colour for $action",
                expected,
                actionStyleColor(queryActionStyle(action), colors),
            )
        }
    }

    @Test
    fun unknownActionsStayNeutral() {
        assertEquals(colors.textMuted, actionBadgeColor("FUTURE_ACTION", colors))
    }

    @Test
    fun proxySinksNoLongerBorrowRewriteOrTransportHues() {
        // The old history table painted these purple and blue. Both are proxy
        // outcomes and must read as the same cyan pill the live log shows.
        assertEquals(
            actionBadgeColor("PROXY", colors),
            actionBadgeColor("PROXY_IPV6_SINK", colors),
        )
        assertEquals(
            actionBadgeColor("PROXY", colors),
            actionBadgeColor("PROXY_HTTPS_SINK", colors),
        )
    }
}
