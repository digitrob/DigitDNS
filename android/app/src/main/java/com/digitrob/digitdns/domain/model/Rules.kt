package com.digitrob.digitdns.domain.model

import com.digitrob.digitdns.core.network.CustomGroupDto
import com.digitrob.digitdns.core.network.PolicyDto
import com.digitrob.digitdns.core.network.RulesResponse

/**
 * The four custom domain lists and the pinned records, as the rules screen edits
 * them. This is a direct mirror of the wire shape: unlike [Client] there is no
 * derived state worth computing once, so it is a data holder with the one piece of
 * meaning the UI needs — which list a domain belongs to.
 */
data class RuleLists(
    val proxied: List<String> = emptyList(),
    val proxiedDisabled: List<String> = emptyList(),
    val blocked: List<String> = emptyList(),
    val direct: List<String> = emptyList(),
    val records: Map<String, String> = emptyMap(),
    /** Service id -> display name. A label, never a routing decision. */
    val services: Map<String, String> = emptyMap(),
    /** Domain -> service id. A domain without an entry has no service. */
    val domainServices: Map<String, String> = emptyMap(),
    /** Service id -> icon URL. Optional and separate from the name. */
    val serviceIcons: Map<String, String> = emptyMap(),
) {
    /** Every pattern any of the four lists holds; the quick-action menu preselects from this. */
    val managedPatterns: Set<String>
        get() = (proxied + proxiedDisabled + blocked + direct).toSet()

    /** The service a domain is labelled with, or empty when it has none. */
    fun serviceOf(domain: String): String = domainServices[domain.normalizedDomain()].orEmpty()

    companion object {
        fun from(dto: RulesResponse): RuleLists = RuleLists(
            proxied = dto.customProxied,
            proxiedDisabled = dto.customProxiedDisabled,
            blocked = dto.customBlocked,
            direct = dto.customDirect,
            records = dto.customRecords,
            services = dto.services,
            domainServices = dto.domainServices,
            serviceIcons = dto.serviceIcons,
        )
    }
}

/** The key a domain is stored under: trimmed, no trailing dot, lowercase. */
fun String.normalizedDomain(): String = trim().trimEnd('.').lowercase(java.util.Locale.US)

/**
 * Turn a service name into its stable id. Renaming keeps the id, so every domain
 * already carrying the label survives it; only a new service derives one.
 */
fun serviceIdFromName(name: String): String =
    name.trim().lowercase(java.util.Locale.US)
        .replace(Regex("[^a-z0-9]+"), "-")
        .trim('-')
        .ifEmpty { "service" }

/**
 * Normalise the icon field. A bare domain becomes that site's favicon, an http(s)
 * URL is kept as given, and anything else is ignored — the same rule the panel
 * applies, so a service created on one client renders on the other.
 */
/**
 * The URL Android actually fetches for an icon. The panel stores DuckDuckGo's
 * `.ico` favicon, which Android's BitmapFactory/Coil cannot decode; its Google
 * S2 PNG equivalent renders on both. Any other URL is left exactly as given.
 */
fun serviceDisplayIcon(url: String): String {
    val match = DUCK_ICON.matchEntire(url.trim()) ?: return url
    return "https://www.google.com/s2/favicons?domain=${match.groupValues[1]}&sz=64"
}

private val DUCK_ICON =
    Regex("^https://icons\\.duckduckgo\\.com/ip3/(.+)\\.ico$", RegexOption.IGNORE_CASE)

fun serviceIconUrl(raw: String): String {
    val value = raw.trim()
    return when {
        value.isEmpty() -> ""
        value.startsWith("http://", ignoreCase = true) ||
            value.startsWith("https://", ignoreCase = true) -> value
        Regex("^[a-z0-9.-]+\\.[a-z]{2,}$", RegexOption.IGNORE_CASE).matches(value) ->
            "https://icons.duckduckgo.com/ip3/$value.ico"
        else -> ""
    }
}

/**
 * Replace [remove] with [add] in one rule list, keeping the slot of the first removed
 * entry. A scope change (a.b.c -> *.a.b.c) must not move the row to the end of the
 * list, which is what "Added (newest)" shows as a jump to the top. With nothing to
 * remove the patterns are appended (the live-log case).
 */
fun replaceDomainPatterns(
    list: List<String>,
    remove: Collection<String>,
    add: List<String>,
): List<String> {
    val first = list.indexOfFirst { it in remove || it in add }
    val keep = { d: String -> d !in remove && d !in add }
    val kept = list.filter(keep)
    if (first < 0) return kept + add
    val at = list.take(first).count(keep)
    return kept.take(at) + add + kept.drop(at)
}

/** Which custom list a domain belongs to. The wire key is [wireKey]. */
enum class RuleList(val wireKey: String) {
    Proxied("custom_proxied"),
    ProxiedDisabled("custom_proxied_disabled"),
    Blocked("custom_blocked"),
    Direct("custom_direct"),
}

/** The action a custom group applies to the domains it holds. */
enum class GroupAction(val wire: String) {
    Proxy("proxy"),
    Direct("direct"),
    Block("block"),
    ;

    companion object {
        /** Unknown values fall back to [Proxy] rather than throwing on a new action. */
        fun from(wire: String): GroupAction =
            entries.firstOrNull { it.wire == wire } ?: Proxy
    }
}

/**
 * A named set of domains sharing one action. The id and creation time are
 * server-owned; the operator edits name, action, domains and enabled.
 */
data class CustomGroup(
    val id: String,
    val name: String,
    val action: GroupAction,
    val domains: List<String>,
    val enabled: Boolean,
    val createdAt: String,
) {
    val isNew: Boolean get() = id.isBlank()

    companion object {
        fun from(dto: CustomGroupDto): CustomGroup = CustomGroup(
            id = dto.id,
            name = dto.name,
            action = GroupAction.from(dto.action),
            domains = dto.domains,
            enabled = dto.enabled,
            createdAt = dto.createdAt,
        )
    }
}

/** One stored policy record. */
data class Policy(
    val key: String,
    val name: String,
    val category: String,
    val enabled: Boolean,
    val customDomains: List<String>,
) {
    companion object {
        fun from(dto: PolicyDto): Policy = Policy(
            key = dto.key,
            name = dto.name,
            category = dto.category,
            enabled = dto.enabled,
            customDomains = dto.customDomains,
        )
    }
}
