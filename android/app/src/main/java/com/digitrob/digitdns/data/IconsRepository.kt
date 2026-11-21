package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.IconsApi
import timber.log.Timber
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Icon-library ids for the service editor. The search itself is best-effort: an
 * offline install or a blocked Iconify endpoint simply leaves the suggestions
 * empty, and the operator can still type a favicon URL by hand.
 */
@Singleton
class IconsRepository @Inject constructor(private val api: IconsApi) {
    suspend fun search(query: String): List<String> = runCatching { api.search(query).icons }
        .getOrElse { error ->
            Timber.d(error, "icon search unavailable")
            emptyList()
        }
}
