package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.PUT
import retrofit2.http.Path

/**
 * `custom-groups` — named sets of domains that share one action (proxy/direct/block).
 *
 * The list comes back under `items`, not `groups`: the dashboard endpoint wraps its
 * array in `{"groups":[...]}` and the v3 adapter unwraps and re-wraps it as `items`.
 * Getting that wrong yields a silently empty list, so the DTO names the v3 key.
 *
 * Update is a **full replace** of the four editable fields — `id` and `created_at` are
 * server-owned and preserved across a PUT regardless of what the body carries.
 */
interface GroupsApi {

    @GET("custom-groups")
    suspend fun list(): GroupsListResponse

    @POST("custom-groups")
    suspend fun create(@Body body: CustomGroupRequest): CustomGroupDto

    @PUT("custom-groups/{id}")
    suspend fun update(@Path("id") id: String, @Body body: CustomGroupRequest): CustomGroupDto

    @DELETE("custom-groups/{id}")
    suspend fun delete(@Path("id") id: String): SimpleSuccess
}

@Serializable
data class GroupsListResponse(
    val items: List<CustomGroupDto> = emptyList(),
)

@Serializable
data class CustomGroupDto(
    val id: String = "",
    val name: String = "",
    /** One of `proxy`, `direct`, `block`. */
    val action: String = "",
    val domains: List<String> = emptyList(),
    val enabled: Boolean = false,
    @SerialName("created_at") val createdAt: String = "",
)

/** The create/update body: the four fields the operator owns. */
@Serializable
data class CustomGroupRequest(
    val name: String,
    val action: String,
    val domains: List<String> = emptyList(),
    val enabled: Boolean = true,
)
