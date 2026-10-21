package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.ClientActionBody
import com.digitrob.digitdns.core.network.ClientCreateRequest
import com.digitrob.digitdns.core.network.ClientDto
import com.digitrob.digitdns.core.network.ClientUpdateRequest
import com.digitrob.digitdns.core.network.ClientsApi
import com.digitrob.digitdns.domain.model.Client
import com.digitrob.digitdns.domain.model.ClientAction
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Reads and writes clients over the v3 API.
 *
 * **Pagination.** The list endpoint is cursor-paged, and this repository walks every
 * page rather than returning the first. A phone screen wants a searchable full set of
 * odd names ("that Xbox account"), not the first 50 with a "load more" that a search
 * would silently miss; the installs this app targets have tens of clients, not
 * hundreds of thousands, so draining the cursor is cheap and removes an entire class
 * of "why isn't it finding it" bugs.
 *
 * The methods return domain [Client]s, not DTOs, so the derived state (quota
 * fraction, expiry) is computed exactly once, in the mapper.
 */
@Singleton
class ClientsRepository @Inject constructor(
    private val api: ClientsApi,
) {
    /** Every client, following `next_cursor` to exhaustion. */
    suspend fun listAll(): List<Client> {
        val all = mutableListOf<ClientDto>()
        var cursor: String? = null
        do {
            val page = api.list(limit = PAGE_SIZE, cursor = cursor)
            all += page.items
            cursor = page.nextCursor.takeIf { it.isNotBlank() }
        } while (cursor != null)
        return all.map(Client::from)
    }

    suspend fun get(id: String): Client = Client.from(api.get(id))

    suspend fun create(request: ClientCreateRequest): Client = Client.from(api.create(request))

    suspend fun update(id: String, request: ClientUpdateRequest): Client =
        Client.from(api.patch(id, request))

    suspend fun delete(id: String) = api.delete(id)

    /**
     * Run a client action and return whatever the server reported back. The
     * per-action response is small and differs by action, so it is returned whole
     * rather than flattened into a domain type — `regenerate-uuid` returns the new
     * UUID, `regenerate-register-secret` the new secret, and a caller reads the field
     * it asked for.
     */
    suspend fun runAction(id: String, action: ClientAction): ActionResult {
        val body = when (action) {
            is ClientAction.Renew -> ClientActionBody(extendDays = action.extendDays)
            is ClientAction.Toggle -> ClientActionBody(enabled = action.enabled)
            is ClientAction.AddIp -> ClientActionBody(ip = action.ip)
            is ClientAction.RemoveIp -> ClientActionBody(ip = action.ip)
            ClientAction.ResetTraffic,
            ClientAction.RegenerateUuid,
            ClientAction.RegenerateRegisterSecret,
            -> ClientActionBody()
        }
        val response = api.action(id, action.wireName, body)
        return ActionResult(
            success = response.success || response.reset,
            uuid = response.uuid,
            registerSecret = response.registerSecret,
        )
    }

    private companion object {
        /** A page size below the server's 200 cap, so a page is a normal round trip. */
        const val PAGE_SIZE = 200
    }
}

/** The union of what any client action can report back. */
data class ActionResult(
    val success: Boolean,
    /** Set by `regenerate-uuid`. */
    val uuid: String = "",
    /** Set by `regenerate-register-secret`. */
    val registerSecret: String = "",
)
