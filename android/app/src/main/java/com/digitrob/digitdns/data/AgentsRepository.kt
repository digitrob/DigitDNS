package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.AgentCreateRequest
import com.digitrob.digitdns.core.network.AgentTestResponse
import com.digitrob.digitdns.core.network.AgentUpdateRequest
import com.digitrob.digitdns.core.network.AgentsApi
import com.digitrob.digitdns.domain.model.AgentTestResult
import com.digitrob.digitdns.domain.model.CreatedAgent
import com.digitrob.digitdns.domain.model.RelayAgentInfo
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Reads and writes relay agents over v3.
 *
 * The write calls return the server's new snapshot so the screen can update a
 * single card without re-listing; [list] remains the source of truth after any
 * assignment change made elsewhere.
 */
@Singleton
class AgentsRepository @Inject constructor(
    private val api: AgentsApi,
) {
    suspend fun list(): List<RelayAgentInfo> =
        api.list().agents.map(RelayAgentInfo::from)

    suspend fun create(name: String, country: String): CreatedAgent {
        val response = api.create(AgentCreateRequest(name = name, country = country))
        return CreatedAgent(
            agent = response.agent?.let(RelayAgentInfo::from) ?: error("the server returned no agent"),
            key = response.key,
        )
    }

    suspend fun update(
        id: String,
        name: String,
        country: String,
        publicIp: String,
        publicIpv6: String,
        ipv6Enabled: Boolean,
    ): RelayAgentInfo {
        val response = api.update(
            id,
            AgentUpdateRequest(
                name = name,
                country = country,
                publicIp = publicIp,
                publicIpv6 = publicIpv6,
                ipv6Enabled = ipv6Enabled,
            ),
        )
        return response.agent?.let(RelayAgentInfo::from) ?: error("the server returned no agent")
    }

    suspend fun delete(id: String) {
        api.delete(id)
    }

    suspend fun setEnabled(id: String, enabled: Boolean) {
        if (enabled) api.enable(id) else api.disable(id)
    }

    suspend fun rotateKey(id: String): String = api.rotateKey(id).key

    /**
     * The clear key stored encrypted at rest. A 409 here means the agent enrolled
     * before that copy existed; rotating the key is the documented fix.
     */
    suspend fun copyKey(id: String): String = api.key(id).key

    suspend fun test(id: String): AgentTestResult {
        val response = api.test(id)
        return response.toResult()
    }

    private fun AgentTestResponse.toResult() = AgentTestResult(
        ok = ok,
        pingMs = pingMs,
        address = address,
        error = error,
        agent = agent?.let(RelayAgentInfo::from),
    )
}
