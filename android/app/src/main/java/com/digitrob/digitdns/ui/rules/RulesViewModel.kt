package com.digitrob.digitdns.ui.rules

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.ResolutionProfileDto
import com.digitrob.digitdns.core.network.ResolutionSaveRequest
import com.digitrob.digitdns.core.network.ResolutionSettings
import com.digitrob.digitdns.data.RulesRepository
import com.digitrob.digitdns.data.SettingsRepository
import com.digitrob.digitdns.data.UpstreamsRepository
import com.digitrob.digitdns.domain.model.RuleLists
import com.digitrob.digitdns.domain.model.replaceDomainPatterns
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
import timber.log.Timber

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
    /** The wildcards that apply to this row, relative to it. */
    val wildcards: List<DomainWildcard> = emptyList(),
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

/** What "this row's rule" means: a wildcard row owns itself, a plain host its pair. */
private fun rowPatterns(domain: String): Set<String> {
    val wildcard = domain.startsWith("*.")
    val base = if (wildcard) domain.removePrefix("*.") else domain
    return if (wildcard) setOf(domain) else setOf(base, "*.$base")
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
) {
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
                .map { row -> row.copy(wildcards = wildcardTypesFor(row.domain, statuses)) }
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
            val searched = filtered.filter {
                search.isBlank() || it.domain.contains(search.trim(), ignoreCase = true)
            }
            return when (sort) {
                DomainSort.ADDED_DESC -> searched.reversed()
                DomainSort.ADDED_ASC -> searched
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
    private val settingsRepository: SettingsRepository,
    private val upstreamsRepository: UpstreamsRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(RulesUiState())
    val state: StateFlow<RulesUiState> = _state.asStateFlow()

    init {
        load()
        loadResolution()
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
                        defaultUpstream = current.defaultUpstream,
                        defaultStrategy = current.defaultStrategy,
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
                _state.update { it.copy(lists = lists, isLoading = false, isRefreshing = false) }
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
                )
            }
            val verb = when (action) {
                RouteAction.Proxy -> "Proxying"
                RouteAction.Block -> "Blocking"
                RouteAction.Direct -> "Routing direct"
                RouteAction.Remove -> "Removed"
            }
            write(next, "$verb ${unique.joinToString(" + ")}.")
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
        viewModelScope.launch {
            write(next, "Scope: ${targets.joinToString(" + ")}.")
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

/** A wildcard needs host.tld at least. */
private const val WILDCARD_MIN_LABELS = 2

/** A parent wildcard needs a real parent above host.tld. */
private const val PARENT_WILDCARD_MIN_LABELS = 3
