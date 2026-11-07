package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.FilterListAddRequest
import com.digitrob.digitdns.core.network.FilterListDto
import com.digitrob.digitdns.core.network.FilterListEnabledRequest
import com.digitrob.digitdns.core.network.FilteringApi
import com.digitrob.digitdns.core.network.FilteringSettingsRequest
import com.digitrob.digitdns.core.network.FilteringStatusDto
import com.digitrob.digitdns.core.network.PresetAddRequest
import com.digitrob.digitdns.core.network.PresetAddResultDto
import com.digitrob.digitdns.core.network.RuleTestDto
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The blocklist engine. Every mutation returns the server's canonical state through
 * a follow-up [status] read in the view model, so the roster never drifts from what
 * the daemon actually compiled.
 */
@Singleton
class FilteringRepository @Inject constructor(
    private val api: FilteringApi,
) {
    suspend fun status(): FilteringStatusDto = api.status()

    suspend fun saveSettings(settings: FilteringSettingsRequest): FilteringStatusDto =
        api.saveSettings(settings)

    suspend fun refreshAll() {
        api.refreshAll()
    }

    suspend fun addPreset(id: String): PresetAddResultDto =
        api.addPresets(PresetAddRequest(listOf(id)))

    suspend fun addPresets(ids: List<String>): PresetAddResultDto =
        api.addPresets(PresetAddRequest(ids))

    suspend fun addList(
        name: String,
        url: String,
        homepage: String,
        content: String,
    ): FilterListDto = api.addList(
        FilterListAddRequest(
            name = name,
            url = url,
            homepage = homepage,
            content = content,
        ),
    )

    suspend fun setEnabled(id: String, enabled: Boolean) {
        api.setEnabled(id, FilterListEnabledRequest(enabled))
    }

    suspend fun delete(id: String) {
        api.delete(id)
    }

    suspend fun update(id: String) {
        api.update(id)
    }

    /** Read-only rule probe against the live matcher. */
    suspend fun testRule(domain: String): RuleTestDto = api.testRule(domain)
}
