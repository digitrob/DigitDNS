package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import okhttp3.MultipartBody
import okhttp3.ResponseBody
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.Multipart
import retrofit2.http.POST
import retrofit2.http.Part
import retrofit2.http.Query
import retrofit2.http.Streaming

/**
 * The migration importer. The web panel and this app share one contract: a
 * source is analyzed into a normalized, secret-free bundle, previewed, and only
 * then applied. Android sends AdGuard YAML and Pi-hole v5 Teleporter archives to
 * the server for parsing; a Pi-hole v6 gravity.db is read locally with Android's
 * SQLite before the normalized bundle is posted.
 */
interface MigrateApi {

    @Multipart
    @POST("migrate/analyze")
    suspend fun analyzeFile(@Part file: MultipartBody.Part): MigratePreviewDto

    @POST("migrate/analyze")
    suspend fun analyzeBundle(@Body body: MigrateAnalyzeRequest): MigratePreviewDto

    @POST("migrate/apply")
    suspend fun apply(@Body body: MigrateApplyRequest): MigrateApplyResultDto

    @POST("migrate/undo")
    suspend fun undo(@Body body: MigrateUndoRequest): SimpleSuccess

    /**
     * The skipped-regex report for an analyzed import. It is tied to the preview
     * token, so it must be fetched before Apply consumes that token.
     */
    @Streaming
    @GET("migrate/report")
    suspend fun report(@Query("token") token: String): ResponseBody
}

@Serializable
data class MigrateAnalyzeRequest(
    val bundle: MigrateBundleDto,
    @SerialName("pihole_toml") val piholeToml: String = "",
)

@Serializable
data class MigrateApplyRequest(
    val token: String,
    val upstreams: Boolean = false,
)

@Serializable
data class MigrateUndoRequest(
    val token: String,
)

@Serializable
data class MigrateSourceDto(
    val kind: String = "",
    val version: String = "",
    @SerialName("file_name") val fileName: String = "",
    val fingerprint: String = "",
)

@Serializable
data class MigrateBundleDto(
    @SerialName("schema_version") val schemaVersion: Int = 1,
    val source: MigrateSourceDto = MigrateSourceDto(),
    val records: List<MigrateRecordDto> = emptyList(),
    val clients: List<MigrateClientDto> = emptyList(),
    val proxied: List<String> = emptyList(),
    val blocked: List<String> = emptyList(),
    val direct: List<String> = emptyList(),
    val allowlist: List<String> = emptyList(),
    @SerialName("filter_lists") val filterLists: List<MigrateFilterListDto> = emptyList(),
    val rewrites: List<MigrateRewriteDto> = emptyList(),
    val filtering: MigrateFilteringDto? = null,
    val upstreams: List<String> = emptyList(),
    @SerialName("upstreams_opt_in") val upstreamsOptIn: Boolean = false,
    val warnings: List<MigrateIssueDto> = emptyList(),
    val skipped: List<MigrateIssueDto> = emptyList(),
    val stats: MigrateStatsDto = MigrateStatsDto(),
)

@Serializable
data class MigrateRecordDto(
    val name: String = "",
    val type: String = "A",
    val value: String = "",
    val ttl: Long = 0,
)

@Serializable
data class MigrateClientDto(
    val name: String = "",
    val ids: List<String> = emptyList(),
    val enabled: Boolean = true,
    @SerialName("filtering_enabled") val filteringEnabled: Boolean? = null,
    val upstreams: List<String> = emptyList(),
    val note: String = "",
)

@Serializable
data class MigrateFilterListDto(
    val name: String = "",
    val url: String = "",
    val homepage: String = "",
    val license: String = "",
    val enabled: Boolean = true,
    val content: String = "",
)

@Serializable
data class MigrateRewriteDto(
    val domain: String = "",
    val target: String = "",
)

@Serializable
data class MigrateFilteringDto(
    val enabled: Boolean? = null,
    @SerialName("block_mode") val blockMode: String = "",
    @SerialName("block_ttl") val blockTtl: Int? = null,
    @SerialName("update_interval_hours") val updateIntervalHours: Int? = null,
    @SerialName("custom_rules") val customRules: String = "",
)

@Serializable
data class MigrateIssueDto(
    val code: String = "",
    val item: String = "",
    val reason: String = "",
)

@Serializable
data class MigrateStatsDto(
    val records: Int = 0,
    val clients: Int = 0,
    val proxied: Int = 0,
    val blocked: Int = 0,
    val direct: Int = 0,
    val allowlist: Int = 0,
    @SerialName("filter_lists") val filterLists: Int = 0,
    @SerialName("filter_rules") val filterRules: Int = 0,
    val rewrites: Int = 0,
    val upstreams: Int = 0,
    val warnings: Int = 0,
    val skipped: Int = 0,
)

@Serializable
data class MigrateSampleDto(
    val proxied: List<String> = emptyList(),
    val blocked: List<String> = emptyList(),
    val direct: List<String> = emptyList(),
    val allowlist: List<String> = emptyList(),
    val records: List<String> = emptyList(),
    val clients: List<String> = emptyList(),
    @SerialName("filter_lists") val filterLists: List<String> = emptyList(),
    @SerialName("custom_rules") val customRules: List<String> = emptyList(),
    val rewrites: List<String> = emptyList(),
    val upstreams: List<String> = emptyList(),
)

@Serializable
data class MigratePreviewDto(
    val token: String = "",
    @SerialName("expires_at") val expiresAt: String = "",
    val source: MigrateSourceDto = MigrateSourceDto(),
    val stats: MigrateStatsDto = MigrateStatsDto(),
    val warnings: List<MigrateIssueDto> = emptyList(),
    val skipped: List<MigrateIssueDto> = emptyList(),
    val filtering: MigrateFilteringDto? = null,
    @SerialName("upstreams_opt_in") val upstreamsOptIn: Boolean = false,
    @SerialName("report_available") val reportAvailable: Boolean = false,
    val sample: MigrateSampleDto = MigrateSampleDto(),
)

@Serializable
data class MigrateApplyResultDto(
    val success: Boolean = false,
    val added: MigrateStatsDto = MigrateStatsDto(),
    val skipped: List<MigrateIssueDto> = emptyList(),
    @SerialName("undo_token") val undoToken: String = "",
)
