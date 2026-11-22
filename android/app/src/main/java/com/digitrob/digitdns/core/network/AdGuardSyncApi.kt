package com.digitrob.digitdns.core.network

import kotlinx.serialization.EncodeDefault
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Path

/**
 * The v3 AdGuard upstream-coverage sync contract.
 *
 * The app posts a pasted list to the server, which owns parsing and the checksum.
 * Preview and apply are separate calls on purpose: the exact text and checksum
 * returned by preview are carried into apply so a source edited behind the review
 * screen cannot be applied from a stale preview. The Android form has no URL field
 * and no refresh button; the alias endpoints for fetched sources still exist here
 * because saved URL-backed sources can be refreshed from other clients.
 *
 * Paths are relative to the API root; [ServerUrlProvider] adds `/api/v3/`.
 */
interface AdGuardSyncApi {

    @GET("sync/adguard/sources")
    suspend fun sources(): List<AdGuardSourceDto>

    @POST("sync/adguard/sources")
    suspend fun saveSource(@Body body: AdGuardSourceSaveRequest): AdGuardSourceDto

    @DELETE("sync/adguard/sources/{id}")
    suspend fun deleteSource(@Path("id") id: String): SimpleSuccess?

    @POST("sync/adguard/sources/{id}/refresh")
    suspend fun refreshSource(@Path("id") id: String): AdGuardRefreshResultDto

    @GET("sync/adguard/sources/{id}/export")
    suspend fun exportSource(@Path("id") id: String): AdGuardExportDto

    @POST("sync/adguard/preview")
    suspend fun preview(@Body body: AdGuardPreviewRequest): AdGuardPreviewDto

    @POST("sync/adguard/apply")
    suspend fun apply(@Body body: AdGuardApplyRequest): AdGuardApplyResultDto
}

@Serializable
data class AdGuardSourceDto(
    val id: String = "",
    val name: String = "",
    val url: String = "",
    val text: String = "",
    val enabled: Boolean = true,
    @SerialName("last_checksum") val lastChecksum: String = "",
    @SerialName("last_applied_at") val lastAppliedAt: String = "",
    @SerialName("managed_upstream") val managedUpstream: String = "",
    @SerialName("managed_patterns") val managedPatterns: List<String> = emptyList(),
    val imported: List<String> = emptyList(),
    @SerialName("updated_at") val updatedAt: String = "",
)

/**
 * The dashboard card sends only these three fields when it saves a list. The API keeps
 * `enabled` and `managed_upstream` on its response records for older clients, but this
 * client deliberately omits both from every write.
 */
@Serializable
data class AdGuardSourceSaveRequest(
    @SerialName("source_id") val sourceId: String = "",
    val name: String = "",
    val text: String = "",
)

@Serializable
data class AdGuardPreviewRequest(
    @SerialName("source_id") val sourceId: String = "",
    val name: String = "",
    val text: String = "",
    // The dashboard always sends the add-only guardrails explicitly, including the
    // implicit risk acknowledgement for broad/conflicting rows.
    @EncodeDefault @SerialName("domain_actions") val domainActions: Map<String, String> = emptyMap(),
    @EncodeDefault @SerialName("removal_mode") val removalMode: String = "keep",
    @EncodeDefault @SerialName("confirm_mirror") val confirmMirror: Boolean = false,
    @EncodeDefault @SerialName("acknowledge_risk") val acknowledgeRisk: Boolean = true,
)

@Serializable
data class AdGuardApplyRequest(
    @SerialName("source_id") val sourceId: String = "",
    val name: String = "",
    // The exact text and checksum from preview, never a re-serialization of the form.
    val text: String = "",
    val checksum: String = "",
    @SerialName("import") val importDomains: List<String> = emptyList(),
    @EncodeDefault @SerialName("removal_mode") val removalMode: String = "keep",
    @EncodeDefault @SerialName("confirm_mirror") val confirmMirror: Boolean = false,
    @EncodeDefault @SerialName("acknowledge_risk") val acknowledgeRisk: Boolean = true,
    // Per-domain route actions default to proxy on the server when omitted, so old
    // clients keep their original behaviour. The importer sends only staged patterns.
    @EncodeDefault @SerialName("domain_actions") val domainActions: Map<String, String> = emptyMap(),
)

@Serializable
data class AdGuardCountsDto(
    val covered: Int = 0,
    @SerialName("adguardOnly") val adguardOnly: Int = 0,
    @SerialName("digitdnsOnly") val digitdnsOnly: Int = 0,
    val conflict: Int = 0,
)

@Serializable
data class AdGuardNoticeDto(
    val kind: String = "",
    val title: String = "",
    val message: String = "",
    val targets: List<String> = emptyList(),
    val domains: List<String> = emptyList(),
)

@Serializable
data class AdGuardRowDto(
    val domain: String = "",
    val kind: String = "",
    val bucket: String = "",
    val group: String = "",
    val target: String = "",
    @SerialName("target_is_self") val targetIsSelf: Boolean = false,
    @SerialName("covered_by") val coveredBy: String = "",
    val warning: String = "",
    // Additive server fields: absent on older installs, so these defaults keep
    // the v5.10 client behaviour while still drawing a useful review row.
    val broad: Boolean = false,
    @SerialName("in_digitdns") val inDigitDns: Boolean = false,
    @SerialName("suggested_action") val suggestedAction: String = "proxy",
    val variants: List<String> = emptyList(),
)

@Serializable
data class AdGuardPatternDto(
    val value: String = "",
    val kind: String = "",
    val group: String = "",
    val target: String = "",
    @SerialName("targetIsSelf") val targetIsSelf: Boolean = false,
    val broad: Boolean = false,
)

@Serializable
data class AdGuardGroupDto(
    val label: String = "",
    val target: String = "",
    @SerialName("targetIsSelf") val targetIsSelf: Boolean = false,
    val patterns: List<AdGuardPatternDto> = emptyList(),
)

@Serializable
data class AdGuardPreviewDto(
    @SerialName("source_id") val sourceId: String = "",
    val name: String = "",
    val enabled: Boolean = true,
    // The server echoes the normalized list; apply must send this value verbatim.
    val text: String = "",
    val checksum: String = "",
    val upstream: String = "",
    val groups: List<AdGuardGroupDto> = emptyList(),
    val rows: List<AdGuardRowDto> = emptyList(),
    val managed: List<AdGuardRowDto> = emptyList(),
    val counts: AdGuardCountsDto = AdGuardCountsDto(),
    val warnings: List<String> = emptyList(),
    @SerialName("generated_block") val generatedBlock: String = "",
    val candidates: Int = 0,
    // Additive fields: older installs omit them and the UI derives from rows/warnings.
    val notices: List<AdGuardNoticeDto> = emptyList(),
    @SerialName("broad_count") val broadCount: Int = 0,
    @SerialName("foreign_targets") val foreignTargets: List<String> = emptyList(),
)

@Serializable
data class AdGuardRefreshResultDto(
    val source: AdGuardSourceDto = AdGuardSourceDto(),
    val warning: String = "",
)

@Serializable
data class AdGuardExportDto(
    val block: String = "",
    val filename: String = "",
)

@Serializable
data class AdGuardApplyResultDto(
    val success: Boolean = false,
    val source: AdGuardSourceDto = AdGuardSourceDto(),
    val counts: AdGuardCountsDto = AdGuardCountsDto(),
    val imported: Int = 0,
    val removed: Int = 0,
    @SerialName("generated_block") val generatedBlock: String = "",
    // Additive v5.11 apply counters. The UI no longer saves a generated block.
    @SerialName("route_changes") val routeChanges: Int = 0,
    @SerialName("adblock_added") val adblockAdded: Int = 0,
)
