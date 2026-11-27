package com.digitrob.digitdns.core.widget

import android.appwidget.AppWidgetManager
import android.content.Context
import android.content.Intent
import android.graphics.Color
import android.util.TypedValue
import android.widget.RemoteViews
import android.widget.RemoteViewsService
import com.digitrob.digitdns.R
import com.digitrob.digitdns.data.ThemeMode
import dagger.hilt.android.AndroidEntryPoint

@AndroidEntryPoint
class ResolverWidgetService : RemoteViewsService() {
    override fun onGetViewFactory(intent: Intent): RemoteViewsFactory =
        ResolverWidgetFactory(applicationContext, intent.getIntExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, -1))
}

private class ResolverWidgetFactory(
    private val context: Context,
    private val appWidgetId: Int,
) : RemoteViewsService.RemoteViewsFactory {
    private var rows: List<WidgetResolverRow> = emptyList()
    private var colors: WidgetThemeColors = WidgetValues.resolverWidgetColors(ThemeMode.DARK)
    private var fontScale: Float = WidgetPrefs.DEFAULT_FONT_SCALE

    override fun onCreate() = Unit

    override fun onDataSetChanged() {
        val snapshot = WidgetSnapshotStore.load(context)
        rows = snapshot?.rows ?: emptyList()
        colors = WidgetValues.resolverWidgetColors(WidgetPrefs.resolveMode(context, appWidgetId))
        fontScale = WidgetPrefs.fontScale(context, appWidgetId)
    }

    override fun onDestroy() {
        rows = emptyList()
    }

    override fun getCount(): Int = rows.size

    override fun getViewAt(position: Int): RemoteViews {
        val views = RemoteViews(context.packageName, R.layout.widget_resolver_row)
        val row = rows.getOrNull(position) ?: return views
        bindName(views, row)
        bindTransport(views, row)
        bindLatency(views, row)
        bindIcon(views, row)
        bindStatusDot(views, row)
        bindCurrentRow(views, row)
        views.setOnClickFillInIntent(R.id.widget_row_root, Intent())
        return views
    }

    private fun bindName(views: RemoteViews, row: WidgetResolverRow) {
        views.setTextViewText(R.id.widget_row_name, row.displayName)
        views.setTextColor(R.id.widget_row_name, if (row.isDimmed) colors.muted else colors.title)
        views.setTextViewTextSize(
            R.id.widget_row_name,
            TypedValue.COMPLEX_UNIT_SP,
            NAME_TEXT_SP * fontScale,
        )
    }

    private fun bindTransport(views: RemoteViews, row: WidgetResolverRow) {
        views.setTextViewText(R.id.widget_row_transport, row.transportLabel)
        views.setTextColor(R.id.widget_row_transport, transportColor(row.transportKey))
        views.setTextViewTextSize(
            R.id.widget_row_transport,
            TypedValue.COMPLEX_UNIT_SP,
            TRANSPORT_TEXT_SP * fontScale,
        )
    }

    private fun transportColor(transportKey: String): Int {
        val key = transportKey.lowercase()
        return when {
            key.contains("doh3") || key.contains("h3") -> colors.transportDoh3
            key.contains("doh") || key == "https" -> colors.transportDoh
            key.contains("dot") || key == "tls" -> colors.transportDot
            key.contains("doq") || key == "quic" -> colors.transportDoq
            key.contains("dnscrypt") -> colors.transportDnscrypt
            else -> colors.transportPlain
        }
    }

    private fun bindLatency(views: RemoteViews, row: WidgetResolverRow) {
        views.setTextViewText(R.id.widget_row_latency, row.latencyText)
        views.setTextColor(R.id.widget_row_latency, latencyColor(row.latencyMs))
        views.setTextViewTextSize(
            R.id.widget_row_latency,
            TypedValue.COMPLEX_UNIT_SP,
            LATENCY_TEXT_SP * fontScale,
        )
    }

    private fun latencyColor(ms: Long?): Int = when (WidgetValues.latencyBucket(ms)) {
        "good" -> colors.latencyGood
        "warn" -> colors.latencyWarn
        "bad" -> colors.latencyBad
        else -> colors.muted
    }

    private fun bindIcon(views: RemoteViews, row: WidgetResolverRow) {
        val icon = WidgetValues.providerIconRes(row.address) ?: R.mipmap.ic_launcher
        views.setImageViewResource(R.id.widget_row_icon, icon)
    }

    private fun bindStatusDot(views: RemoteViews, row: WidgetResolverRow) {
        val dotColor = when (row.statusKey) {
            "healthy" -> colors.dotHealthy
            "warn" -> colors.dotWarn
            "bad" -> colors.dotBad
            else -> colors.dotUnknown
        }
        views.setInt(R.id.widget_row_dot, "setBackgroundColor", dotColor)
    }

    private fun bindCurrentRow(views: RemoteViews, row: WidgetResolverRow) {
        if (row.isCurrent) {
            views.setInt(R.id.widget_row_root, "setBackgroundResource", R.drawable.widget_row_current)
            views.setTextViewText(R.id.widget_row_badge, "FASTEST")
            views.setViewVisibility(R.id.widget_row_badge, android.view.View.VISIBLE)
            views.setTextColor(R.id.widget_row_badge, colors.fastestText)
            views.setInt(R.id.widget_row_badge, "setBackgroundResource", R.drawable.widget_badge_bg)
            views.setTextViewTextSize(
                R.id.widget_row_badge,
                TypedValue.COMPLEX_UNIT_SP,
                BADGE_TEXT_SP * fontScale,
            )
        } else {
            views.setInt(R.id.widget_row_root, "setBackgroundColor", Color.TRANSPARENT)
            views.setViewVisibility(R.id.widget_row_badge, android.view.View.GONE)
        }
    }

    override fun getLoadingView(): RemoteViews? = null

    override fun getViewTypeCount(): Int = 1

    override fun getItemId(position: Int): Long = position.toLong()

    override fun hasStableIds(): Boolean = true

    private companion object {
        const val NAME_TEXT_SP = 12.5f
        const val TRANSPORT_TEXT_SP = 9.5f
        const val LATENCY_TEXT_SP = 11f
        const val BADGE_TEXT_SP = 8.5f
    }
}
