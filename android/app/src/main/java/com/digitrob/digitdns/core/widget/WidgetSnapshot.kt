package com.digitrob.digitdns.core.widget

import android.content.Context
import org.json.JSONArray
import org.json.JSONObject

internal data class WidgetResolverRow(
    val address: String,
    val displayName: String,
    val transportLabel: String,
    val transportKey: String,
    val providerKey: String?,
    val latencyText: String,
    val latencyMs: Long?,
    val measured: Boolean,
    val isCurrent: Boolean,
    val isDefault: Boolean,
    val isDimmed: Boolean,
    val statusKey: String, // healthy | warn | bad | unknown
)

internal data class WidgetSnapshot(
    val updatedAtMs: Long,
    val rows: List<WidgetResolverRow>,
    val filteringEnabled: Boolean?,
    val filteringKnown: Boolean,
)

internal object WidgetSnapshotStore {
    private const val PREFS = "digitdns_widget"
    private const val KEY_SNAPSHOT_JSON = "snapshot_json"

    fun load(context: Context): WidgetSnapshot? {
        val raw = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).getString(KEY_SNAPSHOT_JSON, null) ?: return null
        return runCatching { parse(raw) }.getOrNull()
    }

    fun save(context: Context, snapshot: WidgetSnapshot) {
        val json = toJson(snapshot).toString()
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit().putString(KEY_SNAPSHOT_JSON, json).apply()
    }

    private fun toJson(snapshot: WidgetSnapshot): JSONObject {
        val root = JSONObject()
        root.put("updatedAtMs", snapshot.updatedAtMs)
        root.put("filteringKnown", snapshot.filteringKnown)
        snapshot.filteringEnabled?.let { root.put("filteringEnabled", it) } ?: root.put("filteringEnabled", JSONObject.NULL)
        val rows = JSONArray()
        snapshot.rows.forEach { row ->
            val obj = JSONObject()
            obj.put("address", row.address)
            obj.put("displayName", row.displayName)
            obj.put("transportLabel", row.transportLabel)
            obj.put("transportKey", row.transportKey)
            obj.put("providerKey", row.providerKey ?: JSONObject.NULL)
            obj.put("latencyText", row.latencyText)
            if (row.latencyMs != null) obj.put("latencyMs", row.latencyMs) else obj.put("latencyMs", JSONObject.NULL)
            obj.put("measured", row.measured)
            obj.put("isCurrent", row.isCurrent)
            obj.put("isDefault", row.isDefault)
            obj.put("isDimmed", row.isDimmed)
            obj.put("statusKey", row.statusKey)
            rows.put(obj)
        }
        root.put("rows", rows)
        return root
    }

    private fun parse(raw: String): WidgetSnapshot {
        val root = JSONObject(raw)
        val updatedAtMs = root.optLong("updatedAtMs", 0L)
        val filteringKnown = root.optBoolean("filteringKnown", false)
        val filteringEnabled = if (root.isNull("filteringEnabled")) null else root.optBoolean("filteringEnabled")
        val rowsArr = root.optJSONArray("rows") ?: JSONArray()
        val rows = mutableListOf<WidgetResolverRow>()
        for (i in 0 until rowsArr.length()) {
            val obj = rowsArr.optJSONObject(i) ?: continue
            rows.add(
                WidgetResolverRow(
                    address = obj.optString("address", ""),
                    displayName = obj.optString("displayName", "—"),
                    transportLabel = obj.optString("transportLabel", "Plain"),
                    transportKey = obj.optString("transportKey", "plain"),
                    providerKey = if (obj.isNull("providerKey")) null else obj.optString("providerKey").takeIf { it.isNotBlank() },
                    latencyText = obj.optString("latencyText", "—"),
                    latencyMs = if (obj.isNull("latencyMs")) null else obj.optLong("latencyMs"),
                    measured = obj.optBoolean("measured", false),
                    isCurrent = obj.optBoolean("isCurrent", false),
                    isDefault = obj.optBoolean("isDefault", false),
                    isDimmed = obj.optBoolean("isDimmed", false),
                    statusKey = obj.optString("statusKey", "unknown"),
                ),
            )
        }
        return WidgetSnapshot(updatedAtMs, rows, filteringEnabled, filteringKnown)
    }
}
