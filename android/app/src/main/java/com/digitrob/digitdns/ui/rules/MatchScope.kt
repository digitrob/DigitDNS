package com.digitrob.digitdns.ui.rules

import java.util.Locale

/**
 * The match scope a routing quick action applies under, mirroring the web panel's
 * `stream-action-scope` radiogroup: the exact host, the host's own wildcard, or the
 * parent-domain wildcard.
 *
 * The resolved string is what actually gets written to a rule list, so the scope is
 * encoded into the domain before the write rather than stored anywhere.
 */
enum class MatchScope(val label: String, val hint: String) {
    Exact("Exact", "only this host"),
    Host("*.host", "subdomains only"),
    Parent("*.parent", "subdomains of the parent"),
    ;

    /**
     * Resolve [domain] under this scope, or null when the scope would not be specific
     * enough. A wildcard is only offered when it still names something more than a TLD:
     * `api.target.com` yields `*.target.com`, but `target.com` has no parent wildcard to
     * offer (its parent is `com`), so [Parent] returns null and the caller refuses.
     */
    fun resolve(domain: String): String? {
        val host = domain.trim().trimStart('.').lowercase(Locale.US)
        if (host.isEmpty()) return null
        // A row that is already a wildcard has no meaningful *.host (*.*.base)
        // and no parent below a TLD; only Exact applies to it.
        val wildcard = host.startsWith("*.")
        val base = if (wildcard) host.removePrefix("*.") else host
        val labels = base.split('.').filter { it.isNotEmpty() }
        return when (this) {
            Exact -> host
            Host -> if (!wildcard && labels.size >= HOST_MIN_LABELS) "*.$host" else null
            Parent -> if (!wildcard && labels.size >= PARENT_MIN_LABELS) {
                "*." + labels.drop(1).joinToString(".")
            } else {
                null
            }
        }
    }

    private companion object {
        /** A wildcard for the host needs at least two labels (host.tld). */
        const val HOST_MIN_LABELS = 2

        /** A parent wildcard needs a real parent above the host.tld pair. */
        const val PARENT_MIN_LABELS = 3
    }
}
