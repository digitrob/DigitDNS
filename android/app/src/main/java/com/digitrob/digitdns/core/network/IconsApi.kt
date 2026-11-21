package com.digitrob.digitdns.core.network

import kotlinx.serialization.Serializable
import retrofit2.http.GET
import retrofit2.http.Query

/** `GET /api/v3/icons/search?q=…` — the dashboard's Iconify search proxy. */
interface IconsApi {
    @GET("icons/search")
    suspend fun search(@Query("q") query: String): IconSearchResponse
}

@Serializable
data class IconSearchResponse(val icons: List<String> = emptyList())
