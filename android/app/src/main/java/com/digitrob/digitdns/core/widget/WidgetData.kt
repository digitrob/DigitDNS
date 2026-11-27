package com.digitrob.digitdns.core.widget

import com.digitrob.digitdns.core.network.FilteringStatusDto
import com.digitrob.digitdns.core.network.LiveStats
import com.digitrob.digitdns.core.network.ResolutionSettings
import com.digitrob.digitdns.core.network.UpstreamStat
import com.digitrob.digitdns.data.AuthRepository
import com.digitrob.digitdns.data.FilteringRepository
import com.digitrob.digitdns.data.SettingsRepository
import com.digitrob.digitdns.data.StatsRepository
import com.digitrob.digitdns.data.UpstreamsRepository
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import timber.log.Timber
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The small, authenticated read/write surface the widgets need.
 *
 * A widget can be refreshed by the system long after the activity has been
 * killed, so it cannot assume the app's in-memory session is ready. Every call
 * restores the active install first, then fails soft: a widget that cannot read
 * the daemon shows an honest unavailable state instead of a stale number.
 */
@Singleton
class WidgetData @Inject constructor(
    private val authRepository: AuthRepository,
    private val statsRepository: StatsRepository,
    private val filteringRepository: FilteringRepository,
    private val upstreamsRepository: UpstreamsRepository,
    private val settingsRepository: SettingsRepository,
) {

    data class ResolverSnapshot(
        val stats: List<UpstreamStat>?,
        val resolution: ResolutionSettings?,
        val filtering: FilteringStatusDto?,
    )

    suspend fun liveStats(): LiveStats? = runCatching {
        if (!ready()) null else statsRepository.live()
    }.onFailure { Timber.d(it, "widget stats refresh failed") }.getOrNull()

    suspend fun filteringStatus(): FilteringStatusDto? = runCatching {
        if (!ready()) null else filteringRepository.status()
    }.onFailure { Timber.d(it, "widget filtering refresh failed") }.getOrNull()

    /**
     * Resolver list data: upstream stats + resolution + filtering. Stats are the
     * source of truth for latency/order (server-sorted). Resolution provides
     * adaptiveCurrent/strategy/pins/transports for row flags.
     */
    suspend fun resolverSnapshot(): ResolverSnapshot = runCatching {
        if (!ready()) return ResolverSnapshot(null, null, null)
        coroutineScope {
            val statsDeferred = async { runCatching { upstreamsRepository.stats() }.getOrNull() }
            val resDeferred = async { runCatching { settingsRepository.loadResolution() }.getOrNull() }
            val filterDeferred = async { runCatching { filteringRepository.status() }.getOrNull() }
            ResolverSnapshot(statsDeferred.await(), resDeferred.await(), filterDeferred.await())
        }
    }.onFailure { Timber.d(it, "widget resolver snapshot failed") }
        .getOrElse { ResolverSnapshot(null, null, null) }

    /** Read the canonical state first so a widget toggle cannot undo another client's edits. */
    suspend fun toggleFiltering(): FilteringStatusDto? {
        val current = filteringStatus() ?: return null
        return saveFiltering(current, !current.settings.enabled)
    }

    suspend fun saveFiltering(
        current: FilteringStatusDto,
        enabled: Boolean,
    ): FilteringStatusDto? = runCatching {
        filteringRepository.saveSettings(WidgetValues.filteringToggleRequest(current.settings, enabled))
    }.onFailure { Timber.d(it, "widget filtering toggle failed") }.getOrNull()

    private suspend fun ready(): Boolean {
        authRepository.migrateLegacyIfNeeded()
        authRepository.loadActiveCredential()
        return authRepository.hasCredential() && authRepository.ensureSession()
    }
}
