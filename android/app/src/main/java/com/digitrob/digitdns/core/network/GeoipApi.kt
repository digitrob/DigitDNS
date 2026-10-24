package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.http.GET
import retrofit2.http.POST

/**
 * The server-location database: the country resolved for the install's public
 * address, plus the age of the GeoLite2 file behind it.
 *
 * `update` performs the same download the daemon's daily loop does. It is a POST
 * because it has a side effect (it replaces the file), not because it carries a
 * body — none is sent.
 */
interface GeoipApi {

    @GET("settings/geoip")
    suspend fun status(): GeoipStatusDto

    @POST("settings/geoip/update")
    suspend fun update(): GeoipStatusDto
}

@Serializable
data class GeoipStatusDto(
    val available: Boolean = false,
    val country: String = "",
    @SerialName("database_date") val databaseDate: String = "",
    @SerialName("updated_at") val updatedAt: String = "",
    val source: String = "",
    val error: String = "",
)
