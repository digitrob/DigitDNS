package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.http.GET
import retrofit2.http.POST

/**
 * The install's own state: version, health, cache, and self-update.
 *
 * [status] bundles a health string, the full version info and the live telemetry
 * object (the same shape `/stats` returns). [check] reports whether a newer release
 * exists; on a network failure it still answers 200 with `update_available=false` and
 * an `error`, so a caller must read the error field rather than assume success.
 *
 * There is no changelog, release-notes or download-URL field on this surface — the
 * status DTO carries no such member because the server sends none.
 */
interface SystemApi {

    @GET("status")
    suspend fun status(): SystemStatus

    @GET("update/check")
    suspend fun checkUpdate(): UpdateCheck

    @POST("update/apply")
    suspend fun applyUpdate(): UpdateApplyResponse

    @GET("update/status")
    suspend fun updateStatus(): UpdateProgress

    @POST("cache/flush")
    suspend fun flushCache(): CacheFlushResponse
}

@Serializable
data class SystemStatus(
    val status: String = "",
    val version: VersionInfo? = null,
    @SerialName("public_ip") val publicIp: String = "",
    @SerialName("api_bind") val apiBind: String = "",
    @SerialName("api_bind_list") val apiBindList: List<String> = emptyList(),
    @SerialName("api_exposure") val apiExposure: String = "",
)

@Serializable
data class VersionInfo(
    val version: String = "",
    val channel: String = "",
    val codename: String = "",
    val display: String = "",
    val hash: String = "",
    val commit: String = "",
    @SerialName("go") val goVersion: String = "",
)

@Serializable
data class VersionResponse(
    val version: String = "",
    val display: String = "",
    val hash: String = "",
    val commit: String = "",
    val api: String = "",
)

@Serializable
data class UpdateCheck(
    val current: String = "",
    val latest: String = "",
    val codename: String = "",
    val channel: String = "",
    @SerialName("update_available") val updateAvailable: Boolean = false,
    val supported: Boolean = false,
    val note: String = "",
    /** Set only when the check itself failed; the response is still a 200. */
    val error: String = "",
)

@Serializable
data class UpdateApplyResponse(
    val started: Boolean = false,
)

@Serializable
data class UpdateProgress(
    val running: Boolean = false,
    /** idle | resolve | download | verify | backup | swap | restart | done | error. */
    val phase: String = "",
    val percent: Int = 0,
    val message: String = "",
    val restarting: Boolean = false,
    val error: String = "",
)

@Serializable
data class CacheFlushResponse(
    val flushed: Boolean = false,
)
