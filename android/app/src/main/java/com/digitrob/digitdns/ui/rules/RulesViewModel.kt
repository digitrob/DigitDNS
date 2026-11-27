package com.digitrob.digitdns.ui.rules

import com.digitrob.digitdns.R
import androidx.annotation.StringRes
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.ResolutionProfileDto
import com.digitrob.digitdns.core.network.ResolutionSaveRequest
import com.digitrob.digitdns.core.network.ResolutionSettings
import com.digitrob.digitdns.data.EgressesRepository
import com.digitrob.digitdns.data.IconsRepository
import com.digitrob.digitdns.data.RulesRepository
import com.digitrob.digitdns.data.SettingsRepository
import com.digitrob.digitdns.data.UpstreamsRepository
import com.digitrob.digitdns.domain.model.RelayAgent
import com.digitrob.digitdns.domain.model.RuleLists
import com.digitrob.digitdns.domain.model.iconRepoUrl
import com.digitrob.digitdns.domain.model.normalizedDomain
import com.digitrob.digitdns.domain.model.replaceDomainPatterns
import com.digitrob.digitdns.domain.model.serviceIconQuery
import com.digitrob.digitdns.domain.model.serviceIconUrl
import com.digitrob.digitdns.domain.model.serviceIdFromName
import com.digitrob.digitdns.ui.components.RouteAction
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import java.time.Instant
import java.util.Locale
import javax.inject.Inject
import timber.log.Timber

/** Matches the panel's icon suggestion strip, so both clients offer the same choices. */
private const val ICON_SUGGESTION_LIMIT = 12

/** How the domains list is ordered. Mirrors the panel's own sort select. */
enum class DomainSort(@StringRes val labelRes: Int) {
    ADDED_DESC(R.string.domains_sort_added_desc),
    ADDED_ASC(R.string.domains_sort_added_asc),
    AZ(R.string.domains_sort_az),
    ZA(R.string.domains_sort_za),
    STATUS(R.string.domains_sort_status),
}

/** The status a domain row is in: an active proxy rule, a paused proxy rule, blocked, or direct. */
enum class DomainStatus(@StringRes val labelRes: Int) {
    PROXY(R.string.domains_status_proxy),
    BLOCKED(R.string.domains_status_blocked),
    PAUSED(R.string.domains_status_paused),
    DIRECT(R.string.domains_status_direct),
}

/**
 * The status chips shown above the list. A chip is a toggle: tapping the active chip
 * clears it, and with no chip selected the list shows every status. [DIRECT] surfaces
 * the direct-resolved set, which otherwise has no rows of its own.
 */
enum class DomainFilter(@StringRes val labelRes: Int) {
    PROXY(R.string.domains_status_proxy),
    BLOCKED(R.string.domains_status_blocked),
    PAUSED(R.string.domains_status_paused),
    DIRECT(R.string.domains_status_direct),
}

/** One row of the domains list: a proxied (active/paused), blocked, or direct domain. */
data class DomainRow(
    val domain: String,
    val status: DomainStatus,
    /** The wildcards that apply to this row, relative to it. */
    val wildcards: List<DomainWildcard> = emptyList(),
    /** The service label's id, or empty when the row has no service. */
    val serviceId: String = "",
    /** The geo relay this row exits through, or null for the local relay. */
    val relayAgent: RelayAgent? = null,
) {
    /** A proxied row's switch is on; paused is off. Other statuses carry no switch. */
    val enabled: Boolean get() = status == DomainStatus.PROXY
}

/**
 * One wildcard active for a row, named the way the operator sees it: `*.host` for
 * `*.domain` on the domain's own row, `*.parent` for the parent's wildcard. The
 * [pattern] is what is stored; [status] is the list it lives in.
 */
data class DomainWildcard(
    val type: String,
    val pattern: String,
    val status: DomainStatus,
)

/**
 * The Domains list split for the sinkhole entry.
 *
 * [sinkhole] is the blocked set the plain view collapses into one expandable row;
 * [flat] is everything else, in the order the active sort produced. A status chip
 * or a search empties [sinkhole], so the operator sees the exact rows they asked
 * for instead of a summary.
 */
data class DomainListProjection(
    val sinkhole: List<DomainRow>,
    val flat: List<DomainRow>,
)

/**
 * Splits [rows] into the collapsed sinkhole set and the flat rows. [group] is
 * false while a status chip or a search is active.
 */
fun projectDomainRows(rows: List<DomainRow>, group: Boolean): DomainListProjection {
    if (!group) return DomainListProjection(emptyList(), rows)
    val blocked = rows.filter { it.status == DomainStatus.BLOCKED }
    return if (blocked.isEmpty()) {
        DomainListProjection(emptyList(), rows)
    } else {
        DomainListProjection(blocked, rows.filter { it.status != DomainStatus.BLOCKED })
    }
}

/**
 * The rows the header "select all" tick owns. In the grouped view the Custom
 * Blocklist folds into its own entry with its own tick, so "all" means everything
 * except that group; a flat view (status chip or search) has no fold.
 */
internal fun selectableDomains(rows: List<DomainRow>, grouped: Boolean): Set<String> =
    rows.filter { !grouped || it.status != DomainStatus.BLOCKED }.map { it.domain }.toSet()

/** The rows the folded Custom Blocklist tick owns, empty when the view is flat. */
internal fun sinkholeDomains(rows: List<DomainRow>, grouped: Boolean): Set<String> =
    if (grouped) {
        rows.filter { it.status == DomainStatus.BLOCKED }.map { it.domain }.toSet()
    } else {
        emptySet()
    }

/** What "this row's rule" means: a wildcard row owns itself, a plain host its pair. */
private fun rowPatterns(domain: String): Set<String> {
    val wildcard = domain.startsWith("*.")
    val base = if (wildcard) domain.removePrefix("*.") else domain
    return if (wildcard) setOf(domain) else setOf(base, "*.$base")
}

/**
 * Whether [rule] already covers [name] under the server's matcher: a plain rule
 * covers itself and every subdomain, a wildcard covers strict subdomains only.
 * Keep this in step with internal/core/matcher and the panel's ruleCoversDomain.
 */
internal fun patternCovers(rule: String, name: String): Boolean {
    val pattern = rule.trim().trimEnd('.').lowercase(Locale.US)
    val domain = name.trim().trimEnd('.').lowercase(Locale.US)
    val wildcard = pattern.startsWith("*.")
    val base = if (wildcard) pattern.removePrefix("*.") else pattern
    return pattern.isNotEmpty() && domain.isNotEmpty() && base.isNotEmpty() &&
        (if (wildcard) {
            domain.endsWith(".$base")
        } else {
            domain == base || domain.endsWith(".$base")
        })
}

/**
 * The key a row's attributes are stored under: the exact target when the scope
 * writes one, else the wildcard itself. This is the string the service and relay
 * pickers use, and the one the server keeps only while it is still in a list.
 */
private fun attributeKey(targets: List<String>): String? =
    targets.firstOrNull { !it.startsWith("*.") } ?: targets.firstOrNull()

/**
 * The patterns a Remove click takes with it. A wildcard row is only itself; a
 * plain row also owns its host wildcard when both sit in the same list, which is
 * what the panel's domainRemovalPatterns does.
 */
internal fun removalPatterns(lists: RuleLists, domain: String): Set<String> {
    val key = domain.normalizedDomain()
    val twin = "*.$key"
    val shared = key.isNotEmpty() && !key.startsWith("*.") &&
        listOf(lists.proxied, lists.proxiedDisabled, lists.blocked, lists.direct)
            .any { key in it && twin in it }
    return when {
        key.isEmpty() -> emptySet()
        key.startsWith("*.") -> setOf(key)
        shared -> setOf(key, twin)
        else -> setOf(key)
    }
}

/**
 * Rebase an attribute map onto [next] when the row's pattern changed. An entry
 * already stored on [next] wins; every stale key is dropped either way, so the
 * server cannot prune a value the operator just moved.
 */
private fun Map<String, String>.rebased(olds: Collection<String>, next: String): Map<String, String> {
    val stale = olds.toSet()
    if (stale.isEmpty()) return this
    val moved = if (this[next].isNullOrBlank()) olds.firstNotNullOfOrNull { this[it] } else null
    val kept = this - stale
    return if (moved != null) kept + (next to moved) else kept
}

/** The list a name lives in, or null when it has no rule. */
private fun RuleLists.statusOf(domain: String): DomainStatus? = when {
    proxied.contains(domain) -> DomainStatus.PROXY
    proxiedDisabled.contains(domain) -> DomainStatus.PAUSED
    blocked.contains(domain) -> DomainStatus.BLOCKED
    direct.contains(domain) -> DomainStatus.DIRECT
    else -> null
}

/**
 * Move [domain]'s rule to [scope] inside [lists], keeping the list it already lives in
 * so its status is preserved. Returns null when the name has no rule (the scope is
 * only a modifier then) or the scope writes nothing.
 *
 * A base row owns its exact entry and its host wildcard; a wildcard row owns itself.
 * Both are replaced by the new scope's patterns.
 */
fun RuleLists.withScope(
    domain: String,
    scope: MatchScope,
    previous: List<String> = emptyList(),
): RuleLists? {
    // After *.parent the origin is no longer in any list; the pattern the previous
    // click wrote is where the rule lives, so its status is the fallback.
    val status = statusOf(domain)
        ?: previous.firstNotNullOfOrNull { statusOf(it) }
        ?: return null
    return scope.targets(domain).takeIf { it.isNotEmpty() }?.let { targets ->
        val wildcard = domain.startsWith("*.")
        val base = if (wildcard) domain.removePrefix("*.") else domain
        val rowPatterns = if (wildcard) setOf(domain) else setOf(base, "*.$base")
        val remove = rowPatterns + previous + targets
        fun strip(list: List<String>) = list.filter { it !in remove }
        // The label and relay belong to the pattern the row was displayed under.
        // Move them with the rule: the server prunes any entry whose key is no
        // longer in a list, which is how a scope change used to lose the service.
        val nextKey = attributeKey(targets)
        val oldKeys = if (nextKey == null) {
            emptyList()
        } else {
            (listOf(domain) + previous).map { it.normalizedDomain() }.filter { it != nextKey }
        }
        // The target list keeps the row's slot; the others only lose the patterns.
        copy(
            proxied = if (status == DomainStatus.PROXY) {
                replaceDomainPatterns(proxied, remove, targets)
            } else {
                strip(proxied)
            },
            proxiedDisabled = if (status == DomainStatus.PAUSED) {
                replaceDomainPatterns(proxiedDisabled, remove, targets)
            } else {
                strip(proxiedDisabled)
            },
            blocked = if (status == DomainStatus.BLOCKED) {
                replaceDomainPatterns(blocked, remove, targets)
            } else {
                strip(blocked)
            },
            direct = if (status == DomainStatus.DIRECT) {
                replaceDomainPatterns(direct, remove, targets)
            } else {
                strip(direct)
            },
            domainServices = domainServices.rebased(oldKeys, nextKey.orEmpty()),
            domainAgents = domainAgents.rebased(oldKeys, nextKey.orEmpty()),
            // Keep the Added position across a rebase, like the panel does.
            domainAdded = domainAdded.rebased(oldKeys, nextKey.orEmpty()),
        )
    }
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
    /** The service the list is narrowed to: null = every service, [NO_SERVICE] = unlabelled. */
    val serviceFilter: String? = null,
    /** Domains ticked for a bulk action. Empty means no selection. */
    val selectedDomains: Set<String> = emptySet(),
    val isLoading: Boolean = false,
    /** True only for a pull-to-refresh gesture, so the header Reload button never spins the indicator. */
    val isRefreshing: Boolean = false,
    val isSaving: Boolean = false,
    val error: String? = null,
    val savedMessage: String? = null,
    /** Global resolution defaults plus the per-domain overrides. */
    val resolution: ResolutionSettings? = null,
    /** Live pool addresses, used when the settings snapshot's list is empty. */
    val resolutionUpstreams: List<String> = emptyList(),
    /** Non-null when the settings snapshot could not be loaded; saving is disabled. */
    val resolutionError: String? = null,
    /** The install's own ISO country, used as the flag on "This Server". */
    val serverCountry: String = "",
) {
    /** The exact `domain_agents` value stored for [domain], or empty when it inherits. */
    fun storedRelayId(domain: String): String =
        lists.domainAgents[domain.normalizedDomain()].orEmpty()

    /** The relay a service default would apply to [domain], or null for This Server. */
    fun serviceRelayAgent(domain: String): RelayAgent? {
        val id = lists.serviceAgents[lists.serviceOf(domain)].orEmpty()
        if (id.isBlank()) return null
        return lists.relayById(id)?.copy(id = id)
    }

    /** Every agent as a picker row, ordered by display name. */
    fun relayAgents(): List<RelayAgent> =
        lists.relayAgents.map { (id, meta) -> meta.copy(id = id) }
            .sortedBy { it.name.ifBlank { it.id }.lowercase(Locale.US) }

    /** Hub-side SOCKS5 and WireGuard choices, grouped by kind. */
    fun relayEgresses(): List<Pair<String, RelayAgent>> =
        lists.egresses.map { (id, meta) -> id to meta.copy(id = id) }
            .sortedWith(compareBy({ (_, a) -> a.kind }, { (_, a) -> a.name.lowercase(Locale.US) }))

    /** Every editable domain, tagged with its status. */
    private val allRows: List<DomainRow>
        get() {
            // Effective status per stored pattern, low precedence first so a name in
            // two lists reads the way the row projection above does.
            val statuses = LinkedHashMap<String, DomainStatus>()
            fun put(list: List<String>, status: DomainStatus) {
                list.distinct().forEach { statuses[it] = status }
            }
            put(lists.proxiedDisabled, DomainStatus.PAUSED)
            put(lists.proxied, DomainStatus.PROXY)
            put(lists.blocked, DomainStatus.BLOCKED)
            put(lists.direct, DomainStatus.DIRECT)

            val rows = buildList {
                // distinct(): the saved list on a live install can carry the same domain
                // twice (the panel accepts a paste that repeats an entry), and two rows
                // with the same LazyColumn key crash the Domains tab on open. One row per
                // domain is also what the operator means.
                lists.proxied.distinct().forEach { add(DomainRow(it, DomainStatus.PROXY)) }
                lists.proxiedDisabled.distinct().forEach { add(DomainRow(it, DomainStatus.PAUSED)) }
                lists.blocked.distinct().forEach { add(DomainRow(it, DomainStatus.BLOCKED)) }
                lists.direct.distinct().forEach { add(DomainRow(it, DomainStatus.DIRECT)) }
            }
            return rows
                // *.d is redundant when d is in the same list: the plain rule already
                // covers every subdomain. Hide the duplicate row; a wildcard in a
                // different list still changes what subdomains do.
                .filter { row ->
                    !row.domain.startsWith("*.") ||
                        statuses[row.domain.removePrefix("*.")] != row.status
                }
                .map { row ->
                    row.copy(
                        wildcards = wildcardTypesFor(row.domain, statuses),
                        serviceId = lists.serviceOf(row.domain),
                        relayAgent = lists.relayFor(row.domain),
                    )
                }
        }

    /** The wildcards that apply to [domain], relative to that row. */
    private fun wildcardTypesFor(
        domain: String,
        statuses: Map<String, DomainStatus>,
    ): List<DomainWildcard> {
        val wildcard = domain.startsWith("*.")
        val base = if (wildcard) domain.removePrefix("*.") else domain
        val labels = base.split('.').filter { it.isNotEmpty() }
        val out = mutableListOf<DomainWildcard>()
        if (labels.size >= WILDCARD_MIN_LABELS) {
            val host = "*.$base"
            val hostStatus = statuses[host]
            // A plain rule covers every subdomain already; only show *.host when it
            // lives in a different list (and therefore changes the answer).
            val redundant = !wildcard && statuses[base] != null && hostStatus == statuses[base]
            if (hostStatus != null && !redundant) {
                out += DomainWildcard("*.host", host, hostStatus)
            }
        }
        if (labels.size >= PARENT_WILDCARD_MIN_LABELS) {
            val parent = "*." + labels.drop(1).joinToString(".")
            statuses[parent]?.let { out += DomainWildcard("*.parent", parent, it) }
        }
        return out
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
            // The service filter is a view over the same rows: switching services never
            // refetches and never changes a status. NO_SERVICE selects the unlabelled.
            val byService = when (serviceFilter) {
                null -> filtered
                NO_SERVICE -> filtered.filter { it.serviceId.isEmpty() }
                else -> filtered.filter { it.serviceId == serviceFilter }
            }
            val searched = byService.filter {
                search.isBlank() || it.domain.contains(search.trim(), ignoreCase = true)
            }
            // "Added" is a property of the rule, not of the list it happens to
            // sit in. Reversing the concatenated proxied/paused/blocked/direct
            // rows only reversed the status groups, so with no chip selected the
            // newest proxy was at the bottom. Sort by the server's stamps when
            // they exist, and keep the old reverse as the legacy fallback.
            val added = lists.domainAdded
            fun stamp(domain: String): Long {
                val base = domain.removePrefix("*.")
                val raw = added[domain] ?: added[base]
                return raw?.let { runCatching { Instant.parse(it).toEpochMilli() }.getOrNull() } ?: 0L
            }
            val anyStamp = searched.any { stamp(it.domain) > 0 }
            return when (sort) {
                DomainSort.ADDED_DESC ->
                    if (anyStamp) searched.sortedByDescending { stamp(it.domain) } else searched.reversed()
                DomainSort.ADDED_ASC ->
                    if (anyStamp) searched.sortedBy { stamp(it.domain) } else searched
                // A wildcard row keeps its base's alphabetical slot, so a scope
                // change does not move it under A-Z.
                DomainSort.AZ -> searched.sortedWith(
                    compareBy({ it.domain.removePrefix("*.").lowercase(Locale.US) }, { it.domain }),
                )
                DomainSort.ZA -> searched.sortedWith(
                    compareByDescending<DomainRow> { it.domain.removePrefix("*.").lowercase(Locale.US) }
                        .thenByDescending { it.domain },
                )
                DomainSort.STATUS -> searched.sortedBy { it.status.ordinal }
            }
        }

    val countLabel: String
        get() = "${rows.size} domains"

    /** How many rows carry each service label, over the whole list (not the filter). */
    val serviceCounts: Map<String, Int>
        get() {
            val counts = HashMap<String, Int>()
            for (row in allRows) {
                if (row.serviceId.isNotEmpty()) counts[row.serviceId] = (counts[row.serviceId] ?: 0) + 1
            }
            return counts
        }

    /** Unlabelled rows, the "No service" virtual filter. */
    val noServiceCount: Int get() = allRows.count { it.serviceId.isEmpty() }

    val totalCount: Int get() = allRows.size

    /** A service's display name, falling back to its id if the two ever disagree. */
    fun serviceName(id: String): String = lists.services[id] ?: id

    /** Drop ticks that the current search/filter/service view no longer shows. */
    fun withVisibleSelection(): RulesUiState {
        if (selectedDomains.isEmpty()) return this
        val visible = rows.map { row -> row.domain }.toSet()
        return copy(selectedDomains = selectedDomains intersect visible)
    }
}

/**
 * Edits the proxied and blocked domain lists (and the paused subset of the proxied
 * list) plus the pinned-record map, and performs the routing quick actions that move a
 * domain between the four custom lists.
 */
@HiltViewModel
@Suppress("TooManyFunctions")
class RulesViewModel @Inject constructor(
    private val repository: RulesRepository,
    private val settingsRepository: SettingsRepository,
    private val upstreamsRepository: UpstreamsRepository,
    private val egressesRepository: EgressesRepository,
    private val iconsRepository: IconsRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(RulesUiState())
    val state: StateFlow<RulesUiState> = _state.asStateFlow()

    init {
        load()
        loadResolution()
        loadServerCountry()
        loadEgresses()
    }

    /** Egress labels for the route picker and the row badge. */
    private fun loadEgresses() {
        viewModelScope.launch {
            val list = runCatching { egressesRepository.list() }.getOrNull() ?: return@launch
            val metadata = list.associate { egress ->
                egress.value to RelayAgent(
                    id = egress.value,
                    name = egress.name,
                    country = egress.country,
                    kind = egress.kind,
                )
            }
            _state.update { it.copy(lists = it.lists.copy(egresses = metadata)) }
        }
    }

    /** The local flag on relay choices comes from the settings view, not the rules body. */
    private fun loadServerCountry() {
        viewModelScope.launch {
            val country = runCatching { settingsRepository.load().server?.country }.getOrNull().orEmpty()
            if (country.isNotBlank()) _state.update { it.copy(serverCountry = country) }
        }
    }

    private var resolutionLoading = false

    /**
     * Fetch the resolution defaults and per-domain rules. Called at init, when the
     * gear dialog opens and on every list reload, because a single failed request
     * used to leave the upstream picker with only "Inherit Global" until the app
     * was restarted. A live-pool fallback keeps the picker usable even then, and
     * the error flag disables saving so a partial snapshot cannot wipe the rules.
     */
    fun loadResolution() {
        if (resolutionLoading) return
        resolutionLoading = true
        viewModelScope.launch {
            try {
                val result = runCatching { settingsRepository.loadResolution() }
                val loaded = result.getOrNull()
                val failure = result.exceptionOrNull()
                if (failure != null) {
                    // The dialog used to show only "Inherit Global" with no clue
                    // why; the log carries the HTTP status/message for diagnosis.
                    Timber.w(failure, "resolution settings load failed")
                }
                // Merge the snapshot with the live pool: a settings list can
                // predate a resolver the pool already has, and the dialog must
                // offer both.
                val pool = runCatching { upstreamsRepository.list() }.getOrNull().orEmpty()
                val upstreams = (loaded?.upstreams.orEmpty() + pool).distinct()
                val error = when {
                    loaded == null -> (failure as? ApiException)?.userMessage
                        ?: failure?.message
                        ?: "Could not load resolution settings"
                    upstreams.isEmpty() -> "No resolvers reported by the server"
                    else -> null
                }
                _state.update {
                    it.copy(
                        resolution = loaded ?: it.resolution,
                        resolutionUpstreams = upstreams.ifEmpty { it.resolutionUpstreams },
                        resolutionError = error,
                    )
                }
            } finally {
                resolutionLoading = false
            }
        }
    }

    /**
     * Save one domain's override (null removes it) and keep the global defaults as they
     * are. The server refuses rules for domains that are not in the proxied list, so
     * this is only offered on those rows.
     */
    fun saveResolution(domain: String, profile: ResolutionProfileDto?) {
        val current = _state.value.resolution ?: run {
            _state.update { it.copy(error = "Resolution settings are still loading") }
            loadResolution()
            return
        }
        val profiles = current.profiles.toMutableMap()
        if (profile == null) profiles.remove(domain) else profiles[domain] = profile
        viewModelScope.launch {
            runCatching {
                settingsRepository.saveResolution(
                    ResolutionSaveRequest(
                        transport = current.transport,
                        transports = current.transports,
                        defaultUpstream = current.defaultUpstream,
                        defaultStrategy = current.defaultStrategy,
                        adaptiveIntervalMinutes = current.adaptiveIntervalMinutes
                            .takeIf { current.defaultStrategy == "adaptive" },
                        profiles = profiles,
                    ),
                )
            }.onSuccess { saved ->
                _state.update { it.copy(resolution = saved, savedMessage = "Resolution rule saved") }
            }.onFailure { error ->
                _state.update { it.copy(error = error.message ?: "Could not save the resolution rule") }
            }
        }
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
                _state.update {
                    it.copy(
                        lists = lists,
                        isLoading = false,
                        isRefreshing = false,
                        selectedDomains = it.selectedDomains.intersect(lists.managedPatterns),
                    )
                }
                // The override dialog's upstream picker is only as fresh as this
                // snapshot; a resolver added on the dashboard enables new choices.
                loadResolution()
            } catch (e: ApiException) {
                _state.update { it.copy(isLoading = false, isRefreshing = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update {
                    it.copy(isLoading = false, isRefreshing = false, error = e.message ?: "Could not load rules.")
                }
            }
        }
    }

    fun setSearch(value: String) = _state.update { it.copy(search = value).withVisibleSelection() }
    fun setSort(value: DomainSort) = _state.update { it.copy(sort = value) }

    /** Toggle a status chip: tapping the active chip clears it (show all statuses). */
    fun setFilter(value: DomainFilter) = _state.update {
        it.copy(filter = if (it.filter == value) null else value).withVisibleSelection()
    }

    /** Narrow the list to one service. Null shows every service, [NO_SERVICE] the unlabelled. */
    fun setServiceFilter(value: String?) = _state.update {
        it.copy(serviceFilter = value).withVisibleSelection()
    }

    /**
     * Put [domain] in [id], or clear its label when [id] is blank. This writes only
     * `domain_services`: routing is decided by the four lists and is never touched here.
     */
    fun assignService(domain: String, id: String) {
        viewModelScope.launch {
            val lists = _state.value.lists
            val links = lists.domainServices.toMutableMap()
            val key = domain.normalizedDomain()
            if (id.isEmpty()) links.remove(key) else links[key] = id
            val message = if (id.isEmpty()) {
                "Service label cleared for $domain."
            } else {
                "${lists.services[id] ?: id}: $domain."
            }
            write(lists.copy(domainServices = links), message)
        }
    }

    /**
     * Icon choices for the service form. A domain is reduced to its registrable
     * name first, and an already-chosen image URL produces no query at all. The
     * returned strings are ready to store in `service_icons`, so both clients
     * render the same URL for a picked suggestion.
     */
    suspend fun searchIcons(raw: String): List<String> {
        val query = serviceIconQuery(raw)
        if (query.length < 2) return emptyList()
        return iconsRepository.search(query)
            .map(::iconRepoUrl)
            .filter { it.isNotEmpty() }
            .take(ICON_SUGGESTION_LIMIT)
    }

    /**
     * Create a service when [id] is null, otherwise rename (and re-icon) that one.
     * A rename keeps the id, so every domain already labelled stays labelled.
     */
    fun saveService(id: String?, name: String, icon: String, relayId: String = "") {
        val label = name.trim()
        if (label.isEmpty()) return
        viewModelScope.launch {
            val lists = _state.value.lists
            val services = lists.services.toMutableMap()
            val icons = lists.serviceIcons.toMutableMap()
            val serviceAgents = lists.serviceAgents.toMutableMap()
            val serviceId = id ?: uniqueServiceId(serviceIdFromName(label), services)
            services[serviceId] = label
            val url = serviceIconUrl(icon)
            if (url.isEmpty()) icons.remove(serviceId) else icons[serviceId] = url
            // This Server is the absence of a default, matching the panel. An agent
            // default only applies when the domain has no relay override.
            if (relayId.isBlank() || relayId == "__local__") {
                serviceAgents.remove(serviceId)
            } else {
                serviceAgents[serviceId] = relayId
            }
            write(
                lists.copy(services = services, serviceIcons = icons, serviceAgents = serviceAgents),
                if (id == null) "Added $label." else "Renamed to $label.",
            )
        }
    }

    /**
     * Delete a service and every label pointing at it. Domains keep their routing:
     * a deleted label is the absence of organisation, not a pause or a block.
     */
    fun deleteService(id: String) {
        if (id.isEmpty()) return
        viewModelScope.launch {
            val lists = _state.value.lists
            val name = lists.services[id] ?: id
            write(
                lists.copy(
                    services = lists.services - id,
                    serviceIcons = lists.serviceIcons - id,
                    domainServices = lists.domainServices.filterValues { it != id },
                    serviceAgents = lists.serviceAgents - id,
                ),
                "Deleted $name. Domains keep their routing.",
            )
            if (_state.value.serviceFilter == id) _state.update { it.copy(serviceFilter = null) }
        }
    }

    /** Avoid overwriting an existing service when a new name slugs to a taken id. */
    private fun uniqueServiceId(base: String, existing: Map<String, String>): String {
        if (base !in existing) return base
        var n = 2
        while ("$base-$n" in existing) n++
        return "$base-$n"
    }

    fun dismissMessage() = _state.update { it.copy(savedMessage = null) }

    /** Tick or untick one row. Long-press and, while selecting, a row tap call this. */
    fun toggleSelection(domain: String) = _state.update {
        val next = if (domain in it.selectedDomains) it.selectedDomains - domain else it.selectedDomains + domain
        it.copy(selectedDomains = next)
    }

    /**
     * The select-all checkbox: tick every selectable row, or clear them when they
     * are all ticked. Hidden rows are left alone, and in the grouped view the
     * Custom Blocklist folds into its own entry with its own tick, so "all" here
     * means everything except that group.
     */
    fun toggleSelectAllVisible() = _state.update {
        val grouped = it.filter == null && it.search.isBlank()
        val selectable = selectableDomains(it.rows, grouped)
        val allSelected = selectable.isNotEmpty() && selectable.all { domain -> domain in it.selectedDomains }
        it.copy(
            selectedDomains = if (allSelected) {
                it.selectedDomains - selectable
            } else {
                it.selectedDomains + selectable
            },
        )
    }

    /**
     * The sinkhole entry's own checkbox: tick or clear only the folded Custom
     * Blocklist members, so a label or relay can be applied to that set alone.
     * The whole group moves, not just the rows the list has rendered.
     */
    fun toggleSinkholeSelection() = _state.update {
        val grouped = it.filter == null && it.search.isBlank()
        val members = sinkholeDomains(it.rows, grouped)
        if (members.isEmpty()) return@update it
        val allSelected = members.all { domain -> domain in it.selectedDomains }
        it.copy(
            selectedDomains = if (allSelected) it.selectedDomains - members else it.selectedDomains + members,
        )
    }

    fun clearSelection() = _state.update { it.copy(selectedDomains = emptySet()) }

    /**
     * Label every selected domain in one save. An empty [id] clears the label;
     * routing is never touched. The selection is cleared afterwards.
     */
    fun assignServiceToMany(domains: Set<String>, id: String) {
        if (domains.isEmpty()) return
        viewModelScope.launch {
            val lists = _state.value.lists
            val links = lists.domainServices.toMutableMap()
            for (domain in domains) {
                val key = domain.normalizedDomain()
                if (id.isEmpty()) links.remove(key) else links[key] = id
            }
            val name = if (id.isEmpty()) "No service" else lists.services[id] ?: id
            write(
                lists.copy(domainServices = links),
                "Assigned ${domains.size} domain(s) to $name.",
            )
            _state.update { it.copy(selectedDomains = emptySet()) }
        }
    }

    /**
     * Set one domain's relay: `__inherit__` removes the override so the service
     * default applies, `__local__` writes an explicit This Server choice, and any
     * other value is an agent id. The second line's chip is the only control, so
     * the row height and the right-side actions never move.
     */
    fun assignRelay(domain: String, id: String) {
        viewModelScope.launch {
            val lists = _state.value.lists
            val links = lists.domainAgents.toMutableMap()
            val key = domain.normalizedDomain()
            if (id.isEmpty() || id == "__inherit__") links.remove(key) else links[key] = id
            val message = when (id) {
                "", "__inherit__" -> "Relay now follows the service default for $domain."
                "__local__" -> "Relay set to This Server for $domain."
                else -> {
                    val name = lists.relayAgents[id]?.name?.ifBlank { id } ?: id
                    "Relayed $domain through $name."
                }
            }
            write(lists.copy(domainAgents = links), message)
        }
    }

    /** The bulk Egress action: the same map write for every selected domain. */
    fun assignRelayToMany(domains: Set<String>, id: String) {
        if (domains.isEmpty()) return
        viewModelScope.launch {
            val lists = _state.value.lists
            val links = lists.domainAgents.toMutableMap()
            for (domain in domains) {
                val key = domain.normalizedDomain()
                if (id.isEmpty() || id == "__inherit__") links.remove(key) else links[key] = id
            }
            val label = when (id) {
                "", "__inherit__" -> "the service default"
                "__local__" -> "This Server"
                else -> lists.relayAgents[id]?.name?.ifBlank { id } ?: id
            }
            write(
                lists.copy(domainAgents = links),
                "Set ${domains.size} domain(s) to relay through $label.",
            )
            _state.update { it.copy(selectedDomains = emptySet()) }
        }
    }

    /** Add a proxied domain (bulk paste: one or more names, any separators). */
    fun addDomain(raw: String) {
        val parsed = parseDomains(raw)
        if (parsed.isEmpty()) return
        viewModelScope.launch {
            val lists = _state.value.lists
            // A plain rule covers its whole subtree, so a name under it would be a
            // duplicate that can never win. Skip those instead of appending them;
            // the panel's Add box offers "Add anyway" for the exact-label case, and
            // the Android route menu can rebase the existing rule to a wildcard.
            val known = (lists.proxied + lists.proxiedDisabled).distinct()
            val fresh = parsed.filter { name -> name !in known && known.none { patternCovers(it, name) } }
            val skipped = parsed.size - fresh.size
            if (fresh.isEmpty()) {
                _state.update { it.copy(savedMessage = "Already covered by an existing rule.") }
                return@launch
            }
            val merged = (lists.proxied + fresh).distinct()
            // Stamp the new names locally too: the server assigns the same kind
            // of timestamp on save, and without it a just-added row sorted to
            // the bottom under "Added (newest)" until the next reload.
            val message = buildString {
                append("Added ${fresh.size} domain(s).")
                if (skipped > 0) append(" Skipped $skipped already covered.")
            }
            write(lists.copy(proxied = merged).withAddedStamps(fresh), message)
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
            val patterns = removalPatterns(lists, domain)
            if (patterns.isEmpty()) return@launch
            val keep: (String) -> Boolean = { it !in patterns }
            val next = lists.copy(
                proxied = lists.proxied.filter(keep),
                proxiedDisabled = lists.proxiedDisabled.filter(keep),
                blocked = lists.blocked.filter(keep),
                direct = lists.direct.filter(keep),
                // The server prunes a label whose domain is gone; doing it here keeps
                // the screen from showing a label the next save would drop anyway.
                domainServices = lists.domainServices.filterKeys(keep),
                domainAgents = lists.domainAgents.filterKeys(keep),
                // Drop the Added stamp with the rule, so re-adding the name later
                // puts it at the top instead of in its old position.
                domainAdded = lists.domainAdded.filterKeys(keep),
            )
            if (write(next, "Removed $domain.")) dropResolutionProfiles(patterns)
        }
    }

    /**
     * Apply a routing quick action to [domain], where [domain] is the already-resolved
     * match target (e.g. `*.target.com`). Every action starts from a clean slate — a
     * domain lives in exactly one list — so switching it from blocked to proxied is a
     * move, not an addition to both, exactly as the web panel does.
     */
    fun routeAction(action: RouteAction, domains: List<String>, origin: String? = null) {
        if (domains.isEmpty()) return
        val unique = domains.distinct()
        viewModelScope.launch {
            val lists = _state.value.lists
            // The row's old patterns are replaced in place, so an action taken from
            // the Domains list does not move it to the end either.
            val old = origin?.let { rowPatterns(it) } ?: unique.toSet()
            val keep = { d: String -> d !in old && d !in unique }
            fun strip(list: List<String>) = list.filter(keep)
            val removedKeys = (unique.map(String::normalizedDomain) + listOfNotNull(origin?.normalizedDomain())).toSet()
            val next = when (action) {
                RouteAction.Proxy -> lists.copy(
                    proxied = replaceDomainPatterns(lists.proxied, old, unique),
                    proxiedDisabled = strip(lists.proxiedDisabled),
                    blocked = strip(lists.blocked),
                    direct = strip(lists.direct),
                )
                RouteAction.Block -> lists.copy(
                    proxied = strip(lists.proxied),
                    proxiedDisabled = strip(lists.proxiedDisabled),
                    blocked = replaceDomainPatterns(lists.blocked, old, unique),
                    direct = strip(lists.direct),
                )
                RouteAction.Direct -> lists.copy(
                    proxied = strip(lists.proxied),
                    proxiedDisabled = strip(lists.proxiedDisabled),
                    blocked = strip(lists.blocked),
                    direct = replaceDomainPatterns(lists.direct, old, unique),
                )
                RouteAction.Remove -> lists.copy(
                    proxied = strip(lists.proxied),
                    proxiedDisabled = strip(lists.proxiedDisabled),
                    blocked = strip(lists.blocked),
                    direct = strip(lists.direct),
                    // Removing the rule also drops the label and relay: they would
                    // otherwise be pruned by the server on the next save and blink
                    // out later.
                    domainServices = lists.domainServices.filterKeys { it !in removedKeys },
                    domainAgents = lists.domainAgents.filterKeys { it !in removedKeys },
                )
            }
            val verb = when (action) {
                RouteAction.Proxy -> "Proxying"
                RouteAction.Block -> "Blocking"
                RouteAction.Direct -> "Routing direct"
                RouteAction.Remove -> "Removed"
            }
            // A move or scope change keeps the rule's original Added position;
            // a brand-new rule is stamped now. A removal must not re-add stamps
            // for the names it just deleted.
            write(
                next.stampRouteAction(action, unique, addedStampFor(lists, origin))
                    .rebaseRowAttributes(origin, unique, action),
                "$verb ${unique.joinToString(" + ")}.",
            )
        }
    }

    /**
     * Apply a scope change to a name that already has a rule. The line under the
     * domain is drawn from the saved lists, so this is what makes selecting *.host
     * visible immediately instead of only arming the next action.
     */
    fun changeScope(domain: String, scope: MatchScope, previous: List<String> = emptyList()) {
        val next = _state.value.lists.withScope(domain, scope, previous) ?: return
        val targets = scope.targets(domain)
        val nextKey = attributeKey(targets)?.normalizedDomain()
        val oldKeys = if (nextKey.isNullOrBlank()) {
            emptyList()
        } else {
            (listOf(domain) + previous).map { it.normalizedDomain() }.filter { it.isNotBlank() && it != nextKey }
        }
        viewModelScope.launch {
            if (!write(next, "Scope: ${targets.joinToString(" + ")}.")) return@launch
            moveResolutionProfile(oldKeys, nextKey)
        }
    }

    /**
     * Carry a domain's resolution override to the new pattern after a scope change.
     * The rules save has already pruned the old profile server-side, so this posts
     * the same map with exactly the moved entry. Runs only after a successful save.
     */
    @Suppress("ReturnCount")
    private suspend fun moveResolutionProfile(oldKeys: List<String>, nextKey: String?) {
        val current = _state.value.resolution ?: return
        val key = nextKey?.normalizedDomain().orEmpty()
        if (key.isBlank() || oldKeys.isEmpty()) return
        if (current.profiles.containsKey(key)) return
        val normalizedOlds = oldKeys.map { it.normalizedDomain() }.filter { it.isNotBlank() && it != key }
        val profile = normalizedOlds.firstNotNullOfOrNull { current.profiles[it] } ?: return
        val profiles = current.profiles.toMutableMap()
        normalizedOlds.forEach { profiles.remove(it) }
        profiles[key] = profile
        saveResolutionProfiles(current, profiles, "Resolution rule moved with the domain.")
    }

    /** Drop profiles for keys whose rule was removed, instead of waiting for the next load. */
    private suspend fun dropResolutionProfiles(keys: Collection<String>) {
        val current = _state.value.resolution ?: return
        if (keys.none { current.profiles.containsKey(it) }) return
        val profiles = current.profiles.toMutableMap() - keys.toSet()
        saveResolutionProfiles(current, profiles, null)
    }

    private suspend fun saveResolutionProfiles(
        current: ResolutionSettings,
        profiles: Map<String, ResolutionProfileDto>,
        message: String?,
    ) {
        runCatching {
            settingsRepository.saveResolution(
                ResolutionSaveRequest(
                    transport = current.transport,
                    transports = current.transports,
                    defaultUpstream = current.defaultUpstream,
                    defaultStrategy = current.defaultStrategy,
                    adaptiveIntervalMinutes = current.adaptiveIntervalMinutes
                        .takeIf { current.defaultStrategy == "adaptive" },
                    profiles = profiles,
                ),
            )
        }.onSuccess { saved ->
            _state.update { it.copy(resolution = saved, savedMessage = message ?: it.savedMessage) }
        }.onFailure { error ->
            Timber.w(error, "Could not save the moved resolution rule")
            _state.update { it.copy(error = error.message ?: "Could not move the resolution rule") }
        }
    }

    /** Replace the DNS rewrite map, e.g. from an add, remove or bulk import. */
    fun saveRecords(records: Map<String, String>, message: String = "DNS rewrites saved.") {
        viewModelScope.launch {
            write(_state.value.lists.copy(records = records), message)
        }
    }

    /** Save a hostname to address rewrite. Both sides must be non-blank. */
    fun addRecord(domain: String, ip: String) {
        val host = normalize(domain)
        val address = ip.trim()
        if (host.isEmpty() || address.isEmpty()) return
        val next = _state.value.lists.records + (host to address)
        viewModelScope.launch { write(_state.value.lists.copy(records = next), "Saved $host → $address.") }
    }

    /** Remove a DNS rewrite. */
    fun removeRecord(domain: String) {
        val next = _state.value.lists.records - domain
        viewModelScope.launch { write(_state.value.lists.copy(records = next), "Removed $domain.") }
    }

    private suspend fun write(next: RuleLists, message: String? = null): Boolean {
        _state.update {
            it.copy(
                lists = next,
                isSaving = true,
                error = null,
                // A removal can take a ticked row away; keep the selection honest
                // before the save returns.
                selectedDomains = it.selectedDomains intersect next.managedPatterns,
            )
        }
        return try {
            val ok = repository.save(next)
            _state.update {
                it.copy(isSaving = false, savedMessage = if (ok) message else null, error = if (ok) null else "Save failed.")
            }
            ok
        } catch (e: ApiException) {
            _state.update { it.copy(isSaving = false, error = e.userMessage) }
            false
        } catch (e: IOException) {
            _state.update { it.copy(isSaving = false, error = e.message ?: "Could not save rules.") }
            false
        }
    }

    /**
     * Give any pattern without a stored Added time one, so a newly written rule
     * lands at the top of "Added (newest)" before the server's next read. A moved
     * or scope-renamed rule passes its old stamp in [inherited] and keeps its slot.
     */
    /** The Added stamp already stored for [origin], if any, so a move keeps its slot. */
    private fun addedStampFor(lists: RuleLists, origin: String?): String? {
        if (origin == null) return null
        val base = origin.removePrefix("*.")
        return lists.domainAdded[origin] ?: lists.domainAdded[base] ?: lists.domainAdded["*.$base"]
    }

    /** A removal drops the stamps; every other action stamps a brand-new rule. */
    /**
     * The action can also change the pattern (host and parent scopes). The label
     * and relay follow the row so the new wildcard is not bare. Remove already
     * drops both maps above, so only a live move needs the rebase.
     */
    private fun RuleLists.rebaseRowAttributes(
        origin: String?,
        unique: List<String>,
        action: RouteAction,
    ): RuleLists {
        val nextKey = if (action == RouteAction.Remove) {
            ""
        } else {
            attributeKey(unique)?.normalizedDomain().orEmpty()
        }
        var oldKeys: List<String> = emptyList()
        if (nextKey.isNotBlank()) {
            oldKeys = buildSet {
                if (!origin.isNullOrBlank()) add(origin.normalizedDomain())
            }.filter { it.isNotBlank() && it != nextKey }
        }
        return if (oldKeys.isEmpty()) {
            this
        } else {
            copy(
                domainServices = domainServices.rebased(oldKeys, nextKey),
                domainAgents = domainAgents.rebased(oldKeys, nextKey),
                domainAdded = domainAdded.rebased(oldKeys, nextKey),
            )
        }
    }

    private fun RuleLists.stampRouteAction(
        action: RouteAction,
        domains: List<String>,
        inherited: String?,
    ): RuleLists = if (action == RouteAction.Remove) {
        copy(domainAdded = domainAdded - domains.toSet())
    } else {
        withAddedStamps(domains, inherited)
    }

    private fun RuleLists.withAddedStamps(
        domains: Collection<String>,
        inherited: String? = null,
    ): RuleLists {
        if (domains.isEmpty()) return this
        val now = Instant.now().toString()
        val stamps = domainAdded.toMutableMap()
        var changed = false
        for (domain in domains) {
            if (stamps[domain].isNullOrBlank()) {
                stamps[domain] = inherited ?: now
                changed = true
            }
        }
        return if (changed) copy(domainAdded = stamps) else this
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

/** A wildcard needs host.tld at least. */
private const val WILDCARD_MIN_LABELS = 2

/** A parent wildcard needs a real parent above host.tld. */
private const val PARENT_WILDCARD_MIN_LABELS = 3

/** The service filter's virtual "unlabelled" entry; never a stored service id. */
const val NO_SERVICE = "__none__"
