package com.digitrob.digitdns.domain.model

import com.digitrob.digitdns.core.network.ExplainDto
import com.digitrob.digitdns.core.network.ExplainTargetDto

/** The support-facing answer to "why did this name do that?". */
data class ExplainResult(
    val domain: String,
    val clientIp: String,
    val account: String,
    val engine: Boolean,
    val action: String,
    val rule: String,
    val pattern: String,
    val blocklistSource: String,
    val blocklistPattern: String,
    val allowSource: String,
    val allowPattern: String,
    val suppressedSource: String,
    val suppressedPattern: String,
    val customRecord: String,
    val target: ExplainTarget?,
) {
    val targetLabel: String
        get() = when (target?.kind) {
            "agent" -> "Relay agent ${target.name.ifBlank { target.agentId }}"
            "egress" -> "${target.egressKind.ifBlank { "Egress" }} ${target.name.ifBlank { target.egressId }}"
            else -> "This Server"
        }
}

data class ExplainTarget(
    val kind: String,
    val agentId: String,
    val name: String,
    val country: String,
    val egressId: String,
    val egressKind: String,
    val healthy: Boolean,
    val failClosed: Boolean,
    val fallbackReason: String,
) {
    companion object {
        fun from(dto: ExplainTargetDto?) = dto?.let {
            ExplainTarget(
                kind = it.kind,
                agentId = it.agentId,
                name = it.name,
                country = it.country,
                egressId = it.egressId,
                egressKind = it.egressKind,
                healthy = it.healthy,
                failClosed = it.failClosed,
                fallbackReason = it.fallbackReason,
            )
        }
    }
}

fun ExplainDto.toDomain() = ExplainResult(
    domain = domain,
    clientIp = clientIp,
    account = account,
    engine = engine,
    action = action,
    rule = rule,
    pattern = pattern,
    blocklistSource = blocklistSource,
    blocklistPattern = blocklistPattern,
    allowSource = allowSource,
    allowPattern = allowPattern,
    suppressedSource = suppressedSource,
    suppressedPattern = suppressedPattern,
    customRecord = customRecord,
    target = ExplainTarget.from(target),
)
