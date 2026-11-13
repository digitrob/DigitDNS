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

/**
 * Home-screen status widget: shows total queries and the live query rate,
 * and opens the dashboard on tap. The app's polling loop pushes fresh numbers
 * into the shared preferences below and calls [updateAll]; the system also
 * re-asks every 30 minutes via [android.appwidget.AppWidgetProviderInfo.updatePeriodMillis].
 *
 * Only the status widget ships in this first pass; the toggle and query-log
 * widgets are tracked as follow-ups.
 */
class DigitDnsStatusWidgetProvider : AppWidgetProvider() {

    override fun onUpdate(context: Context, appWidgetManager: AppWidgetManager, appWidgetIds: IntArray) {
        val prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
        val totalQueries = prefs.getLong(KEY_TOTAL_QUERIES, 0L)
        val qps = prefs.getFloat(KEY_QPS, 0f)
        for (id in appWidgetIds) {
            updateOne(context, appWidgetManager, id, totalQueries, qps)
        }
    }

    companion object {
        const val PREFS = "digitdns_widget"
        const val KEY_TOTAL_QUERIES = "total_queries"
        const val KEY_QPS = "qps"

        fun updateAll(context: Context, totalQueries: Long, qps: Float) {
            val prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
            prefs.edit().putLong(KEY_TOTAL_QUERIES, totalQueries).putFloat(KEY_QPS, qps).apply()
            val manager = AppWidgetManager.getInstance(context)
            val component = ComponentName(context, DigitDnsStatusWidgetProvider::class.java)
            val ids = manager.getAppWidgetIds(component)
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
            views.setTextViewText(R.id.widget_qps_value, String.format(java.util.Locale.US, "%.1f", qps))
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
            manager.updateAppWidget(appWidgetId, views)
        }
    }
}
