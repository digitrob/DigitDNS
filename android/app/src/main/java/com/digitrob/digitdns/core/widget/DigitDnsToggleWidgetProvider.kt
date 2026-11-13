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
 * Home-screen toggle widget: shows whether filtering is on or off and opens the
 * app on tap. The app's polling loop pushes the current state into the shared
 * preferences below and calls [updateAll]; the system also re-asks every 30
 * minutes. Toggling from the widget opens the app (a real in-widget toggle
 * would need a network call from a BroadcastReceiver — tracked as a follow-up).
 */
class DigitDnsToggleWidgetProvider : AppWidgetProvider() {

    override fun onUpdate(context: Context, appWidgetManager: AppWidgetManager, appWidgetIds: IntArray) {
        val prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
        val enabled = prefs.getBoolean(KEY_ENABLED, true)
        for (id in appWidgetIds) {
            updateOne(context, appWidgetManager, id, enabled)
        }
    }

    companion object {
        const val PREFS = "digitdns_widget"
        const val KEY_ENABLED = "filtering_enabled"

        fun updateAll(context: Context, enabled: Boolean) {
            val prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
            prefs.edit().putBoolean(KEY_ENABLED, enabled).apply()
            val manager = AppWidgetManager.getInstance(context)
            val component = ComponentName(context, DigitDnsToggleWidgetProvider::class.java)
            val ids = manager.getAppWidgetIds(component)
            for (id in ids) {
                updateOne(context, manager, id, enabled)
            }
        }

        private fun updateOne(
            context: Context,
            manager: AppWidgetManager,
            appWidgetId: Int,
            enabled: Boolean,
        ) {
            val views = RemoteViews(context.packageName, R.layout.widget_toggle)
            views.setTextViewText(
                R.id.widget_toggle_state,
                if (enabled) context.getString(R.string.widget_toggle_on) else context.getString(R.string.widget_toggle_off),
            )
            views.setTextColor(
                R.id.widget_toggle_state,
                if (enabled) 0xFF10B981.toInt() else 0xFFFF4655.toInt(),
            )
            val intent = Intent(context, MainActivity::class.java).apply {
                flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
                putExtra("navigate", "adblock")
            }
            val pending = PendingIntent.getActivity(
                context,
                appWidgetId,
                intent,
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
            )
            views.setOnClickPendingIntent(R.id.widget_toggle_open, pending)
            views.setOnClickPendingIntent(R.id.widget_toggle_state, pending)
            manager.updateAppWidget(appWidgetId, views)
        }
    }
}
