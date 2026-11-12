package com.digitrob.digitdns.core.network

import kotlinx.serialization.Serializable
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST

/** The global DNS policy engine switch (`/api/v3/engine`). */
interface EngineApi {

    @GET("engine")
    suspend fun status(): EngineState

    @POST("engine")
    suspend fun set(@Body body: EngineState): EngineState
}

@Serializable
data class EngineState(
    // No default: the shared Json config omits default-valued properties, and
    // the server requires `enabled` on POST. With `= true` here, turning the
    // engine back on serialised to `{}` and was rejected; turning it off sent
    // `false` and worked, which is exactly the one-way switch symptom.
    val enabled: Boolean,
)
