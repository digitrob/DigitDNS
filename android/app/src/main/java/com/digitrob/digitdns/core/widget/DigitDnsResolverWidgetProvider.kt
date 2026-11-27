@file:Suppress("TooGenericExceptionCaught")

package com.digitrob.digitdns.core.widget

import android.app.PendingIntent
import android.appwidget.AppWidgetManager
import android.appwidget.AppWidgetProvider
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.util.TypedValue
import android.widget.RemoteViews
import com.digitrob.digitdns.MainActivity
import com.digitrob.digitdns.R
import com.digitrob.digitdns.data.AppPreferences
import com.digitrob.digitdns.data.SettingsRepository
import dagger.hilt.android.AndroidEntryPoint
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import timber.log.Timber
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import javax.inject.Inject

private const val TOGGLE_REQUEST_OFFSET = 1_000_000
private const val REFRESH_REQUEST_OFFSET = 2_000_000
private const val SETTINGS_REQUEST_OFFSET = 3_000_000

private const val HEADER_TEXT_SP = 9f
private const val FOOTER_TEXT_SP = 11f
private const val EMPTY_TEXT_SP = 11f

@AndroidEntryPoint
class DigitDnsResolverWidgetProvider : AppWidgetProvider() {

    @Inject lateinit var widgetData: WidgetData
    @Inject lateinit var settingsRepository: SettingsRepository
    @Inject lateinit var appPreferences: AppPreferences

    override fun onEnabled(context: Context) {
        super.onEnabled(context)
        val manager = AppWidgetManager.getInstance(context)
        val ids = manager.getAppWidgetIds(ComponentName(context, DigitDnsResolverWidgetProvider::class.java))
        if (ids.isEmpty()) return
        renderFromCache(context, ids)
        val pending = goAsync()
        CoroutineScope(SupervisorJob() + Dispatchers.IO).launch {
            try {
                refreshAll(context, ids)
            } finally {
                pending.finish()
            }
        }
    }

    override fun onAppWidgetOptionsChanged(
        context: Context,
        appWidgetManager: AppWidgetManager,
        appWidgetId: Int,
        newOptions: android.os.Bundle?,
    ) {
        super.onAppWidgetOptionsChanged(context, appWidgetManager, appWidgetId, newOptions)
        renderFromCache(context, intArrayOf(appWidgetId))
    }

    override fun onDeleted(context: Context, appWidgetIds: IntArray) {
        super.onDeleted(context, appWidgetIds)
        appWidgetIds.forEach { WidgetPrefs.clear(context, it) }
    }

    override fun onUpdate(context: Context, appWidgetManager: AppWidgetManager, appWidgetIds: IntArray) {
        super.onUpdate(context, appWidgetManager, appWidgetIds)
        if (appWidgetIds.isEmpty()) return
        renderFromCache(context, appWidgetIds)
        val pending = goAsync()
        CoroutineScope(SupervisorJob() + Dispatchers.IO).launch {
            try {
                refreshAll(context, appWidgetIds)
            } finally {
                pending.finish()
            }
        }
    }

    override fun onReceive(context: Context, intent: Intent) {
        super.onReceive(context, intent)
        when (intent.action) {
            WidgetActions.TOGGLE -> handleToggle(context, intent)
            WidgetActions.REFRESH -> handleRefresh(context, intent)
            WidgetActions.RERENDER -> handleRerender(context, intent)
        }
    }

    private fun handleRerender(context: Context, intent: Intent) {
        val ids = intent.getIntArrayExtra(AppWidgetManager.EXTRA_APPWIDGET_IDS)
            ?: WidgetPrefs.widgetIds(context)
        if (ids.isEmpty()) return
        val manager = AppWidgetManager.getInstance(context)
        for (widgetId in ids) {
            renderOne(context, manager, widgetId, loading = false)
            manager.notifyAppWidgetViewDataChanged(widgetId, R.id.widget_list)
        }
    }

    private fun handleToggle(context: Context, intent: Intent) {
        val widgetId = intent.getIntExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, AppWidgetManager.INVALID_APPWIDGET_ID)
        if (widgetId == AppWidgetManager.INVALID_APPWIDGET_ID) return
        val snapshot = WidgetSnapshotStore.load(context)
        val currentEnabled = snapshot?.filteringEnabled ?: return
        val optimistic = !currentEnabled
        val now = System.currentTimeMillis()
        val newSnap = WidgetSnapshot(
            updatedAtMs = now,
            rows = snapshot.rows,
            filteringEnabled = optimistic,
            filteringKnown = true,
        )
        WidgetSnapshotStore.save(context, newSnap)
        renderFromCache(context, intArrayOf(widgetId))
        val pending = goAsync()
        CoroutineScope(SupervisorJob() + Dispatchers.IO).launch {
            try {
                val result = widgetData.toggleFiltering()
                if (result != null) {
                    val latest = WidgetSnapshotStore.load(context) ?: newSnap
                    WidgetSnapshotStore.save(
                        context,
                        latest.copy(
                            filteringEnabled = result.settings.enabled,
                            filteringKnown = true,
                        ),
                    )
                }
                val manager = AppWidgetManager.getInstance(context)
                manager.notifyAppWidgetViewDataChanged(widgetId, R.id.widget_list)
                renderFromCache(context, intArrayOf(widgetId))
            } finally {
                pending.finish()
            }
        }
    }

    private fun handleRefresh(context: Context, intent: Intent) {
        val widgetId = intent.getIntExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, AppWidgetManager.INVALID_APPWIDGET_ID)
        if (widgetId == AppWidgetManager.INVALID_APPWIDGET_ID) return
        val manager = AppWidgetManager.getInstance(context)
        renderOne(context, manager, widgetId, loading = true)
        val pending = goAsync()
        CoroutineScope(SupervisorJob() + Dispatchers.IO).launch {
            try {
                refreshAll(context, intArrayOf(widgetId))
            } finally {
                pending.finish()
            }
        }
    }

    private fun renderFromCache(context: Context, widgetIds: IntArray) {
        val manager = AppWidgetManager.getInstance(context)
        for (widgetId in widgetIds) {
            renderOne(context, manager, widgetId, loading = false)
        }
    }

    private suspend fun refreshAll(context: Context, widgetIds: IntArray) = runCatching {
        runCatching {
            val mode = appPreferences.themeMode.first()
            WidgetPrefs.prefs(context).edit().putString(WidgetPrefs.KEY_APP_THEME_MODE, mode.name).apply()
        }
        runCatching {
            val style = appPreferences.iconStyle.first()
            WidgetPrefs.prefs(context).edit().putString(WidgetPrefs.KEY_ICON_STYLE, style.name).apply()
        }

        val snapRes = widgetData.resolverSnapshot()
        val now = System.currentTimeMillis()
        val rows = buildRowsFromSnapshot(snapRes)
        val filteringKnown = snapRes.filtering != null
        val filteringEnabled = snapRes.filtering?.settings?.enabled
        val existing = WidgetSnapshotStore.load(context)
        val snapshot = WidgetSnapshot(
            updatedAtMs = now,
            rows = if (rows.isNotEmpty()) rows else (existing?.rows ?: emptyList()),
            filteringEnabled = filteringEnabled ?: existing?.filteringEnabled,
            filteringKnown = filteringKnown || (existing?.filteringKnown == true),
        )
        WidgetSnapshotStore.save(context, snapshot)

        val manager = AppWidgetManager.getInstance(context)
        for (widgetId in widgetIds) {
            manager.notifyAppWidgetViewDataChanged(widgetId, R.id.widget_list)
            renderOne(context, manager, widgetId, loading = false)
        }
    }.onFailure { Timber.d(it, "widget refresh failed") }

    private fun buildRowsFromSnapshot(snap: WidgetData.ResolverSnapshot): List<WidgetResolverRow> {
        val res = snap.resolution
        val stats = snap.stats
        if (stats != null && stats.isNotEmpty()) {
            val current = res?.adaptiveCurrent
            val defaultAddr = res?.upstreams?.firstOrNull()
            val allowedTransports = res?.transports?.map { it.lowercase(Locale.US) }?.toSet()
            val dnscryptEnabled = allowedTransports?.contains("dnscrypt") == true
            return stats.mapIndexed { index, s ->
                val isCurrent = (current != null && s.address == current) || (current == null && index == 0)
                val isDefault = s.address == defaultAddr
                val protoKey = WidgetValues.resolveProtocolKey(s.address)
                val dimmed = !WidgetValues.transportAllowedForProtocol(protoKey, allowedTransports, dnscryptEnabled)
                val measured = s.measured == true
                val ms = if (measured) s.displayLatencyMs.toLong() else null
                val statusKey = when {
                    !measured -> "unknown"
                    ms != null && ms <= 40L -> "healthy"
                    ms != null && ms <= 80L -> "warn"
                    else -> "bad"
                }
                WidgetResolverRow(
                    address = s.address,
                    displayName = WidgetValues.resolverDisplayName(s.address),
                    transportLabel = WidgetValues.resolverTransportLabel(s.address),
                    transportKey = protoKey,
                    providerKey = WidgetValues.providerKeyFromAddress(s.address),
                    latencyText = WidgetValues.formatLatencyLabel(ms, measured),
                    latencyMs = ms,
                    measured = measured,
                    isCurrent = isCurrent,
                    isDefault = isDefault,
                    isDimmed = dimmed,
                    statusKey = statusKey,
                )
            }
        }
        val ups = res?.upstreams ?: emptyList()
        val current = res?.adaptiveCurrent
        val allowedTransports = res?.transports?.map { it.lowercase(Locale.US) }?.toSet()
        val dnscryptEnabled = allowedTransports?.contains("dnscrypt") == true
        return ups.mapIndexed { index, addr ->
            val protoKey = WidgetValues.resolveProtocolKey(addr)
            val dimmed = !WidgetValues.transportAllowedForProtocol(protoKey, allowedTransports, dnscryptEnabled)
            val isCurrent = (current != null && addr == current) || (current == null && index == 0)
            WidgetResolverRow(
                address = addr,
                displayName = WidgetValues.resolverDisplayName(addr),
                transportLabel = WidgetValues.resolverTransportLabel(addr),
                transportKey = protoKey,
                providerKey = WidgetValues.providerKeyFromAddress(addr),
                latencyText = "—",
                latencyMs = null,
                measured = false,
                isCurrent = isCurrent,
                isDefault = index == 0,
                isDimmed = dimmed,
                statusKey = "unknown",
            )
        }
    }

    private fun renderOne(
        context: Context,
        manager: AppWidgetManager,
        widgetId: Int,
        loading: Boolean,
    ) {
        try {
            val mode = WidgetPrefs.resolveMode(context, widgetId)
            val colors = WidgetValues.resolverWidgetColors(mode)
            val alphaPct = WidgetPrefs.alpha(context, widgetId)
            val fontScale = WidgetPrefs.fontScale(context, widgetId)

            val snapshot = WidgetSnapshotStore.load(context)
            val views = RemoteViews(context.packageName, R.layout.widget_resolver)
            views.setInt(
                R.id.widget_resolver_root,
                "setBackgroundColor",
                WidgetValues.bgColorWithAlphaPercent(colors.bgColor, alphaPct),
            )

            val updatedText = if (snapshot == null || snapshot.updatedAtMs <= 0L) "—" else {
                SimpleDateFormat("HH:mm:ss", Locale.getDefault()).format(Date(snapshot.updatedAtMs))
            }
            views.setTextViewText(
                R.id.widget_updated,
                if (loading) context.getString(R.string.widget_refreshing) else updatedText,
            )
            views.setTextColor(R.id.widget_updated, colors.muted)
            views.setTextViewTextSize(
                R.id.widget_updated,
                TypedValue.COMPLEX_UNIT_SP,
                HEADER_TEXT_SP * fontScale,
            )

            views.setViewVisibility(
                R.id.widget_refresh_spinner,
                if (loading) android.view.View.VISIBLE else android.view.View.GONE,
            )
            views.setViewVisibility(
                R.id.widget_refresh,
                if (loading) android.view.View.GONE else android.view.View.VISIBLE,
            )

            val refreshPI = PendingIntent.getBroadcast(
                context,
                widgetId + REFRESH_REQUEST_OFFSET,
                Intent(context, DigitDnsResolverWidgetProvider::class.java).apply {
                    action = WidgetActions.REFRESH
                    putExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, widgetId)
                },
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_MUTABLE,
            )
            views.setOnClickPendingIntent(R.id.widget_refresh, refreshPI)

            val filteringKnown = snapshot?.filteringKnown == true
            val filteringOn = snapshot?.filteringEnabled == true
            val filteringText = when {
                !filteringKnown -> context.getString(R.string.widget_toggle_unavailable)
                filteringOn -> context.getString(R.string.widget_toggle_on)
                else -> context.getString(R.string.widget_toggle_off)
            }
            views.setTextViewText(R.id.widget_filtering, filteringText)
            views.setTextColor(R.id.widget_filtering, if (filteringOn) colors.filteringOn else colors.filteringOff)
            views.setTextViewTextSize(
                R.id.widget_filtering,
                TypedValue.COMPLEX_UNIT_SP,
                FOOTER_TEXT_SP * fontScale,
            )
            val togglePI = PendingIntent.getBroadcast(
                context,
                widgetId + TOGGLE_REQUEST_OFFSET,
                Intent(context, DigitDnsResolverWidgetProvider::class.java).apply {
                    action = WidgetActions.TOGGLE
                    putExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, widgetId)
                },
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_MUTABLE,
            )
            views.setOnClickPendingIntent(R.id.widget_filtering, togglePI)

            // The gear opens the appearance editor directly: a broadcast -> activity
            // hop is blocked by the background activity-start rules on Android 10+.
            val settingsPI = PendingIntent.getActivity(
                context,
                widgetId + SETTINGS_REQUEST_OFFSET,
                Intent(context, DigitDnsResolverWidgetConfigureActivity::class.java).apply {
                    flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
                    putExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, widgetId)
                },
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
            )
            views.setOnClickPendingIntent(R.id.widget_settings, settingsPI)

            val openPI = PendingIntent.getActivity(
                context,
                widgetId,
                Intent(context, MainActivity::class.java).apply {
                    flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
                },
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
            )
            views.setOnClickPendingIntent(R.id.widget_open, openPI)

            val svcIntent = Intent(context, ResolverWidgetService::class.java).apply {
                putExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, widgetId)
                data = Uri.parse(toUri(Intent.URI_INTENT_SCHEME))
            }
            views.setRemoteAdapter(R.id.widget_list, svcIntent)
            views.setEmptyView(R.id.widget_list, R.id.widget_empty)
            views.setTextViewTextSize(
                R.id.widget_empty,
                TypedValue.COMPLEX_UNIT_SP,
                EMPTY_TEXT_SP * fontScale,
            )
            views.setTextColor(R.id.widget_empty, colors.muted)

            val rowOpenPI = PendingIntent.getActivity(
                context,
                widgetId,
                Intent(context, MainActivity::class.java).apply {
                    flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
                },
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_MUTABLE,
            )
            views.setPendingIntentTemplate(R.id.widget_list, rowOpenPI)

            manager.updateAppWidget(widgetId, views)
            manager.notifyAppWidgetViewDataChanged(widgetId, R.id.widget_list)
        } catch (e: Exception) {
            Timber.d(e, "widget render failed for %d", widgetId)
        }
    }
}
