package com.digitrob.digitdns.core.network

import kotlinx.serialization.Serializable
import retrofit2.http.GET
import retrofit2.http.Query

/** The bounded daemon log tail (`/api/v3/system-log`). */
interface SystemLogApi {

    @GET("system-log")
    suspend fun tail(@Query("limit") limit: Int = 2000): SystemLogResponse
}

@Serializable
data class SystemLogResponse(val lines: List<String> = emptyList())
