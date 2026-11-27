package com.digitrob.digitdns.core.widget

import com.digitrob.digitdns.core.network.ResolutionSettings
import com.digitrob.digitdns.core.network.UpstreamStat
import java.util.Locale

/**
 * Turns one resolver snapshot into the rows the widget list renders.
 *
 * Kept out of the provider so the RemoteViews plumbing and the data mapping can be
 * read and tested separately: stats are the source of truth for order/latency, and
 * resolution supplies the adaptive current, default and allowed transports.
 */
internal object WidgetRows {

    fun from(snap: WidgetData.ResolverSnapshot): List<WidgetResolverRow> {
        val stats = snap.stats
        if (stats != null && stats.isNotEmpty()) return fromStats(snap, stats)
        return fromUpstreams(snap)
    }

    private fun fromStats(
        snap: WidgetData.ResolverSnapshot,
        stats: List<UpstreamStat>,
    ): List<WidgetResolverRow> {
        val res = snap.resolution
        val current = res?.adaptiveCurrent
        val defaultAddr = res?.upstreams?.firstOrNull()
        val allowed = allowedTransports(res)
        return stats.mapIndexed { index, stat ->
            val measured = stat.measured == true
            val ms = if (measured) stat.displayLatencyMs.toLong() else null
            WidgetResolverRow(
                address = stat.address,
                displayName = WidgetValues.resolverDisplayName(stat.address),
                transportLabel = WidgetValues.resolverTransportLabel(stat.address),
                transportKey = WidgetValues.resolveProtocolKey(stat.address),
                providerKey = WidgetValues.providerKeyFromAddress(stat.address),
                latencyText = WidgetValues.formatLatencyLabel(ms, measured),
                latencyMs = ms,
                measured = measured,
                isCurrent = (current != null && stat.address == current) || (current == null && index == 0),
                isDefault = stat.address == defaultAddr,
                isDimmed = isDimmed(stat.address, allowed),
                statusKey = WidgetValues.statusKeyFor(measured, ms),
            )
        }
    }

    private fun fromUpstreams(snap: WidgetData.ResolverSnapshot): List<WidgetResolverRow> {
        val res = snap.resolution
        val upstreams = res?.upstreams ?: emptyList()
        val current = res?.adaptiveCurrent
        val allowed = allowedTransports(res)
        return upstreams.mapIndexed { index, address ->
            WidgetResolverRow(
                address = address,
                displayName = WidgetValues.resolverDisplayName(address),
                transportLabel = WidgetValues.resolverTransportLabel(address),
                transportKey = WidgetValues.resolveProtocolKey(address),
                providerKey = WidgetValues.providerKeyFromAddress(address),
                latencyText = "—",
                latencyMs = null,
                measured = false,
                isCurrent = (current != null && address == current) || (current == null && index == 0),
                isDefault = index == 0,
                isDimmed = isDimmed(address, allowed),
                statusKey = "unknown",
            )
        }
    }

    private fun allowedTransports(res: ResolutionSettings?): Set<String>? =
        res?.transports?.map { it.lowercase(Locale.US) }?.toSet()

    private fun isDimmed(address: String, allowed: Set<String>?): Boolean {
        val dnscryptEnabled = allowed?.contains("dnscrypt") == true
        val protocol = WidgetValues.resolveProtocolKey(address)
        return !WidgetValues.transportAllowedForProtocol(protocol, allowed, dnscryptEnabled)
    }
}
