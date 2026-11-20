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

private const val ACTION_TOGGLE = "com.digitrob.digitdns.widget.TOGGLE_FILTERING"
private const val TOGGLE_REQUEST_OFFSET = 1_000_000

/**
 * Home-screen toggle widget for the resolver's ad blocking.
 *
 * Tapping the state label reads the canonical filtering status, flips `enabled`,
 * and saves the whole settings object so block mode, TTL and rules are preserved.
 * The app's Ad Blocking screen mirrors its state here through [WidgetPublisher];
 * the system's 30-minute refresh reads it from the daemon through [WidgetData].
 * `null` means "not read yet / signed out", and is rendered as unavailable rather
 * than as a misleading on/off claim.
 */
@AndroidEntryPoint
class DigitDnsToggleWidgetProvider : AppWidgetProvider() {

    @Inject lateinit var widgetData: WidgetData

    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action == ACTION_TOGGLE) {
            val pending = goAsync()
            CoroutineScope(SupervisorJob() + Dispatchers.IO).launch {
                try {
                    val saved = widgetData.toggleFiltering()
                    updateAll(context, saved?.settings?.enabled)
                } catch (t: Throwable) {
                    Timber.d(t, "filtering widget toggle failed")
                    updateAll(context, null)
                } finally {
                    pending.finish()
                }
            }
            return
        }
        super.onReceive(context, intent)
    }

    override fun onUpdate(context: Context, appWidgetManager: AppWidgetManager, appWidgetIds: IntArray) {
        super.onUpdate(context, appWidgetManager, appWidgetIds)
        if (appWidgetIds.isEmpty()) return

        val pending = goAsync()
        CoroutineScope(SupervisorJob() + Dispatchers.IO).launch {
            try {
                val status = widgetData.filteringStatus()
                updateAll(context, status?.settings?.enabled, appWidgetIds)
            } catch (t: Throwable) {
                Timber.d(t, "filtering widget update failed")
            } finally {
                pending.finish()
            }
        }
    }

    companion object {
        const val PREFS = "digitdns_widget"
        const val KEY_ENABLED = "filtering_enabled"
        const val KEY_AVAILABLE = "filtering_available"

        fun updateAll(
            context: Context,
            enabled: Boolean?,
            appWidgetIds: IntArray? = null,
        ) {
            val manager = AppWidgetManager.getInstance(context)
            val component = ComponentName(context, DigitDnsToggleWidgetProvider::class.java)
            val ids = appWidgetIds ?: manager.getAppWidgetIds(component)
            if (ids.isEmpty()) return

            val prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
            val edit = prefs.edit().putBoolean(KEY_AVAILABLE, enabled != null)
            if (enabled != null) edit.putBoolean(KEY_ENABLED, enabled)
            edit.apply()
            for (id in ids) {
                updateOne(context, manager, id, enabled)
            }
        }

        private fun updateOne(
            context: Context,
            manager: AppWidgetManager,
            appWidgetId: Int,
            enabled: Boolean?,
        ) {
            val views = RemoteViews(context.packageName, R.layout.widget_toggle)
            val stateText = when (enabled) {
                true -> R.string.widget_toggle_on
                false -> R.string.widget_toggle_off
                null -> R.string.widget_toggle_unavailable
            }
            views.setTextViewText(R.id.widget_toggle_state, context.getString(stateText))
            views.setTextColor(
                R.id.widget_toggle_state,
                when (enabled) {
                    true -> 0xFF10B981.toInt()
                    false -> 0xFFFF4655.toInt()
                    null -> 0xFFFFB020.toInt()
                },
            )

            val intent = Intent(context, MainActivity::class.java).apply {
                flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
                putExtra("navigate", "adblock")
            }
            val openPending = PendingIntent.getActivity(
                context,
                appWidgetId,
                intent,
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
            )
            val toggleIntent = Intent(context, DigitDnsToggleWidgetProvider::class.java).apply {
                action = ACTION_TOGGLE
            }
            val togglePending = PendingIntent.getBroadcast(
                context,
                appWidgetId + TOGGLE_REQUEST_OFFSET,
                toggleIntent,
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
            )
            views.setOnClickPendingIntent(R.id.widget_toggle_state, if (enabled == null) openPending else togglePending)
            views.setOnClickPendingIntent(R.id.widget_toggle_open, openPending)
            views.setContentDescription(
                R.id.widget_toggle_state,
                context.getString(if (enabled == null) R.string.widget_toggle_open else R.string.widget_toggle_tap),
            )
            views.setContentDescription(R.id.widget_toggle_open, context.getString(R.string.widget_toggle_open))
            manager.updateAppWidget(appWidgetId, views)
        }
    }
}
