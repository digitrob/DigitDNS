package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.CustomGroupRequest
import com.digitrob.digitdns.core.network.GroupsApi
import com.digitrob.digitdns.domain.model.CustomGroup
import com.digitrob.digitdns.domain.model.GroupAction
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Custom groups over the v3 API. Small enough to be a straight pass-through, but kept
 * as a repository so the ViewModel maps DTO→domain in one place and the screen never
 * sees a `CustomGroupDto`.
 *
 * There is no pagination here: the endpoint returns the whole set under `items`.
 */
@Singleton
class GroupsRepository @Inject constructor(
    private val api: GroupsApi,
) {
    suspend fun list(): List<CustomGroup> = api.list().items.map(CustomGroup::from)

    suspend fun create(group: PendingGroup): CustomGroup =
        CustomGroup.from(api.create(group.toRequest()))

    suspend fun update(id: String, group: PendingGroup): CustomGroup =
        CustomGroup.from(api.update(id, group.toRequest()))

    suspend fun delete(id: String) = api.delete(id)
}

/** The editable fields of a group, before it has an id. */
data class PendingGroup(
    val name: String,
    val action: GroupAction,
    val domains: List<String>,
    val enabled: Boolean,
) {
    fun toRequest(): CustomGroupRequest = CustomGroupRequest(
        name = name,
        action = action.wire,
        domains = domains,
        enabled = enabled,
    )
}
