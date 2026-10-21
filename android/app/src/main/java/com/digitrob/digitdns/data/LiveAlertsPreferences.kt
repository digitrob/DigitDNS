package com.digitrob.digitdns.data

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import javax.inject.Inject
import javax.inject.Singleton

private val Context.liveAlertsDataStore: DataStore<Preferences> by preferencesDataStore(
    name = "digitdns_live_alerts",
)

/**
 * Whether the operator wants live alerts.
 *
 * A plain preference, not a secret, so it sits in ordinary preferences rather than
 * [com.digitrob.digitdns.core.security.SecretStore]. Default off: the stream holds a
 * foreground service and a persistent notification, and starting that without the
 * operator asking would be a surprise, so the app only connects after an explicit
 * opt-in.
 */
@Singleton
class LiveAlertsPreferences @Inject constructor(
    private val context: Context,
) {
    private object Keys {
        val enabled = booleanPreferencesKey("enabled")
    }

    val enabled: Flow<Boolean> = context.liveAlertsDataStore.data.map { prefs ->
        prefs[Keys.enabled] ?: false
    }

    suspend fun setEnabled(enabled: Boolean) {
        context.liveAlertsDataStore.edit { prefs ->
            prefs[Keys.enabled] = enabled
        }
    }
}
