package com.digitrob.digitdns.data

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import javax.inject.Inject
import javax.inject.Singleton

private val Context.appPrefsDataStore: DataStore<Preferences> by preferencesDataStore(
    name = "digitdns_app_prefs",
)

/**
 * The dashboard cards a phone can show, in the panel's own vocabulary.
 *
 * Each is its own toggle so an operator who never looks at CPU can drop it, and the
 * order is stored, not derived, so the grid can be rearranged. The enum name is the
 * stable key written to storage; renaming one would reset a user's layout, so these
 * names are effectively part of the on-disk contract.
 */
enum class DashboardCard {
    QUERY_RATE,
    RAM_CACHE,
    SNI_PROXY,
    CPU_USAGE,
    RAM_USAGE,
    LIVE_TRAFFIC,
    TELEMETRY_CHART,
    UPSTREAM_RACERS,
    RESOLVER_LATENCY,
    ;

    /** Whether the card is shown by default on a fresh install. */
    val enabledByDefault: Boolean
        get() = true

    companion object {
        val defaultOrder: List<DashboardCard> = entries.toList()

        /** Cards a fresh install hides until the operator switches them on. */
        val defaultDisabled: Set<DashboardCard> = entries.filterNot { it.enabledByDefault }.toSet()

        /** Parse a stored comma-separated order, ignoring unknown names. */
        fun parseOrder(stored: String?): List<DashboardCard> {
            if (stored.isNullOrBlank()) return defaultOrder
            val known = stored.split(',').mapNotNull { name -> entries.firstOrNull { it.name == name } }
            // Any card the stored order does not name (a newly added one) is appended, so
            // an upgrade never silently hides a new card.
            return known + defaultOrder.filterNot { it in known }
        }

        /** Parse the stored disabled set, falling back to the off-by-default cards. */
        fun parseDisabled(stored: String?): Set<DashboardCard> {
            if (stored.isNullOrBlank()) return defaultDisabled
            return stored.split(',').mapNotNull { name -> entries.firstOrNull { it.name == name } }.toSet()
        }
    }
}

/** The three ways the app can decide between its dark and light schemes. */
enum class ThemeMode {
    /** Follow the OS setting. The default, so the app matches the rest of the phone. */
    SYSTEM,

    DARK,

    LIGHT,
    ;

    companion object {
        /** Parse a stored value, falling back to [SYSTEM] for anything unrecognised. */
        fun fromStored(value: String?): ThemeMode =
            entries.firstOrNull { it.name == value } ?: SYSTEM
    }
}

/**
 * App-level preferences that are neither secrets nor server connection data: today,
 * just the theme. A plain Preferences DataStore, like
 * [LiveAlertsPreferences][com.digitrob.digitdns.data.LiveAlertsPreferences] — these are
 * UI choices, not credentials, so they do not belong in `SecretStore`.
 *
 * Default is [ThemeMode.SYSTEM]: the panel ships dark as its fallback, but a phone user
 * expects an app to honour the system setting until told otherwise.
 */
@Singleton
class AppPreferences @Inject constructor(
    private val context: Context,
) {
    private object Keys {
        val themeMode = stringPreferencesKey("theme_mode")
        val dashboardOrder = stringPreferencesKey("dashboard_order")
        val dashboardDisabled = stringPreferencesKey("dashboard_disabled")
    }

    val themeMode: Flow<ThemeMode> = context.appPrefsDataStore.data.map { prefs ->
        ThemeMode.fromStored(prefs[Keys.themeMode])
    }

    suspend fun setThemeMode(mode: ThemeMode) {
        context.appPrefsDataStore.edit { prefs ->
            prefs[Keys.themeMode] = mode.name
        }
    }

    /** The saved dashboard card order, defaulting to the panel's order. */
    val dashboardOrder: Flow<List<DashboardCard>> = context.appPrefsDataStore.data.map { prefs ->
        DashboardCard.parseOrder(prefs[Keys.dashboardOrder])
    }

    /** The dashboard cards the operator has switched off. */
    val dashboardDisabled: Flow<Set<DashboardCard>> = context.appPrefsDataStore.data.map { prefs ->
        DashboardCard.parseDisabled(prefs[Keys.dashboardDisabled])
    }

    suspend fun setDashboardOrder(order: List<DashboardCard>) {
        context.appPrefsDataStore.edit { prefs ->
            prefs[Keys.dashboardOrder] = order.joinToString(",") { it.name }
        }
    }

    suspend fun setDashboardCardEnabled(card: DashboardCard, enabled: Boolean) {
        context.appPrefsDataStore.edit { prefs ->
            val current = DashboardCard.parseDisabled(prefs[Keys.dashboardDisabled]).toMutableSet()
            if (enabled) current.remove(card) else current.add(card)
            prefs[Keys.dashboardDisabled] = current.joinToString(",") { it.name }
        }
    }

    /** Put a disabled card back on, at the end of the order, when its switch is flipped. */
    suspend fun reenableDashboardCard(card: DashboardCard) = setDashboardCardEnabled(card, true)

    /** Existing disabled state for a single card, for a settings row. */
    fun isDisabled(disabled: Set<DashboardCard>, card: DashboardCard): Boolean = card in disabled
}
