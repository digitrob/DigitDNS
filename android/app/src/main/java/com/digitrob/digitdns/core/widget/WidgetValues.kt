package com.digitrob.digitdns.core.widget

import com.digitrob.digitdns.core.network.FilteringSettingsDto
import com.digitrob.digitdns.core.network.FilteringSettingsRequest
import java.util.Locale

/**
 * Pure formatting and request-building helpers shared by the home-screen widgets.
 *
 * Keeping these out of the AppWidgetProviders means the parts worth pinning down —
 * a stable QPS rendering and the "change only `enabled`" save body — can be unit
 * tested without standing up RemoteViews or a BroadcastReceiver.
 */
internal object WidgetValues {

    /** Locale.US matches the panel's telemetry formatting; bad readings stay honest. */
    fun formatQps(qps: Float): String {
        if (!qps.isFinite()) return "—"
        return String.format(Locale.US, "%.1f", qps)
    }

    /**
     * The save endpoint requires every filtering setting, not just `enabled`.
     * Reusing the status we just read keeps block mode, TTL, allowlist and custom
     * rules intact when the widget or Quick Settings tile flips one switch.
     */
    fun filteringToggleRequest(settings: FilteringSettingsDto, enabled: Boolean): FilteringSettingsRequest =
        FilteringSettingsRequest(
            enabled = enabled,
            blockMode = settings.blockMode,
            blockTtl = settings.blockTtl,
            updateIntervalHours = settings.updateIntervalHours,
            allowlist = settings.allowlist.orEmpty(),
            customRules = settings.customRules,
        )
}
