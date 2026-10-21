package com.digitrob.digitdns.data

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.intPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import com.digitrob.digitdns.domain.model.Connection
import com.digitrob.digitdns.domain.model.CredentialMode
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import java.util.UUID
import javax.inject.Inject
import javax.inject.Singleton

private val Context.connectionDataStore: DataStore<Preferences> by preferencesDataStore(
    name = "digitdns_connections",
)

/**
 * Where the saved connections live.
 *
 * Host, port, admin path and the TLS preference are *not* secrets on their own (the
 * admin path is unguessable, but it is an address, not a credential), so they sit in
 * a plain Preferences DataStore. The credentials — the session token and the master
 * key — never come near this class; they live in [SecretStore], which is
 * Keystore-backed and keys each secret by the connection [Connection.id]. Keeping the
 * split means a backup or a `run-as` dump of this file leaks nothing usable.
 *
 * Several installs can be saved (an operator with more than one DigitDNS box), each a
 * [Connection] with a stable [Connection.id], and exactly one is *active* — the one the
 * app is currently talking to. The entries are serialised into a handful of
 * indexed preference keys rather than a JSON blob, so a partial read cannot produce a
 * half-parsed profile.
 *
 * The pre-multi-server file stored one connection under un-suffixed keys and a
 * different DataStore name; [migrateLegacyIfNeeded] carries it forward exactly once.
 */
@Singleton
class ConnectionStore @Inject constructor(
    private val context: Context,
) {
    private object Keys {
        const val COUNT = "count"
        const val ACTIVE = "active_id"

        fun host(i: Int) = stringPreferencesKey("host_$i")
        fun port(i: Int) = intPreferencesKey("port_$i")
        fun adminPath(i: Int) = stringPreferencesKey("admin_path_$i")
        fun https(i: Int) = booleanPreferencesKey("https_$i")
        fun credentialMode(i: Int) = stringPreferencesKey("credential_mode_$i")
        fun label(i: Int) = stringPreferencesKey("label_$i")
        fun id(i: Int) = stringPreferencesKey("id_$i")

        val count = intPreferencesKey(COUNT)
        val active = stringPreferencesKey(ACTIVE)
    }

    /** Every saved install, in the order they were added. */
    val servers: Flow<List<Connection>> = context.connectionDataStore.data.map { prefs ->
        val count = prefs[Keys.count] ?: 0
        (0 until count).mapNotNull { i -> readAt(prefs, i) }
    }

    /** The id of the active install, or null when nothing is saved. */
    val activeId: Flow<String?> = context.connectionDataStore.data.map { prefs ->
        prefs[Keys.active]
    }

    /**
     * The active install, or null when the app has never been set up.
     *
     * This is the shape most callers want — the interceptor, the SSE service and the
     * login screen all mean "the one we are talking to" — so it is offered directly
     * rather than making every caller cross-reference the list with the active id.
     */
    val connection: Flow<Connection?> =
        context.connectionDataStore.data.map { prefs ->
            val id = prefs[Keys.active] ?: return@map null
            serversFrom(prefs).firstOrNull { it.id == id }
        }

    /** The active install read synchronously, for the rare non-suspending caller. */
    suspend fun active(): Connection? = connection.first()

    /** Add a new install and make it active. Returns the created profile with an id. */
    suspend fun add(connection: Connection): Connection {
        val stored = connection.withIdIfMissing()
        context.connectionDataStore.edit { prefs ->
            val count = prefs[Keys.count] ?: 0
            writeAt(prefs, count, stored)
            prefs[Keys.count] = count + 1
            prefs[Keys.active] = stored.id
        }
        return stored
    }

    /** Replace the active install's non-identity fields, preserving its id. */
    suspend fun saveActive(connection: Connection) {
        context.connectionDataStore.edit { prefs ->
            val count = prefs[Keys.count] ?: 0
            val index = (0 until count).firstOrNull { i -> prefs[Keys.id(i)] == connection.id } ?: return@edit
            writeAt(prefs, index, connection)
            prefs[Keys.active] = connection.id
        }
    }

    /**
     * Backwards-compatible single-connection save: overwrite the active profile, or
     * create one when none exists. Kept so setup flows written before multi-server
     * keep working without each having to reason about the list.
     */
    suspend fun save(connection: Connection) {
        val existing = active()
        if (existing == null) {
            add(connection)
        } else {
            saveActive(connection.copy(id = existing.id))
        }
    }

    /** Switch the active install. No-op when [id] is not saved. */
    suspend fun setActive(id: String) {
        context.connectionDataStore.edit { prefs ->
            if (serversFrom(prefs).any { it.id == id }) {
                prefs[Keys.active] = id
            }
        }
    }

    /** Remove one install. If it was active, the first remaining install becomes active. */
    suspend fun remove(id: String) {
        context.connectionDataStore.edit { prefs ->
            val remaining = serversFrom(prefs).filterNot { it.id == id }
            prefs.clear()
            remaining.forEachIndexed { i, c -> writeAt(prefs, i, c) }
            prefs[Keys.count] = remaining.size
            val currentActive = prefs[Keys.active]
            if (currentActive == id || currentActive == null) {
                remaining.firstOrNull()?.let { prefs[Keys.active] = it.id }
            }
        }
    }

    suspend fun clear() {
        context.connectionDataStore.edit { it.clear() }
    }

    /**
     * Replace only the active install's admin path, leaving the rest intact.
     *
     * Used after the server rotates the path: every session and bookmark under the old
     * one is dead, so the app must adopt the new path before its next call or it would
     * be talking to a URL the install no longer serves. Doing it as a targeted edit
     * rather than a full [saveActive] means a concurrent change to another field is not
     * clobbered by a value captured before the rotation.
     */
    suspend fun updateAdminPath(adminPath: String) {
        context.connectionDataStore.edit { prefs ->
            val active = prefs[Keys.active] ?: return@edit
            val count = prefs[Keys.count] ?: 0
            val index = (0 until count).firstOrNull { i -> prefs[Keys.id(i)] == active } ?: return@edit
            prefs[Keys.adminPath(index)] = adminPath
        }
    }

    /**
     * Adopt a pre-multi-server install, if one is present, exactly once.
     *
     * The old store used a different DataStore name, so it cannot be read through the
     * new one; instead the app's bootstrap copies the legacy values across on first
     * run. Returns the id assigned to the migrated install, or null when there was
     * nothing to migrate — the caller uses it to move the legacy credentials too.
     */
    suspend fun migrateLegacyIfNeeded(): String? {
        val legacy = runCatching {
            context.getSharedPreferences(LEGACY_PREFS, Context.MODE_PRIVATE)
        }.getOrNull()

        val host = legacy?.getString(LEGACY_HOST, null)
        val port = legacy?.getInt(LEGACY_PORT, 0) ?: 0
        // Migrate only once, and only when there is something real to move: an existing
        // active install means this has already run, and a blank host/port is the
        // leftover file of a version that never saved a connection.
        val shouldMigrate = legacy != null && !host.isNullOrBlank() && port != 0 && active() == null

        val id = if (shouldMigrate) {
            val fresh = UUID.randomUUID().toString()
            add(
                Connection(
                    host = host!!,
                    port = port,
                    adminPath = legacy!!.getString(LEGACY_ADMIN_PATH, "").orEmpty(),
                    isHttps = legacy.getBoolean(LEGACY_HTTPS, true),
                    credentialMode = legacy.getString(LEGACY_CREDENTIAL_MODE, null)
                        ?.let { runCatching { CredentialMode.valueOf(it) }.getOrNull() }
                        ?: CredentialMode.SESSION,
                    label = legacy.getString(LEGACY_LABEL, "").orEmpty(),
                    id = fresh,
                ),
            )
            legacy.edit().clear().apply()
            fresh
        } else {
            null
        }
        return id
    }

    private fun serversFrom(prefs: Preferences): List<Connection> {
        val count = prefs[Keys.count] ?: 0
        return (0 until count).mapNotNull { i -> readAt(prefs, i) }
    }

    private fun readAt(prefs: Preferences, i: Int): Connection? {
        val id = prefs[Keys.id(i)]
        val host = prefs[Keys.host(i)]
        val port = prefs[Keys.port(i)]
        // A profile missing any of its identifying keys is treated as absent rather than
        // defaulted: a half-written row would otherwise surface as a connection to "".
        return if (id == null || host == null || port == null) {
            null
        } else {
            Connection(
                host = host,
                port = port,
                adminPath = prefs[Keys.adminPath(i)].orEmpty(),
                isHttps = prefs[Keys.https(i)] ?: true,
                credentialMode = prefs[Keys.credentialMode(i)]
                    ?.let { runCatching { CredentialMode.valueOf(it) }.getOrNull() }
                    ?: CredentialMode.SESSION,
                label = prefs[Keys.label(i)].orEmpty(),
                id = id,
            )
        }
    }

    private fun writeAt(prefs: androidx.datastore.preferences.core.MutablePreferences, i: Int, c: Connection) {
        prefs[Keys.id(i)] = c.id
        prefs[Keys.host(i)] = c.host
        prefs[Keys.port(i)] = c.port
        prefs[Keys.adminPath(i)] = c.adminPath
        prefs[Keys.https(i)] = c.isHttps
        prefs[Keys.credentialMode(i)] = c.credentialMode.name
        prefs[Keys.label(i)] = c.label
    }

    private fun Connection.withIdIfMissing(): Connection =
        if (id.isBlank()) copy(id = UUID.randomUUID().toString()) else this

    private companion object {
        // The pre-multi-server DataStore file name and its raw keys.
        const val LEGACY_PREFS = "digitdns_connection"
        const val LEGACY_HOST = "host"
        const val LEGACY_PORT = "port"
        const val LEGACY_ADMIN_PATH = "admin_path"
        const val LEGACY_HTTPS = "https"
        const val LEGACY_CREDENTIAL_MODE = "credential_mode"
        const val LEGACY_LABEL = "label"
    }
}
