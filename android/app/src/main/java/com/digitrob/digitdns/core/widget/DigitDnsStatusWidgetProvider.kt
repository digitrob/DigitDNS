@file:Suppress("TooGenericExceptionCaught")

package com.digitrob.digitdns.core.widget

import android.app.PendingIntent
import android.appwidget.AppWidgetManager
import android.appwidget.AppWidgetProvider
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.widget.RemoteViews
import com.digitrob.digitdns.MainActivity
import com.digitrob.digitdns.R
import dagger.hilt.android.AndroidEntryPoint
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import timber.log.Timber
import javax.inject.Inject

/**
 * Home-screen status widget: shows total queries and the live query rate,
 * and opens the dashboard on tap.
 *
 * Two paths keep it current: the app's existing dashboard polls publish fresh
 * numbers through [WidgetPublisher], and the system's 30-minute refresh asks the
 * daemon itself through [WidgetData]. The cached preference paint happens first
 * in both cases, so the widget is never blank while the network call is in flight.
 */
@AndroidEntryPoint
class DigitDnsStatusWidgetProvider : AppWidgetProvider() {

    @Inject lateinit var widgetData: WidgetData

    override fun onUpdate(context: Context, appWidgetManager: AppWidgetManager, appWidgetIds: IntArray) {
        super.onUpdate(context, appWidgetManager, appWidgetIds)
        if (appWidgetIds.isEmpty()) return

        val pending = goAsync()
        CoroutineScope(SupervisorJob() + Dispatchers.IO).launch {
            try {
                val stats = widgetData.liveStats()
                if (stats != null) {
                    updateAll(context, stats.totalQueries, stats.qps.toFloat(), appWidgetIds)
                }
            } catch (t: Throwable) {
                Timber.d(t, "status widget update failed")
            } finally {
                pending.finish()
            }
        }
    }

    companion object {
        const val PREFS = "digitdns_widget"
        const val KEY_TOTAL_QUERIES = "total_queries"
        const val KEY_QPS = "qps"

        fun updateAll(
            context: Context,
            totalQueries: Long,
            qps: Float,
            appWidgetIds: IntArray? = null,
        ) {
            val manager = AppWidgetManager.getInstance(context)
            val component = ComponentName(context, DigitDnsStatusWidgetProvider::class.java)
            val ids = appWidgetIds ?: manager.getAppWidgetIds(component)
            if (ids.isEmpty()) return

            val prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
            prefs.edit().putLong(KEY_TOTAL_QUERIES, totalQueries).putFloat(KEY_QPS, qps).apply()
            for (id in ids) {
                updateOne(context, manager, id, totalQueries, qps)
            }
        }

        private fun updateOne(
            context: Context,
            manager: AppWidgetManager,
            appWidgetId: Int,
            totalQueries: Long,
            qps: Float,
        ) {
            val views = RemoteViews(context.packageName, R.layout.widget_status)
            views.setTextViewText(R.id.widget_queries_value, totalQueries.toString())
            views.setTextViewText(R.id.widget_qps_value, WidgetValues.formatQps(qps))
            val intent = Intent(context, MainActivity::class.java).apply {
                flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
            }
            val pending = PendingIntent.getActivity(
                context,
                appWidgetId,
                intent,
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
            )
            views.setOnClickPendingIntent(R.id.widget_open, pending)
            views.setOnClickPendingIntent(R.id.widget_title, pending)
            views.setContentDescription(R.id.widget_open, context.getString(R.string.widget_status_open))
            views.setContentDescription(R.id.widget_title, context.getString(R.string.widget_status_open))
            manager.updateAppWidget(appWidgetId, views)
        }
    }
}
