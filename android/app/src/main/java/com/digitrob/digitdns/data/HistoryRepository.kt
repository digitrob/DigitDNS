package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.HistoryApi
import com.digitrob.digitdns.core.network.StatsSeries
import com.digitrob.digitdns.core.network.StatsSettings
import com.digitrob.digitdns.core.network.StatsSettingsSave
import com.digitrob.digitdns.core.network.StatsSummary
import com.digitrob.digitdns.core.network.StatsTop
import com.digitrob.digitdns.domain.model.HistoryRange
import com.digitrob.digitdns.domain.model.TopDimension
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
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
 * The six ranked tables are independent endpoints, so they load concurrently. On a
 * phone only five rows per table fit; requesting the same five the UI renders keeps
 * the response small without changing the server's bounded top-N contract.
 */
@Singleton
class HistoryRepository @Inject constructor(
    private val api: HistoryApi,
) {
    suspend fun load(range: HistoryRange): HistorySnapshot = coroutineScope {
        val summary = async { api.summary(range.wire) }
        val series = async { api.series(range.wire) }
        val tops = TopDimension.entries.associateWith { dimension ->
            async { api.top(range.wire, dimension.wire, TOP_LIMIT) }
        }
        HistorySnapshot(
            summary = summary.await(),
            series = series.await(),
            tops = tops.mapValues { (_, deferred) -> deferred.await() },
        )
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
        const val TOP_LIMIT = 10
    }
}

/** The save response, normalised so a server that omits the echo still returns a value. */
data class StatsSettingsResponseView(
    val success: Boolean,
    val restartRequired: Boolean,
    val settings: StatsSettings,
)
