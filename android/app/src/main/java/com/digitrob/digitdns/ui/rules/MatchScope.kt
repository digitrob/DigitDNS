package com.digitrob.digitdns.ui.rules

import java.util.Locale

/**
 * The match scope a routing quick action applies under, mirroring the web panel's
 * `stream-action-scope` radiogroup: the exact host, the host's own wildcard, the host
 * plus its wildcard, or the parent-domain wildcard.
 *
 * The resolved strings are what actually get written to a rule list, so the scope is
 * encoded into the domain(s) before the write rather than stored anywhere.
 */
enum class MatchScope(val label: String, val hint: String) {
    Exact("Exact", "host + subs"),
    Host("*.host", "subs only"),
    Parent("*.parent", "parent tree"),
    ;

    /**
     * Every pattern this scope writes, empty when the scope would not be specific
     * enough.
     *
     * A wildcard is only offered when it still names something more than a TLD:
     * `api.target.com` yields `*.target.com`, but `target.com` has no parent wildcard
     * to offer (its parent is `com`). A row that is already a wildcard (`*.a.b.c`)
     * rebases instead of refusing: [Exact] drops the `*.` back to `a.b.c`, [Host] is
     * the row itself, and [Parent] drops one label (`*.b.c`).
     */
    fun targets(domain: String): List<String> {
        val host = domain.trim().trimStart('.').lowercase(Locale.US)
        if (host.isEmpty()) return emptyList()
        val wildcard = host.startsWith("*.")
        val base = if (wildcard) host.removePrefix("*.") else host
        val labels = base.split('.').filter { it.isNotEmpty() }
        return when (this) {
            // Exact is the host itself; on a wildcard row that drops the "*.".
            Exact -> listOf(base)
            Host -> if (labels.size >= HOST_MIN_LABELS) listOf("*.$base") else emptyList()
            Parent -> if (labels.size >= PARENT_MIN_LABELS) {
                listOf("*." + labels.drop(1).joinToString("."))
            } else {
                emptyList()
            }
        }
    }

    /** The primary pattern, or null when the scope writes nothing for [domain]. */
    fun resolve(domain: String): String? = targets(domain).firstOrNull()

    companion object {
        /** A wildcard for the host needs at least two labels (host.tld). */
        private const val HOST_MIN_LABELS = 2

        /** A parent wildcard needs a real parent above the host.tld pair. */
        private const val PARENT_MIN_LABELS = 3

        /**
         * The scope whose patterns are already in [patterns] for [domain], so the menu
         * opens on what is configured instead of resetting to [Exact] every time.
         * The plain rule wins (it covers subdomains too), then the host wildcard, then
         * the parent wildcard.
         */
        fun initialScope(domain: String, patterns: Set<String>): MatchScope {
            if (patterns.isEmpty()) return Exact
            val preference = if (domain.trim().trimStart('.').startsWith("*.")) {
                WILDCARD_SCOPE_PREFERENCE
            } else {
                PLAIN_SCOPE_PREFERENCE
            }
            return preference.firstOrNull { scope ->
                scope.targets(domain).let { targets -> targets.isNotEmpty() && targets.all(patterns::contains) }
            } ?: Exact
        }

        /** Plain row: the plain rule already covers its subdomains, so it comes first. */
        private val PLAIN_SCOPE_PREFERENCE = listOf(Exact, Host, Parent)

        /** A wildcard row's own rule is the host wildcard, so that comes first. */
        private val WILDCARD_SCOPE_PREFERENCE = listOf(Host, Parent, Exact)
    }
}
