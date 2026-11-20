package com.digitrob.digitdns.core.widget

import android.content.Context
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Mirrors values the app already has into the widget's shared preferences.
 *
 * The widgets use the preferences for their instant/system refresh paint, so an
 * open dashboard or Ad Blocking screen can keep them current without a second
 * network request. When no widget is installed this is a deliberate no-op — the
 * 3 s dashboard poll must not pay for preferences or an AppWidgetManager call.
 */
@Singleton
class WidgetPublisher @Inject constructor(
    @ApplicationContext private val context: Context,
) {

    fun publishStats(totalQueries: Long, qps: Double) {
        DigitDnsStatusWidgetProvider.updateAll(context, totalQueries, qps.toFloat())
    }

    fun publishFiltering(enabled: Boolean?) {
        DigitDnsToggleWidgetProvider.updateAll(context, enabled)
    }
}
