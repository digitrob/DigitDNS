package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.ConfigResponse
import com.digitrob.digitdns.core.network.ConfigApi
import com.digitrob.digitdns.core.network.LiveStats
import com.digitrob.digitdns.core.network.StatsApi
import javax.inject.Inject
import javax.inject.Singleton

/** Reads the live telemetry the dashboard tiles render. */
@Singleton
class StatsRepository @Inject constructor(
    private val api: StatsApi,
) {
    suspend fun live(): LiveStats = api.live()
}

/**
 * Reads the small config slice the Connect tab needs. The full config response is
 * Phase 4's; this exposes only the pieces used here so the two phases do not fight
 * over one large DTO.
 */
@Singleton
class ConfigRepository @Inject constructor(
    private val api: ConfigApi,
) {
    suspend fun config(): ConfigResponse = api.config()
}
