package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.HistoryApi
import com.digitrob.digitdns.core.network.StatsSeries
import com.digitrob.digitdns.core.network.StatsSettings
import com.digitrob.digitdns.core.network.StatsSettingsSave
import com.digitrob.digitdns.core.network.StatsSummary
import com.digitrob.digitdns.core.network.StatsTop
import com.digitrob.digitdns.domain.model.HistoryRange
import com.digitrob.digitdns.domain.model.TopDimension
import javax.inject.Inject
import javax.inject.Singleton

/** One range's cards, fetched together so the dashboard commits a coherent frame. */
data class HistorySnapshot(
    val summary: StatsSummary,
    val series: StatsSeries,
    val tops: Map<TopDimension, StatsTop>,
)

/**
 * Historical aggregates and their server-wide policy.
 *
 * One request returns the summary, the series and every ranked table. On a phone
 * only five rows per table fit; requesting the same five the UI renders keeps the
 * response small without changing the server's bounded top-N contract.
 */
@Singleton
class HistoryRepository @Inject constructor(
    private val api: HistoryApi,
) {
    suspend fun load(range: HistoryRange): HistorySnapshot {
        val data = api.history(range.wire, null, TOP_LIMIT)
        val tops = TopDimension.entries.associateWith { dimension ->
            data.tops[dimension.wire] ?: StatsTop(range = data.range, dimension = dimension.wire)
        }
        return HistorySnapshot(data.summary, data.series, tops)
    }

    suspend fun settings(): StatsSettings = api.settings()

    suspend fun saveSettings(settings: StatsSettings): StatsSettingsResponseView {
        val response = api.saveSettings(
            StatsSettingsSave(
                enabled = settings.enabled,
                retentionDays = settings.retentionDays,
                topDomains = settings.topDomains,
                topClients = settings.topClients,
                anonymizeClients = settings.anonymizeClients,
                excludeClients = settings.excludeClients.orEmpty(),
            ),
        )
        return StatsSettingsResponseView(
            success = response.success,
            restartRequired = response.restartRequired,
            settings = response.settings ?: settings,
        )
    }

    private companion object {
        const val TOP_LIMIT = 5
    }
}

/** The save response, normalised so a server that omits the echo still returns a value. */
data class StatsSettingsResponseView(
    val success: Boolean,
    val restartRequired: Boolean,
    val settings: StatsSettings,
)
