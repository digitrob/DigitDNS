package com.digitrob.digitdns.ui.rules

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.data.RulesRepository
import com.digitrob.digitdns.domain.model.RuleLists
import com.digitrob.digitdns.ui.components.RouteAction
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import java.util.Locale
import javax.inject.Inject

/** How the domains list is ordered. Mirrors the panel's own sort select. */
enum class DomainSort(val label: String) {
    ADDED_DESC("Added (newest)"),
    ADDED_ASC("Added (oldest)"),
    AZ("Name A–Z"),
    ZA("Name Z–A"),
    STATUS("Status (on first)"),
}

/** The status a domain row is in: an active proxy rule, a paused proxy rule, blocked, or direct. */
enum class DomainStatus(val label: String) {
    PROXY("Proxy"),
    BLOCKED("Blocked"),
    PAUSED("Paused"),
    DIRECT("Direct"),
}

/**
 * The status chips shown above the list. A chip is a toggle: tapping the active chip
 * clears it, and with no chip selected the list shows every status. [DIRECT] surfaces
 * the direct-resolved set, which otherwise has no rows of its own.
 */
enum class DomainFilter(val label: String) {
    PROXY("Proxy"),
    BLOCKED("Blocked"),
    PAUSED("Paused"),
    DIRECT("Direct"),
}

/** One row of the domains list: a proxied (active/paused), blocked, or direct domain. */
data class DomainRow(
    val domain: String,
    val status: DomainStatus,
) {
    /** A proxied row's switch is on; paused is off. Other statuses carry no switch. */
    val enabled: Boolean get() = status == DomainStatus.PROXY
}

/**
 * The editable state of the Domains screen and the pinned-record map.
 *
 * The screen projects every custom rule that lives on the Domains tab — the proxied set
 * (active + paused), the blocked set and the direct set — because those are the rules an
 * operator edits here. With no chip selected the list shows all of them; a chip narrows
 * it to one status.
 */
data class RulesUiState(
    val lists: RuleLists = RuleLists(),
    val search: String = "",
    val sort: DomainSort = DomainSort.ADDED_DESC,
    val filter: DomainFilter? = null,
    val isLoading: Boolean = false,
    /** True only for a pull-to-refresh gesture, so the header Reload button never spins the indicator. */
    val isRefreshing: Boolean = false,
    val isSaving: Boolean = false,
    val error: String? = null,
    val savedMessage: String? = null,
) {
    /** Every editable domain, tagged with its status. */
    private val allRows: List<DomainRow>
        get() = buildList {
            // distinct(): the saved list on a live install can carry the same domain
            // twice (the panel accepts a paste that repeats an entry), and two rows
            // with the same LazyColumn key crash the Domains tab on open. One row per
            // domain is also what the operator means.
            lists.proxied.distinct().forEach { add(DomainRow(it, DomainStatus.PROXY)) }
            lists.proxiedDisabled.distinct().forEach { add(DomainRow(it, DomainStatus.PAUSED)) }
            lists.blocked.distinct().forEach { add(DomainRow(it, DomainStatus.BLOCKED)) }
            lists.direct.distinct().forEach { add(DomainRow(it, DomainStatus.DIRECT)) }
        }

    /** The domains after the active chip, search and sort, ready to render. */
    val rows: List<DomainRow>
        get() {
            val status = filter?.let {
                when (it) {
                    DomainFilter.PROXY -> DomainStatus.PROXY
                    DomainFilter.BLOCKED -> DomainStatus.BLOCKED
                    DomainFilter.PAUSED -> DomainStatus.PAUSED
                    DomainFilter.DIRECT -> DomainStatus.DIRECT
                }
            }
            val filtered = if (status == null) allRows else allRows.filter { it.status == status }
            val searched = filtered.filter {
                search.isBlank() || it.domain.contains(search.trim(), ignoreCase = true)
            }
            return when (sort) {
                DomainSort.ADDED_DESC -> searched.reversed()
                DomainSort.ADDED_ASC -> searched
                DomainSort.AZ -> searched.sortedBy { it.domain.lowercase(Locale.US) }
                DomainSort.ZA -> searched.sortedByDescending { it.domain.lowercase(Locale.US) }
                DomainSort.STATUS -> searched.sortedBy { it.status.ordinal }
            }
        }

    val countLabel: String
        get() = if (filter == null) "${rows.size} domains" else "${rows.size} ${filter.label.lowercase()} domains"
}

/**
 * Edits the proxied and blocked domain lists (and the paused subset of the proxied
 * list) plus the pinned-record map, and performs the routing quick actions that move a
 * domain between the four custom lists.
 */
@HiltViewModel
class RulesViewModel @Inject constructor(
    private val repository: RulesRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(RulesUiState())
    val state: StateFlow<RulesUiState> = _state.asStateFlow()

    init {
        load()
    }

    /** The header Reload button's path: refreshes without engaging the pull indicator. */
    fun load() = performLoad(pull = false)

    /** The pull-to-refresh gesture's path: shows the indicator for the duration. */
    fun refresh() = performLoad(pull = true)

    private fun performLoad(pull: Boolean) {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = true, isRefreshing = pull, error = null) }
            try {
                val lists = repository.load()
                _state.update { it.copy(lists = lists, isLoading = false, isRefreshing = false) }
            } catch (e: ApiException) {
                _state.update { it.copy(isLoading = false, isRefreshing = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update {
                    it.copy(isLoading = false, isRefreshing = false, error = e.message ?: "Could not load rules.")
                }
            }
        }
    }

    fun setSearch(value: String) = _state.update { it.copy(search = value) }
    fun setSort(value: DomainSort) = _state.update { it.copy(sort = value) }

    /** Toggle a status chip: tapping the active chip clears it (show all statuses). */
    fun setFilter(value: DomainFilter) = _state.update {
        it.copy(filter = if (it.filter == value) null else value)
    }

    fun dismissMessage() = _state.update { it.copy(savedMessage = null) }

    /** Add a proxied domain (bulk paste: one or more names, any separators). */
    fun addDomain(raw: String) {
        val domains = parseDomains(raw)
        if (domains.isEmpty()) return
        viewModelScope.launch {
            val merged = (_state.value.lists.proxied + domains).distinct()
            write(_state.value.lists.copy(proxied = merged), "Added ${domains.size} domain(s).")
        }
    }

    /**
     * Flip a proxied domain on or off. A proxied row's switch parks it in the paused
     * list; a paused row's switch brings it back.
     */
    fun toggleDomain(domain: String, enabled: Boolean) {
        viewModelScope.launch {
            val lists = _state.value.lists
            val next = if (enabled) {
                lists.copy(proxied = lists.proxied - domain, proxiedDisabled = lists.proxiedDisabled + domain)
            } else {
                lists.copy(proxiedDisabled = lists.proxiedDisabled - domain, proxied = lists.proxied + domain)
            }
            write(next, if (enabled) "Paused $domain." else "Resumed $domain.")
        }
    }

    /** Remove one domain from whichever custom list it lives in. The UI confirms first. */
    fun removeDomain(domain: String) {
        viewModelScope.launch {
            val lists = _state.value.lists
            val next = lists.copy(
                proxied = lists.proxied - domain,
                proxiedDisabled = lists.proxiedDisabled - domain,
                blocked = lists.blocked - domain,
                direct = lists.direct - domain,
            )
            write(next, "Removed $domain.")
        }
    }

    /**
     * Apply a routing quick action to [domain], where [domain] is the already-resolved
     * match target (e.g. `*.target.com`). Every action starts from a clean slate — a
     * domain lives in exactly one list — so switching it from blocked to proxied is a
     * move, not an addition to both, exactly as the web panel does.
     */
    fun routeAction(action: RouteAction, domain: String) {
        if (domain.isBlank()) return
        viewModelScope.launch {
            val lists = _state.value.lists
            val stripped = lists.copy(
                proxied = lists.proxied - domain,
                proxiedDisabled = lists.proxiedDisabled - domain,
                blocked = lists.blocked - domain,
                direct = lists.direct - domain,
            )
            val next = when (action) {
                RouteAction.Proxy -> stripped.copy(proxied = stripped.proxied + domain)
                RouteAction.Block -> stripped.copy(blocked = stripped.blocked + domain)
                RouteAction.Direct -> stripped.copy(direct = stripped.direct + domain)
                RouteAction.Remove -> stripped
            }
            val verb = when (action) {
                RouteAction.Proxy -> "Proxying"
                RouteAction.Block -> "Blocking"
                RouteAction.Direct -> "Routing direct"
                RouteAction.Remove -> "Removed"
            }
            write(next, "$verb $domain.")
        }
    }

    /** Replace the pinned-record map, e.g. from a static-record add or remove. */
    fun saveRecords(records: Map<String, String>) {
        viewModelScope.launch {
            write(_state.value.lists.copy(records = records), "Records saved.")
        }
    }

    /** Pin a hostname to an IP. Both sides must be non-blank; the map is replaced whole. */
    fun addRecord(domain: String, ip: String) {
        val host = normalize(domain)
        val address = ip.trim()
        if (host.isEmpty() || address.isEmpty()) return
        val next = _state.value.lists.records + (host to address)
        viewModelScope.launch { write(_state.value.lists.copy(records = next), "Pinned $host → $address.") }
    }

    /** Remove a pinned record. */
    fun removeRecord(domain: String) {
        val next = _state.value.lists.records - domain
        viewModelScope.launch { write(_state.value.lists.copy(records = next), "Removed $domain.") }
    }

    private suspend fun write(next: RuleLists, message: String? = null) {
        _state.update { it.copy(lists = next, isSaving = true, error = null) }
        try {
            val ok = repository.save(next)
            _state.update {
                it.copy(isSaving = false, savedMessage = if (ok) message else null, error = if (ok) null else "Save failed.")
            }
        } catch (e: ApiException) {
            _state.update { it.copy(isSaving = false, error = e.userMessage) }
        } catch (e: IOException) {
            _state.update { it.copy(isSaving = false, error = e.message ?: "Could not save rules.") }
        }
    }

    /** Trim whitespace and a stray trailing dot; lowercase so the list stays consistent. */
    private fun normalize(raw: String): String =
        raw.trim().trimEnd('.').lowercase(Locale.US)

    companion object {
        /**
         * Parse a bulk paste / uploaded file body into domain names: one per line, but
         * also tolerant of commas, semicolons and spaces; blank lines and `#` comments
         * are dropped. Mirrors the panel's bulk-import parser.
         */
        fun parseDomains(raw: String): List<String> = raw
            .lineSequence()
            .map { it.substringBefore('#') }
            .flatMap { it.split(',', ';', ' ', '\t') }
            .map { it.trim().trimEnd('.').lowercase(Locale.US) }
            .filter { it.isNotEmpty() }
            .distinct()
            .toList()
    }
}
