package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.PATCH
import retrofit2.http.POST
import retrofit2.http.Path
import retrofit2.http.Query

/**
 * The v3 ad-blocking surface, from `internal/api/openapi/v3.json` and the
 * dashboard handlers it delegates to.
 *
 * The status document is deliberately the same one the web panel reads: settings,
 * installed lists, the curated preset catalogue, cached preset counts and the
 * compiled totals. Keeping one shape means the phone and the browser cannot show
 * different counts for the same install.
 */
interface FilteringApi {

    @GET("filtering")
    suspend fun status(): FilteringStatusDto

    @POST("filtering/settings")
    suspend fun saveSettings(@Body body: FilteringSettingsRequest): FilteringStatusDto

    @POST("filtering/refresh")
    suspend fun refreshAll(): SimpleSuccess

    @GET("filtering/presets")
    suspend fun presets(): FilteringPresetsDto

    /**
     * Ask the live matcher how it would classify a name. Read-only: nothing is
     * added or recompiled by this call.
     */
    @GET("filter/test")
    suspend fun testRule(@Query("domain") domain: String): RuleTestDto

    @POST("filtering/presets")
    suspend fun addPresets(@Body body: PresetAddRequest): PresetAddResultDto

    @POST("filtering/lists")
    suspend fun addList(@Body body: FilterListAddRequest): FilterListDto

    @PATCH("filtering/lists/{id}")
    suspend fun setEnabled(
        @Path("id") id: String,
        @Body body: FilterListEnabledRequest,
    ): SimpleSuccess

    @DELETE("filtering/lists/{id}")
    suspend fun delete(@Path("id") id: String): SimpleSuccess

    @POST("filtering/lists/{id}/update")
    suspend fun update(@Path("id") id: String): SimpleSuccess
}

@Serializable
data class FilteringStatusDto(
    val settings: FilteringSettingsDto = FilteringSettingsDto(),
    val lists: List<FilterListDto> = emptyList(),
    val presets: List<FilterPresetDto> = emptyList(),
    @SerialName("preset_counts") val presetCounts: List<PresetCountDto> = emptyList(),
    @SerialName("preset_counts_running") val presetCountsRunning: Boolean = false,
    @SerialName("total_rules") val totalRules: Int = 0,
    @SerialName("total_domains") val totalDomains: Int = 0,
    val updating: List<String> = emptyList(),
)

@Serializable
data class FilteringSettingsDto(
    val enabled: Boolean = true,
    @SerialName("block_mode") val blockMode: String = "null_ip",
    @SerialName("block_ttl") val blockTtl: Int = 10,
    @SerialName("update_interval_hours") val updateIntervalHours: Int = 24,
    /** The server encodes an empty Go slice as `null`; both mean "no entries". */
    val allowlist: List<String>? = null,
    @SerialName("custom_rules") val customRules: String = "",
)

/**
 * The save body. Every field is required and has no default, so kotlinx serialization
 * always encodes it: the server validates `block_mode` even when the operator leaves
 * the default `null_ip` selected.
 */
@Serializable
data class FilteringSettingsRequest(
    val enabled: Boolean,
    @SerialName("block_mode") val blockMode: String,
    @SerialName("block_ttl") val blockTtl: Int,
    @SerialName("update_interval_hours") val updateIntervalHours: Int,
    val allowlist: List<String>,
    @SerialName("custom_rules") val customRules: String,
)

@Serializable
data class FilterListDto(
    val id: String = "",
    val name: String = "",
    val url: String = "",
    val homepage: String = "",
    val license: String = "",
    @SerialName("preset_id") val presetId: String = "",
    val enabled: Boolean = false,
    val format: String = "",
    @SerialName("last_updated") val lastUpdated: String = "",
    @SerialName("last_error") val lastError: String = "",
    @SerialName("rule_count") val ruleCount: Int = 0,
    @SerialName("domain_count") val domainCount: Int = 0,
    @SerialName("allowed_count") val allowedCount: Int = 0,
    val skipped: Int = 0,
    val bytes: Int = 0,
    val etag: String = "",
    @SerialName("last_modified") val lastModified: String = "",
    @SerialName("source_uploaded") val sourceUploaded: Boolean = false,
)

@Serializable
data class FilterPresetDto(
    val id: String = "",
    val name: String = "",
    val group: String = "",
    val url: String = "",
    val homepage: String = "",
    val license: String = "",
    val description: String = "",
    val warning: String = "",
    val recommended: Boolean = false,
    val base: Boolean = false,
)

@Serializable
data class PresetCountDto(
    @SerialName("preset_id") val presetId: String = "",
    @SerialName("domain_count") val domainCount: Int = 0,
    @SerialName("rule_count") val ruleCount: Int = 0,
    val bytes: Int = 0,
    @SerialName("last_updated") val lastUpdated: String = "",
    @SerialName("last_error") val lastError: String = "",
    val etag: String = "",
    @SerialName("last_modified") val lastModified: String = "",
)

@Serializable
data class FilteringPresetsDto(
    val presets: List<FilterPresetDto> = emptyList(),
)

@Serializable
data class PresetAddRequest(
    val ids: List<String>,
)

@Serializable
data class PresetAddResultDto(
    val added: List<FilterListDto> = emptyList(),
    val skipped: List<PresetSkipDto> = emptyList(),
    val failed: List<PresetSkipDto> = emptyList(),
)

@Serializable
data class PresetSkipDto(
    val id: String = "",
    val name: String = "",
    val reason: String = "",
)

@Serializable
data class FilterListAddRequest(
    val name: String,
    val url: String = "",
    val homepage: String = "",
    val content: String = "",
)

@Serializable
data class FilterListEnabledRequest(
    val enabled: Boolean,
)

@Serializable
data class RuleTestDto(
    val domain: String = "",
    val action: String = "DIRECT",
    val rule: String = "",
    val pattern: String = "",
)
