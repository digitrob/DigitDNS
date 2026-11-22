@file:Suppress("TooGenericExceptionCaught", "TooManyFunctions", "LongParameterList")

package com.digitrob.digitdns.ui.rules

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.AdGuardApplyRequest
import com.digitrob.digitdns.core.network.AdGuardPreviewDto
import com.digitrob.digitdns.core.network.AdGuardPreviewRequest
import com.digitrob.digitdns.core.network.AdGuardRowDto
import com.digitrob.digitdns.core.network.AdGuardSourceDto
import com.digitrob.digitdns.core.network.AdGuardSourceSaveRequest
import com.digitrob.digitdns.data.AdGuardSyncRepository
import com.digitrob.digitdns.data.RulesRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import javax.inject.Inject

internal const val ADGUARD_REMOVAL_KEEP = "keep"

internal const val ADGUARD_ACTION_PROXY = "proxy"
internal const val ADGUARD_ACTION_DIRECT = "direct"
internal const val ADGUARD_ACTION_BLOCK = "block"

private val ADGUARD_ACTIONS = setOf(ADGUARD_ACTION_PROXY, ADGUARD_ACTION_DIRECT, ADGUARD_ACTION_BLOCK)

/**
 * The importer's scope resolver. A wildcard named by the AdGuard source is already a
 * complete scope: deriving `*.host` or `*.parent` from it would stack a wildcard on
 * top of a wildcard, so only the listed pattern is offered.
 */
internal fun adGuardScopeTargets(domain: String, scope: MatchScope): List<String> {
    val raw = domain.trim().trimStart('.').lowercase(Locale.US)
    return when {
        raw.isEmpty() -> emptyList()
        raw.startsWith("*.") -> if (scope == MatchScope.Exact) listOf(raw) else emptyList()
        else -> scope.targets(domain)
    }
}

/** Which of the two mutually exclusive review filters is pressed, if any. */
enum class AdGuardFilter { All, SelectedOnly, AlreadyPresent }

data class AdGuardImportUiState(
    val sources: List<AdGuardSourceDto> = emptyList(),
    val selectedSourceId: String = "",
    val displayName: String = "",
    val text: String = "",
    val sourceMenuOpen: Boolean = false,
    val deleteConfirmation: Boolean = false,
    val loadingSources: Boolean = false,
    val previewing: Boolean = false,
    val applying: Boolean = false,
    val saving: Boolean = false,
    val preview: AdGuardPreviewDto? = null,
    val selectedImport: Set<String> = emptySet(),
    val importTouched: Boolean = false,
    val recentlyUnchecked: Set<String> = emptySet(),
    val search: String = "",
    val filter: AdGuardFilter = AdGuardFilter.All,
    /** Keys are resolved patterns, not the row that opened the shared menu. */
    val domainActions: Map<String, String> = emptyMap(),
    /** A scope replaces the origin pattern in both import and domain_actions. */
    val stagedScopes: Map<String, List<String>> = emptyMap(),
    /** Covered/conflict rows whose route the operator explicitly edited. */
    val existingRoutes: Set<String> = emptySet(),
    /** All four DigitDNS routing lists, used to disable colliding wildcard scopes. */
    val digitDnsPatterns: Set<String> = emptySet(),
    /** True when staged review inputs changed after the last preview checksum. */
    val reviewDirty: Boolean = false,
    val status: String = "Ready",
    val error: String? = null,
    val message: String? = null,
)

/**
 * State machine for the add-only AdGuard importer. Parsing, coverage and checksums
 * belong to the server; this class owns selection, staged route scopes, the mutually
 * exclusive filters and the exact preview text/checksum carried into apply.
 */
@HiltViewModel
class AdGuardUpstreamImportViewModel @Inject constructor(
    private val repository: AdGuardSyncRepository,
    private val rulesRepository: RulesRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(AdGuardImportUiState())
    val state: StateFlow<AdGuardImportUiState> = _state.asStateFlow()

    init {
        loadSources()
        loadPatterns()
    }

    fun loadSources() {
        viewModelScope.launch { refreshSources() }
    }

    /** Best effort, like the live log: a failure leaves scope collision checks permissive. */
    fun loadPatterns() {
        viewModelScope.launch {
            runCatching { rulesRepository.load() }
                .onSuccess { lists ->
                    val patterns = lists.managedPatterns
                        .map { it.trim().trimStart('.').trimEnd('.').lowercase(Locale.US) }
                        .filter { it.isNotEmpty() }
                        .toSet()
                    _state.update { it.copy(digitDnsPatterns = patterns) }
                }
        }
    }

    fun toggleSourceMenu() = _state.update { it.copy(sourceMenuOpen = !it.sourceMenuOpen) }

    fun dismissSourceMenu() = _state.update { it.copy(sourceMenuOpen = false) }

    /** Selecting a saved list loads and previews it; there is no separate Load button. */
    fun loadSource(id: String) {
        if (id.isBlank()) {
            newSource()
            return
        }
        val source = _state.value.sources.find { it.id == id }
        if (source == null) {
            _state.update { it.copy(error = "That saved list is no longer available.") }
            return
        }
        _state.update { current ->
            current.copy(
                selectedSourceId = id,
                displayName = source.name,
                text = source.text,
                sourceMenuOpen = false,
                deleteConfirmation = false,
                preview = null,
                selectedImport = emptySet(),
                importTouched = false,
                recentlyUnchecked = emptySet(),
                domainActions = emptyMap(),
                stagedScopes = emptyMap(),
                existingRoutes = emptySet(),
                reviewDirty = false,
                status = "Source loaded",
                error = null,
                message = null,
            )
        }
        preview(resetSelection = true)
    }

    fun newSource() = _state.update { current ->
        current.copy(
            selectedSourceId = "",
            displayName = "",
            text = "",
            sourceMenuOpen = false,
            deleteConfirmation = false,
            preview = null,
            selectedImport = emptySet(),
            importTouched = false,
            recentlyUnchecked = emptySet(),
            domainActions = emptyMap(),
            stagedScopes = emptyMap(),
            existingRoutes = emptySet(),
            reviewDirty = false,
            search = "",
            filter = AdGuardFilter.All,
            status = "Ready",
            error = null,
            message = null,
        )
    }

    fun setDisplayName(value: String) = _state.update { it.copy(displayName = value, error = null) }

    fun setText(value: String) = _state.update { it.copy(text = value, error = null) }

    fun setSearch(value: String) = _state.update { it.copy(search = value) }

    fun setFilter(value: AdGuardFilter) = _state.update { current ->
        current.copy(filter = if (current.filter == value) AdGuardFilter.All else value, recentlyUnchecked = emptySet())
    }

    fun saveSource() {
        val current = _state.value
        if (current.text.isBlank()) {
            _state.update { it.copy(error = "Paste an AdGuard upstream list before saving.") }
            return
        }
        val name = current.displayName.trim().ifBlank { defaultSourceName() }
        viewModelScope.launch {
            _state.update { it.copy(saving = true, status = "Saving…", error = null, message = null, displayName = name) }
            try {
                val saved = repository.saveSource(
                    AdGuardSourceSaveRequest(
                        sourceId = current.selectedSourceId,
                        name = name,
                        text = current.text,
                    ),
                )
                refreshSources(saved.id)
                _state.update { it.copy(saving = false, status = "List saved", message = "AdGuard list saved.") }
            } catch (e: Throwable) {
                _state.update { it.copy(saving = false, status = "Save failed", error = errorText(e)) }
            }
        }
    }

    fun requestDelete() {
        if (_state.value.selectedSourceId.isBlank()) {
            _state.update { it.copy(error = "Select a saved list before deleting.") }
            return
        }
        _state.update { it.copy(deleteConfirmation = true, sourceMenuOpen = false) }
    }

    fun dismissDelete() = _state.update { it.copy(deleteConfirmation = false) }

    fun confirmDelete() {
        val id = _state.value.selectedSourceId
        if (id.isBlank()) {
            dismissDelete()
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(saving = true, deleteConfirmation = false, error = null, message = null) }
            try {
                repository.deleteSource(id)
                newSource()
                refreshSources()
                _state.update {
                    it.copy(
                        saving = false,
                        status = "List deleted",
                        message = "AdGuard source deleted. Imported domains were kept.",
                    )
                }
            } catch (e: Throwable) {
                _state.update { it.copy(saving = false, status = "Delete failed", error = errorText(e)) }
            }
        }
    }

    fun preview(resetSelection: Boolean = true) {
        loadPatterns()
        viewModelScope.launch {
            try {
                fetchPreviewSuspend(resetSelection)
            } catch (_: Throwable) {
                // fetchPreviewSuspend already recorded a friendly error in state.
            }
        }
    }

    fun toggleImport(domain: String) = _state.update { current ->
        val wasSelected = domain in current.selectedImport
        val next = if (wasSelected) current.selectedImport - domain else current.selectedImport + domain
        val recently = if (wasSelected) current.recentlyUnchecked + domain else current.recentlyUnchecked - domain
        pruneStagedRoutes(current.copy(selectedImport = next, recentlyUnchecked = recently, importTouched = true))
    }

    /** The header checkbox acts on importable rows visible under the current filter. */
    fun toggleVisibleImports(domains: List<String>) = _state.update { current ->
        val visible = domains.toSet()
        val allSelected = visible.isNotEmpty() && visible.all { it in current.selectedImport }
        val next = if (allSelected) current.selectedImport - visible else current.selectedImport + visible
        val recently = if (allSelected) current.recentlyUnchecked - visible else current.recentlyUnchecked
        pruneStagedRoutes(current.copy(selectedImport = next, recentlyUnchecked = recently, importTouched = true))
    }

    /** Proxy / Direct / Block bulk chips keep each row's staged scope. */
    fun setBulkRoute(action: String) {
        if (action !in ADGUARD_ACTIONS) return
        _state.update { current ->
            current.selectedImport.fold(current) { acc, origin ->
                val targets = acc.stagedScopes[origin].takeUnless { it.isNullOrEmpty() } ?: listOf(origin)
                applyRoute(acc, origin, action, targets)
            }
        }
    }

    /**
     * Called by the shared Domains-list menu in importer mode. A scope click and an
     * action click both stage the resolved patterns so apply writes exactly what the
     * menu previewed.
     */
    fun setRoute(domain: String, action: String, targets: List<String>) {
        if (action !in ADGUARD_ACTIONS) return
        _state.update { current -> applyRoute(current, domain, action, targets) }
    }

    fun apply() {
        val initial = _state.value
        val initialPreview = initial.preview
        if (initialPreview == null || (initial.selectedImport.isEmpty() && initial.existingRoutes.isEmpty())) return
        if (initial.text != initialPreview.text) {
            preview(resetSelection = true)
            _state.update { it.copy(message = "The source changed. Review the new preview before applying.") }
        } else {
            loadPatterns()
            viewModelScope.launch {
                _state.update { it.copy(applying = true, status = "Applying…", error = null, message = null) }
                try {
                    if (_state.value.reviewDirty) fetchPreviewSuspend(resetSelection = false)
                    val current = _state.value
                    val preview = current.preview ?: return@launch
                    val result = repository.apply(
                        AdGuardApplyRequest(
                            sourceId = preview.sourceId.ifBlank { current.selectedSourceId },
                            name = current.displayName.trim().ifBlank { preview.name },
                            text = preview.text,
                            checksum = preview.checksum,
                            importDomains = adGuardActiveTargets(current).toList().sorted(),
                            removalMode = ADGUARD_REMOVAL_KEEP,
                            confirmMirror = false,
                            acknowledgeRisk = true,
                            domainActions = resolvedDomainActions(current),
                        ),
                    )
                    _state.update {
                        it.copy(applying = false, status = "Applied", message = "AdGuard upstream list applied to DigitDNS.")
                    }
                    loadPatterns()
                    refreshSources(result.source.id.ifBlank { preview.sourceId })
                    fetchPreviewSuspend(resetSelection = true)
                } catch (e: Throwable) {
                    _state.update { it.copy(applying = false, status = "Apply failed", error = errorText(e)) }
                }
            }
        }
    }

    fun dismissMessage() = _state.update { it.copy(message = null) }

    fun dismissError() = _state.update { it.copy(error = null) }

    private fun applyRoute(
        current: AdGuardImportUiState,
        domain: String,
        action: String,
        targets: List<String>,
    ): AdGuardImportUiState {
        val row = current.preview?.rows?.firstOrNull { it.domain == domain }
        val patterns = targets.map { it.trim().lowercase(Locale.US) }.filter { it.isNotEmpty() }.distinct()
        if (row == null || row.bucket == "digitdnsOnly" || patterns.isEmpty()) return current
        val oldPatterns = current.stagedScopes[domain] ?: listOf(domain)

        val actions = current.domainActions.toMutableMap()
        oldPatterns.forEach { old -> if (old !in patterns) actions.remove(old) }
        patterns.forEach { pattern -> actions[pattern] = action }

        val scopes = current.stagedScopes.toMutableMap()
        scopes[domain] = patterns

        val existingRoutes = if (row.bucket == "adguardOnly") {
            current.existingRoutes
        } else {
            current.existingRoutes + domain
        }

        return pruneStagedRoutes(
            current.copy(
                domainActions = actions,
                stagedScopes = scopes,
                existingRoutes = existingRoutes,
                reviewDirty = true,
            ),
        )
    }

    /** A re-preview can remove rows; stale scopes and actions must not survive it. */
    private fun pruneStagedRoutes(current: AdGuardImportUiState): AdGuardImportUiState {
        val parsed = current.preview?.rows
            ?.filter { it.bucket != "digitdnsOnly" }
            ?.map { it.domain }
            ?.toSet()
            .orEmpty()
        val existingRoutes = current.existingRoutes intersect parsed
        val origins = current.selectedImport + existingRoutes
        val stagedScopes = current.stagedScopes.filterKeys { it in origins }
        val pruned = current.copy(
            existingRoutes = existingRoutes,
            stagedScopes = stagedScopes,
        )
        val active = adGuardActiveTargets(pruned)
        return pruned.copy(domainActions = pruned.domainActions.filterKeys { it in active })
    }

    private suspend fun refreshSources(preferredId: String? = null) {
        _state.update { it.copy(loadingSources = true, error = null) }
        try {
            val sources = repository.sources()
            val wanted = preferredId ?: _state.value.selectedSourceId
            val selected = sources.find { it.id == wanted }
            _state.update { current ->
                current.copy(
                    sources = sources,
                    loadingSources = false,
                    selectedSourceId = selected?.id.orEmpty(),
                    displayName = selected?.name?.takeIf { it.isNotBlank() } ?: current.displayName,
                    // A refreshed source can omit text; never blank a form that is
                    // about to be re-previewed after save or apply.
                    text = selected?.text?.takeIf { it.isNotBlank() } ?: current.text,
                )
            }
        } catch (e: Throwable) {
            _state.update { it.copy(loadingSources = false, error = errorText(e)) }
        }
    }

    private suspend fun fetchPreviewSuspend(resetSelection: Boolean) {
        val initial = _state.value
        if (initial.text.isBlank()) {
            _state.update { it.copy(error = "Paste an AdGuard upstream list before previewing.") }
            return
        }
        val name = initial.displayName.trim().ifBlank { defaultSourceName() }
        _state.update { it.copy(previewing = true, status = "Previewing…", error = null, displayName = name) }
        try {
            val current = _state.value
            val preview = repository.preview(
                AdGuardPreviewRequest(
                    sourceId = current.selectedSourceId,
                    name = name,
                    text = current.text,
                    domainActions = resolvedDomainActions(current),
                    removalMode = ADGUARD_REMOVAL_KEEP,
                    confirmMirror = false,
                    acknowledgeRisk = true,
                ),
            )
            _state.update { state ->
                val importable = importableDomains(preview)
                val next = if (resetSelection) {
                    state.copy(
                        selectedImport = importable,
                        importTouched = false,
                        recentlyUnchecked = emptySet(),
                        domainActions = emptyMap(),
                        stagedScopes = emptyMap(),
                        existingRoutes = emptySet(),
                    )
                } else {
                    state.copy(selectedImport = state.selectedImport intersect importable, recentlyUnchecked = emptySet())
                }
                val refreshed = next.copy(
                    previewing = false,
                    status = "Preview ready",
                    preview = preview,
                    reviewDirty = false,
                )
                pruneStagedRoutes(refreshed)
            }
        } catch (e: Throwable) {
            _state.update { it.copy(previewing = false, status = "Preview failed", error = errorText(e)) }
            throw e
        }
    }

    private fun errorText(e: Throwable): String =
        e.message?.trim().takeUnless { it.isNullOrEmpty() } ?: "The server could not complete that request."
}

/** Only parsed AdGuard rows appear in the review; send-back rows are out of scope. */
internal fun adGuardDisplayRows(preview: AdGuardPreviewDto): List<AdGuardRowDto> = preview.rows

internal fun importableDomains(preview: AdGuardPreviewDto): Set<String> =
    preview.rows.filter { it.bucket == "adguardOnly" }.map { it.domain }.toSet()

/**
 * A wildcard scope replaces its origin. Explicit edits on already-present rows ride
 * alongside new imports, but never become source-owned provenance on the server.
 */
internal fun adGuardActiveTargets(state: AdGuardImportUiState): Set<String> {
    val origins = state.selectedImport + state.existingRoutes
    return origins.flatMap { origin ->
        state.stagedScopes[origin]?.takeIf { it.isNotEmpty() } ?: listOf(origin)
    }.toSet()
}

internal fun resolvedDomainActions(state: AdGuardImportUiState): Map<String, String> =
    state.domainActions.filterKeys { it in adGuardActiveTargets(state) }

internal fun adGuardRowAction(row: AdGuardRowDto, state: AdGuardImportUiState): String {
    val staged = state.stagedScopes[row.domain].orEmpty()
    val explicit = state.domainActions[row.domain]
        ?: staged.firstNotNullOfOrNull { pattern -> state.domainActions[pattern] }
    if (explicit != null && explicit in ADGUARD_ACTIONS) return explicit
    return row.suggestedAction.takeIf { it in ADGUARD_ACTIONS } ?: ADGUARD_ACTION_PROXY
}

internal data class AdGuardStagedCounts(val imports: Int, val routeChanges: Int)

internal fun adGuardStagedCounts(state: AdGuardImportUiState): AdGuardStagedCounts {
    val active = adGuardActiveTargets(state)
    val routes = state.domainActions.keys
        .filter { it in active && state.domainActions[it] != ADGUARD_ACTION_PROXY }
        .toSet() + state.existingRoutes
    return AdGuardStagedCounts(imports = state.selectedImport.size, routeChanges = routes.size)
}

internal fun filterAdGuardRows(
    rows: List<AdGuardRowDto>,
    query: String,
    filter: AdGuardFilter,
    selectedImport: Set<String>,
    recentlyUnchecked: Set<String> = emptySet(),
): List<AdGuardRowDto> {
    val needle = query.trim().lowercase(Locale.US)
    return rows.filter { row ->
        if (row.bucket == "digitdnsOnly") return@filter false
        val matches = needle.isBlank() || row.domain.contains(needle) ||
            row.group.contains(needle) || row.target.contains(needle)
        when (filter) {
            AdGuardFilter.All -> matches
            AdGuardFilter.SelectedOnly -> matches && (row.domain in selectedImport || row.domain in recentlyUnchecked)
            AdGuardFilter.AlreadyPresent -> matches && row.bucket == "covered"
        }
    }
}

internal fun canApplyAdGuardSync(state: AdGuardImportUiState): Boolean {
    val preview = state.preview ?: return false
    return !state.applying && preview.checksum.isNotBlank() && state.text == preview.text &&
        (state.selectedImport.isNotEmpty() || state.existingRoutes.isNotEmpty())
}

internal fun defaultSourceName(now: Date = Date()): String =
    "AdGuard Home — " + SimpleDateFormat("yyyy-MM-dd HH:mm", Locale.getDefault()).format(now)
