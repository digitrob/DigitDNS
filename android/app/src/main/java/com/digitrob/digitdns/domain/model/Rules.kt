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
) {
    companion object {
        fun from(dto: RulesResponse): RuleLists = RuleLists(
            proxied = dto.customProxied,
            proxiedDisabled = dto.customProxiedDisabled,
            blocked = dto.customBlocked,
            direct = dto.customDirect,
            records = dto.customRecords,
        )
    }
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
