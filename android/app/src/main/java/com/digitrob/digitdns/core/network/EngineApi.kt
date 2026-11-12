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
data class EngineState(val enabled: Boolean = true)
