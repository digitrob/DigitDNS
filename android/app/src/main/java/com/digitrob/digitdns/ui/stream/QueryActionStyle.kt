package com.digitrob.digitdns.ui.stream

import java.util.Locale

/** The colour family an action badge belongs to; the composable maps it to theme colours. */
enum class QueryActionTone { Info, Proxy, Danger, Success, Warning, Purple, Muted }

/** The operator-facing label and tone for one raw server action. */
data class QueryActionStyle(val label: String, val tone: QueryActionTone)

/**
 * Mirrors the server's logQuery action strings. The raw value is not a label:
 * PROXY_IPV6_SINK used to render as "Proxy_ipv6_sink" in a neutral badge, and the
 * same missing table also hid it from the Proxy filter.
 */
fun queryActionStyle(action: String): QueryActionStyle = when (action.uppercase(Locale.ROOT)) {
    "PROXY" -> QueryActionStyle("Proxy", QueryActionTone.Proxy)
    "PROXY_IPV6_SINK" -> QueryActionStyle("Proxy IPv6 Sink", QueryActionTone.Proxy)
    "PROXY_FAILED" -> QueryActionStyle("Proxy Failed", QueryActionTone.Danger)
    "BLOCK" -> QueryActionStyle("Block", QueryActionTone.Danger)
    "RATELIMIT" -> QueryActionStyle("Rate Limited", QueryActionTone.Danger)
    "QUOTA" -> QueryActionStyle("Quota", QueryActionTone.Danger)
    "CUSTOM" -> QueryActionStyle("Rewrite", QueryActionTone.Purple)
    "CACHED" -> QueryActionStyle("Direct", QueryActionTone.Success)
    "STALE" -> QueryActionStyle("Stale", QueryActionTone.Warning)
    "DIRECT" -> QueryActionStyle("Direct", QueryActionTone.Success)
    else -> QueryActionStyle(
        // An action this build has never heard of is still readable: split the
        // wire form into words instead of showing PROXY_FUTURE_THING verbatim.
        label = action.trim()
            .split('_')
            .filter { it.isNotBlank() }
            .joinToString(" ") { word ->
                word.lowercase(Locale.ROOT).replaceFirstChar { it.uppercase(Locale.ROOT) }
            }
            .ifBlank { "—" },
        tone = QueryActionTone.Muted,
    )
}

/** Whether the log's action filter should keep this action. */
fun actionMatchesFilter(action: String, filter: ActionFilter): Boolean = when (filter) {
    ActionFilter.All -> true
    // PROXY, PROXY_IPV6_SINK and PROXY_FAILED are all proxy outcomes.
    ActionFilter.Proxy -> action.uppercase(Locale.ROOT).startsWith("PROXY")
    ActionFilter.Direct -> action.uppercase(Locale.ROOT) in setOf("DIRECT", "CACHED", "STALE", "CUSTOM")
    ActionFilter.Block -> action.uppercase(Locale.ROOT) in setOf("BLOCK", "RATELIMIT", "QUOTA")
}
