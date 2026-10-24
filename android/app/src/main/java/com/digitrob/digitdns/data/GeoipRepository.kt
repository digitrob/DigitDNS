package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.GeoipApi
import com.digitrob.digitdns.core.network.GeoipStatusDto
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The server-location database. Both calls answer the same shape; the only
 * difference is that [update] first downloads a fresh file, which can take a few
 * seconds and is why the UI disables the button while it runs.
 */
@Singleton
class GeoipRepository @Inject constructor(
    private val api: GeoipApi,
) {
    suspend fun status(): GeoipStatusDto = api.status()

    suspend fun update(): GeoipStatusDto = api.update()
}
