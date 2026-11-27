package com.digitrob.digitdns.core.widget

import android.appwidget.AppWidgetManager
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.res.Configuration
import com.digitrob.digitdns.data.ThemeMode

/**
 * The broadcast actions the widget provider answers. Kept here so the settings UI,
 * the launcher configure screen and the provider all use the same strings.
 */
internal object WidgetActions {
    const val TOGGLE = "com.digitrob.digitdns.widget.TOGGLE"
    const val REFRESH = "com.digitrob.digitdns.widget.REFRESH"

    /** Re-paint from the cached snapshot, no network call (theme/alpha/font changes). */
    const val RERENDER = "com.digitrob.digitdns.widget.RERENDER"
}

/**
 * The per-widget preferences: theme override, background transparency and text scale.
 *
 * These live in their own SharedPreferences file rather than the app DataStore because
 * the widget provider, the RemoteViews service and the launcher configure screen all
 * run outside the activity and need a synchronous read. The app theme and icon style
 * are mirrored in here too, so a widget refresh never needs to open the DataStore.
 */
object WidgetPrefs {
    const val FILE = "digitdns_widget"

    const val KEY_APP_THEME_MODE = "app_theme_mode"
    const val KEY_ICON_STYLE = "icon_style"

    private const val KEY_THEME_OVERRIDE = "theme_override_"
    private const val KEY_ALPHA = "alpha_"
    private const val KEY_FONT_SCALE = "font_scale_"

    /** Sentinel stored when the widget should follow the app theme. */
    const val THEME_APP = "APP"

    const val DEFAULT_ALPHA = 100
    private const val ALPHA_MIN = 0
    private const val ALPHA_MAX = 100
    const val MIN_FONT_SCALE = 0.85f
    const val MAX_FONT_SCALE = 1.20f
    const val DEFAULT_FONT_SCALE = 1.0f

    fun prefs(context: Context) = context.getSharedPreferences(FILE, Context.MODE_PRIVATE)

    fun themeOverride(context: Context, widgetId: Int): String? =
        prefs(context).getString(KEY_THEME_OVERRIDE + widgetId, null)

    fun setThemeOverride(context: Context, widgetId: Int, value: String?) {
        prefs(context).edit().apply {
            if (value == null) remove(KEY_THEME_OVERRIDE + widgetId) else putString(KEY_THEME_OVERRIDE + widgetId, value)
        }.apply()
    }

    fun alpha(context: Context, widgetId: Int): Int =
        prefs(context).getInt(KEY_ALPHA + widgetId, DEFAULT_ALPHA).coerceIn(ALPHA_MIN, ALPHA_MAX)

    fun setAlpha(context: Context, widgetId: Int, percent: Int) {
        prefs(context).edit().putInt(KEY_ALPHA + widgetId, percent.coerceIn(ALPHA_MIN, ALPHA_MAX)).apply()
    }

    fun fontScale(context: Context, widgetId: Int): Float =
        prefs(context).getFloat(KEY_FONT_SCALE + widgetId, DEFAULT_FONT_SCALE)
            .coerceIn(MIN_FONT_SCALE, MAX_FONT_SCALE)

    fun setFontScale(context: Context, widgetId: Int, scale: Float) {
        prefs(context).edit()
            .putFloat(KEY_FONT_SCALE + widgetId, scale.coerceIn(MIN_FONT_SCALE, MAX_FONT_SCALE))
            .apply()
    }

    /** Drop the per-widget choices when the launcher removes the widget. */
    fun clear(context: Context, widgetId: Int) {
        prefs(context).edit()
            .remove(KEY_THEME_OVERRIDE + widgetId)
            .remove(KEY_ALPHA + widgetId)
            .remove(KEY_FONT_SCALE + widgetId)
            .apply()
    }

    /** The ids of every placed Resolver widget. */
    fun widgetIds(context: Context): IntArray =
        AppWidgetManager.getInstance(context)
            .getAppWidgetIds(ComponentName(context, DigitDnsResolverWidgetProvider::class.java))

    /** Resolve the effective theme for one widget: override -> app theme -> system. */
    fun resolveMode(context: Context, widgetId: Int): ThemeMode =
        resolveModeFor(context, themeOverride(context, widgetId))

    /**
     * The same resolution for an unsaved override value, so the appearance editor can
     * preview a theme chip before it is written.
     */
    fun resolveModeFor(context: Context, overrideValue: String?): ThemeMode {
        val appMode = ThemeMode.fromStored(prefs(context).getString(KEY_APP_THEME_MODE, null))
        val systemDark = (context.resources.configuration.uiMode and Configuration.UI_MODE_NIGHT_MASK) ==
            Configuration.UI_MODE_NIGHT_YES
        return WidgetValues.resolveWidgetTheme(
            overrideValue?.let { if (it == THEME_APP) null else ThemeMode.fromStored(it) },
            appMode,
            systemDark,
        )
    }

    /** Ask the provider to repaint from cache (no network). */
    fun requestRerender(context: Context, widgetIds: IntArray = widgetIds(context)) {
        if (widgetIds.isEmpty()) return
        val intent = Intent(context, DigitDnsResolverWidgetProvider::class.java).apply {
            action = WidgetActions.RERENDER
            putExtra(AppWidgetManager.EXTRA_APPWIDGET_IDS, widgetIds)
        }
        context.sendBroadcast(intent)
    }
}
