package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.EgressSaveRequest
import com.digitrob.digitdns.core.network.RelayModeDto
import com.digitrob.digitdns.core.network.RelayModeTestResponse
import com.digitrob.digitdns.data.AgentsRepository
import com.digitrob.digitdns.data.EgressesRepository
import com.digitrob.digitdns.data.RelayModeRepository
import com.digitrob.digitdns.data.RulesRepository
import com.digitrob.digitdns.domain.model.EgressInfo
import com.digitrob.digitdns.domain.model.RelayAgentInfo
import com.digitrob.digitdns.domain.model.RuleLists
import com.digitrob.digitdns.domain.model.normalizedDomain
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import java.util.Locale
import javax.inject.Inject
import timber.log.Timber

/** A key the screen must show once, either from creation or a rotation. */
data class RevealedAgentKey(
    val agentName: String,
    val key: String,
    val rotated: Boolean = false,
)

data class RelayAgentsUiState(
    val agents: List<RelayAgentInfo> = emptyList(),
    /** Hub-side SOCKS5 and WireGuard egresses. */
    val egresses: List<EgressInfo> = emptyList(),
    /** The egress whose Test button is running. */
    val testingEgressId: String? = null,
    /** Full-instance relay hubs this server serves. */
    val relayHubs: List<RelayModeDto> = emptyList(),
    val relayGlobalEnabled: Boolean = true,
    val relaySavingId: String? = null,
    val relayTestingId: String? = null,
    /** Hub id -> last test result. */
    val relayTests: Map<String, RelayModeTestResponse> = emptyMap(),
    /** Agent id -> number of domains that would currently exit through it. */
    val assignedCounts: Map<String, Int> = emptyMap(),
    /** Agent id -> service names whose default relay is that agent. */
    val serviceLabels: Map<String, List<String>> = emptyMap(),
    val isLoading: Boolean = false,
    val isRefreshing: Boolean = false,
    val isSaving: Boolean = false,
    val testingId: String? = null,
    val error: String? = null,
    val message: String? = null,
    /** Set when a create/rotate returns the clear key; the dialog clears it. */
    val revealedKey: RevealedAgentKey? = null,
    /** Text the screen should put on the clipboard, then call [RelayAgentsViewModel.consumeClipboard]. */
    val clipboardText: String? = null,
)

/**
 * The Relay Agents section: the agent catalogue and the operator actions the panel
 * offers. A successful test replaces one card from the server's answer instead of
 * re-listing, so the page does not jump while the badge changes.
 */
@Suppress("TooManyFunctions")
@HiltViewModel
class RelayAgentsViewModel @Inject constructor(
    private val repository: AgentsRepository,
    private val rulesRepository: RulesRepository,
    private val egressesRepository: EgressesRepository,
    private val relayModeRepository: RelayModeRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(RelayAgentsUiState())
    val state: StateFlow<RelayAgentsUiState> = _state.asStateFlow()

    init {
        load()
    }

    fun load() = performLoad(pull = false)

    fun refresh() = performLoad(pull = true)

    /** Silent periodic refresh: latency and health age like the web cards, and
     * no spinner appears unless the list is still empty. */
    fun autoRefresh() {
        if (_state.value.isLoading || _state.value.isRefreshing || _state.value.isSaving) return
        performLoad(pull = false)
    }

    private fun performLoad(pull: Boolean) {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = !pull && it.agents.isEmpty(), isRefreshing = pull, error = null) }
            try {
                val list = repository.list()
                val egresses = runCatching { egressesRepository.list() }.getOrNull()
                val relayEnvelope = runCatching { relayModeRepository.envelope() }.getOrNull()
                val rules = runCatching { rulesRepository.load() }.getOrNull()
                _state.update {
                    it.copy(
                        agents = list,
                        egresses = egresses ?: it.egresses,
                        relayHubs = relayEnvelope?.relayHubs ?: it.relayHubs,
                        relayGlobalEnabled = relayEnvelope?.globalEnabled ?: it.relayGlobalEnabled,
                        assignedCounts = rules?.let(::assignmentCounts).orEmpty(),
                        serviceLabels = rules?.let(::serviceDefaults).orEmpty(),
                        isLoading = false,
                        isRefreshing = false,
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(isLoading = false, isRefreshing = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update {
                    it.copy(isLoading = false, isRefreshing = false, error = e.message ?: "Could not load relay agents.")
                }
            }
        }
    }

    fun dismissMessage() = _state.update { it.copy(message = null) }

    fun dismissKey() = _state.update { it.copy(revealedKey = null) }

    fun consumeClipboard() = _state.update { it.copy(clipboardText = null) }

    fun clearError() = _state.update { it.copy(error = null) }

    fun create(name: String, country: String) {
        val code = country.trim().uppercase(Locale.US)
        if (code.length != 2) {
            _state.update { it.copy(error = "Choose a two-letter country.") }
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(isSaving = true, error = null) }
            try {
                val created = repository.create(name.trim(), code)
                _state.update {
                    it.copy(
                        agents = (it.agents + created.agent).sortedBy { a -> a.displayName.lowercase(Locale.US) },
                        isSaving = false,
                        revealedKey = RevealedAgentKey(created.agent.displayName, created.key),
                        message = "Added ${created.agent.displayName}. Give the agent its key.",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(isSaving = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isSaving = false, error = e.message ?: "Could not create the agent.") }
            }
        }
    }

    fun update(
        id: String,
        name: String,
        country: String,
        publicIp: String,
        publicIpv6: String,
        ipv6Enabled: Boolean,
    ) {
        val code = country.trim().uppercase(Locale.US)
        if (code.length != 2) {
            _state.update { it.copy(error = "Choose a two-letter country.") }
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(isSaving = true, error = null) }
            try {
                val updated = repository.update(
                    id,
                    name.trim(),
                    code,
                    publicIp.trim(),
                    publicIpv6.trim(),
                    ipv6Enabled,
                )
                _state.update { state ->
                    state.copy(
                        agents = state.agents.map { if (it.id == id) updated else it }
                            .sortedBy { a -> a.displayName.lowercase(Locale.US) },
                        isSaving = false,
                        message = "Saved ${updated.displayName}.",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(isSaving = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isSaving = false, error = e.message ?: "Could not save the agent.") }
            }
        }
    }

    fun delete(id: String) {
        viewModelScope.launch {
            val name = _state.value.agents.firstOrNull { it.id == id }?.displayName ?: "agent"
            try {
                repository.delete(id)
                _state.update { state ->
                    state.copy(
                        agents = state.agents.filterNot { it.id == id },
                        message = "Deleted $name. Its assignments fell back to This Server.",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not delete the agent.") }
            }
        }
    }

    fun setEnabled(id: String, enabled: Boolean) {
        viewModelScope.launch {
            val name = _state.value.agents.firstOrNull { it.id == id }?.displayName ?: "agent"
            try {
                repository.setEnabled(id, enabled)
                _state.update { state ->
                    state.copy(
                        agents = state.agents.map { if (it.id == id) it.copy(enabled = enabled) else it },
                        message = if (enabled) "Enabled $name." else "Disabled $name.",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not change the agent.") }
            }
        }
    }

    fun rotateKey(id: String) {
        viewModelScope.launch {
            val name = _state.value.agents.firstOrNull { it.id == id }?.displayName ?: "agent"
            try {
                val key = repository.rotateKey(id)
                _state.update { it.copy(revealedKey = RevealedAgentKey(name, key, rotated = true)) }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not rotate the key.") }
            }
        }
    }

    /** Fetch the authenticated clear key and hand it to the screen for the clipboard. */
    fun copyKey(id: String) {
        viewModelScope.launch {
            try {
                val key = repository.copyKey(id)
                _state.update { it.copy(clipboardText = key, message = "Agent key copied.") }
            } catch (e: ApiException) {
                val message = if (e.isConflict) {
                    "This agent key predates encrypted storage. Rotate the key to store a copy."
                } else {
                    e.userMessage
                }
                _state.update { it.copy(error = message) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not read the agent key.") }
            }
        }
    }

    fun copyIp(ip: String) {
        if (ip.isBlank()) return
        _state.update { it.copy(clipboardText = ip, message = "Public IP copied.") }
    }

    fun test(id: String) {
        viewModelScope.launch {
            _state.update { it.copy(testingId = id, error = null) }
            try {
                val result = repository.test(id)
                if (result.ok) {
                    val tested = result.agent
                    _state.update { state ->
                        val agents = if (tested != null) {
                            state.agents.map { if (it.id == id) tested else it }
                        } else {
                            state.agents
                        }
                        state.copy(
                            agents = agents,
                            testingId = null,
                            message = String.format(Locale.US, "Connection OK · %.1f ms", result.pingMs),
                        )
                    }
                } else {
                    _state.update {
                        it.copy(
                            testingId = null,
                            error = "Connection failed: ${result.error.ifBlank { "no route to the relay port" }}",
                        )
                    }
                }
            } catch (e: ApiException) {
                Timber.w("agent test failed: %s", e.userMessage)
                _state.update { it.copy(testingId = null, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update {
                    it.copy(testingId = null, error = e.message ?: "Could not test the agent.")
                }
            }
        }
    }

    fun createEgress(body: EgressSaveRequest) {
        viewModelScope.launch {
            _state.update { it.copy(isSaving = true, error = null) }
            try {
                val created = egressesRepository.create(body)
                _state.update {
                    it.copy(
                        egresses = (it.egresses + created).sortedBy { e -> e.displayName.lowercase(Locale.US) },
                        isSaving = false,
                        message = "Added ${created.displayName}.",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(isSaving = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isSaving = false, error = e.message ?: "Could not add the egress.") }
            }
        }
    }

    fun updateEgress(id: String, body: EgressSaveRequest) {
        viewModelScope.launch {
            _state.update { it.copy(isSaving = true, error = null) }
            try {
                val updated = egressesRepository.update(id, body)
                _state.update {
                    it.copy(
                        egresses = it.egresses.map { e -> if (e.id == id) updated else e },
                        isSaving = false,
                        message = "Saved ${updated.displayName}.",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(isSaving = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isSaving = false, error = e.message ?: "Could not save the egress.") }
            }
        }
    }

    fun deleteEgress(id: String) {
        viewModelScope.launch {
            try {
                egressesRepository.delete(id)
                _state.update {
                    it.copy(
                        egresses = it.egresses.filterNot { e -> e.id == id },
                        message = "Egress deleted.",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not delete the egress.") }
            }
        }
    }

    fun setEgressEnabled(id: String, enabled: Boolean) {
        viewModelScope.launch {
            try {
                egressesRepository.setEnabled(id, enabled)
                _state.update {
                    it.copy(
                        egresses = it.egresses.map { e -> if (e.id == id) e.copy(enabled = enabled) else e },
                        message = if (enabled) "Egress enabled." else "Egress disabled.",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not update the egress.") }
            }
        }
    }

    fun testEgress(id: String) {
        viewModelScope.launch {
            _state.update { it.copy(testingEgressId = id, error = null) }
            try {
                val result = egressesRepository.test(id)
                val list = runCatching { egressesRepository.list() }.getOrNull()
                _state.update {
                    it.copy(
                        egresses = list ?: it.egresses,
                        testingEgressId = null,
                        message = if (result.ok) {
                            "Egress online — ${String.format(Locale.US, "%.0f", result.latencyMs)} ms"
                        } else {
                            "Egress test failed — ${result.error.ifBlank { "unreachable" }}"
                        },
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(testingEgressId = null, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(testingEgressId = null, error = e.message ?: "Egress test failed.") }
            }
        }
    }

    fun setRelayGlobal(enabled: Boolean) {
        viewModelScope.launch {
            try {
                val hubs = relayModeRepository.setGlobal(enabled)
                _state.update { it.copy(relayGlobalEnabled = enabled, relayHubs = hubs) }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not change relay mode.") }
            }
        }
    }

    fun refreshRelayMode() {
        viewModelScope.launch {
            try {
                val hubs = relayModeRepository.list()
                _state.update { it.copy(relayHubs = hubs) }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not load relay hubs.") }
            }
        }
    }

    fun testRelayHub(id: String) {
        if (id.isBlank()) return
        viewModelScope.launch {
            _state.update { it.copy(relayTestingId = id, error = null) }
            try {
                val result = relayModeRepository.testHub(id)
                _state.update {
                    it.copy(
                        relayTestingId = null,
                        relayTests = it.relayTests + (id to result),
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(relayTestingId = null, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(relayTestingId = null, error = e.message ?: "Relay test failed.") }
            }
        }
    }

    fun deleteRelayHub(id: String) {
        if (id.isBlank()) return
        viewModelScope.launch {
            try {
                val hubs = relayModeRepository.delete(id)
                _state.update { it.copy(relayHubs = hubs, message = "Relay hub removed.") }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not remove the relay hub.") }
            }
        }
    }

    fun saveRelayHub(
        id: String,
        name: String,
        enabled: Boolean,
        hubUrl: String,
        agentKey: String,
        token: String,
    ) {
        viewModelScope.launch {
            _state.update { it.copy(relaySavingId = id.ifBlank { "new" }, error = null) }
            try {
                val hubs = relayModeRepository.save(
                    id = id,
                    name = name,
                    enabled = enabled,
                    hubUrl = hubUrl.trim(),
                    agentKey = agentKey.trim(),
                    token = token.trim(),
                )
                _state.update {
                    it.copy(
                        relayHubs = hubs,
                        relaySavingId = null,
                        message = if (enabled) "Relay hub saved." else "Relay hub disabled.",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(relaySavingId = null, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(relaySavingId = null, error = e.message ?: "Could not save the relay hub.") }
            }
        }
    }}

/**
 * Count the domains that resolve to each agent: direct assignments first, then a
 * service default for every domain that has no direct assignment. `__local__` and
 * `__inherit__` are choices, not agent ids.
 */
private fun assignmentCounts(rules: RuleLists): Map<String, Int> {
    val domains = mutableMapOf<String, MutableSet<String>>()
    fun add(agentId: String, domain: String) {
        if (agentId.isBlank() || agentId == "__local__" || agentId == "__inherit__") return
        domains.getOrPut(agentId) { mutableSetOf() }.add(domain.normalizedDomain())
    }
    rules.domainAgents.forEach { (pattern, agentId) -> add(agentId, pattern) }
    rules.serviceAgents.forEach { (serviceId, agentId) ->
        rules.domainServices.forEach { (domain, sid) ->
            if (sid != serviceId) return@forEach
            val key = domain.normalizedDomain()
            if (rules.domainAgents.containsKey(key)) return@forEach
            add(agentId, key)
        }
    }
    return domains.mapValues { (_, set) -> set.size }
}

/** Service names whose default relay points at each agent. */
private fun serviceDefaults(rules: RuleLists): Map<String, List<String>> {
    val out = mutableMapOf<String, MutableList<String>>()
    rules.serviceAgents.forEach { (serviceId, agentId) ->
        if (agentId.isBlank() || agentId == "__local__" || agentId == "__inherit__") return@forEach
        out.getOrPut(agentId) { mutableListOf() }.add(rules.services[serviceId] ?: serviceId)
    }
    return out.mapValues { (_, names) -> names.distinct().sorted() }
}
