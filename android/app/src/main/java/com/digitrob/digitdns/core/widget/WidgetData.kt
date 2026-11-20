package com.digitrob.digitdns.core.widget

import com.digitrob.digitdns.core.network.FilteringStatusDto
import com.digitrob.digitdns.core.network.LiveStats
import com.digitrob.digitdns.data.AuthRepository
import com.digitrob.digitdns.data.FilteringRepository
import com.digitrob.digitdns.data.StatsRepository
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
) {

    suspend fun liveStats(): LiveStats? = runCatching {
        if (!ready()) null else statsRepository.live()
    }.onFailure { Timber.d(it, "widget stats refresh failed") }.getOrNull()

    suspend fun filteringStatus(): FilteringStatusDto? = runCatching {
        if (!ready()) null else filteringRepository.status()
    }.onFailure { Timber.d(it, "widget filtering refresh failed") }.getOrNull()

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
