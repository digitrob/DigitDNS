package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.SubscriptionApi
import com.digitrob.digitdns.core.network.SubscriptionSaveRequest
import com.digitrob.digitdns.domain.model.SubscriptionSettings
import javax.inject.Inject
import javax.inject.Singleton

/** The save result: the refreshed settings, the computed origin, and any listener error. */
data class SubscriptionSaveResult(
    val settings: SubscriptionSettings?,
    val origin: String,
    val listenerError: String,
)

/**
 * The subscription portal. The save returns the recomputed `subscription_origin` so a
 * screen can show the link the portal will actually serve, and a `listener_error`
 * when the portal could not be rebound (the settings still saved).
 */
@Singleton
class SubscriptionRepository @Inject constructor(
    private val api: SubscriptionApi,
) {
    suspend fun load(): SubscriptionSettings = SubscriptionSettings.from(api.get())

    suspend fun save(request: SubscriptionSaveRequest): SubscriptionSaveResult {
        val response = api.save(request)
        return SubscriptionSaveResult(
            settings = response.subscription?.let(SubscriptionSettings::from),
            origin = response.subscriptionOrigin,
            listenerError = response.listenerError,
        )
    }
}
