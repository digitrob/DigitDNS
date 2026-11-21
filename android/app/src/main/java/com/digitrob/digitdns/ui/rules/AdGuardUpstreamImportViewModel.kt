@file:Suppress("TooGenericExceptionCaught", "TooManyFunctions")

package com.digitrob.digitdns.ui.rules

import android.net.Uri
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.AdGuardApplyRequest
import com.digitrob.digitdns.core.network.AdGuardApplyResultDto
import com.digitrob.digitdns.core.network.AdGuardPreviewDto
import com.digitrob.digitdns.core.network.AdGuardPreviewRequest
import com.digitrob.digitdns.core.network.AdGuardRowDto
import com.digitrob.digitdns.core.network.AdGuardSourceDto
import com.digitrob.digitdns.core.network.AdGuardSourceSaveRequest
import com.digitrob.digitdns.data.AdGuardSyncRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

internal const val ADGUARD_REMOVAL_KEEP = "keep"
internal const val ADGUARD_REMOVAL_SYNCED_ONLY = "synced_only"
internal const val ADGUARD_REMOVAL_MIRROR = "mirror"

/** How many upstream addresses one compact notice may name before it trails off. */
private const val MAX_NOTICE_TARGETS = 3

data class AdGuardImportUiState(
    val sources: List<AdGuardSourceDto> = emptyList(),
    val selectedSourceId: String = "",
    val displayName: String = "",
    val enabled: Boolean = true,
    val managedUpstream: String = "",
    val text: String = "",
    val loadingSources: Boolean = false,
    val previewing: Boolean = false,
    val applying: Boolean = false,
    val saving: Boolean = false,
    val preview: AdGuardPreviewDto? = null,
    val selectedImport: Set<String> = emptySet(),
    val selectedManaged: Set<String> = emptySet(),
    val importTouched: Boolean = false,
    val search: String = "",
    val selectedOnly: Boolean = false,
    val removalMode: String = ADGUARD_REMOVAL_KEEP,
    val confirmMirror: Boolean = false,
    val acknowledgeRisk: Boolean = false,
    val result: AdGuardApplyResultDto? = null,
    val error: String? = null,
    val message: String? = null,
)

/** A compact, operator-facing notice. Raw server warning strings are never shown. */
internal data class AdGuardNoticeUi(
    val kind: String,
    val title: String,
    val message: String,
    val targets: List<String> = emptyList(),
    val domains: List<String> = emptyList(),
)

/**
 * State machine for the AdGuard upstream-list importer. The server owns parsing and
 * the preview checksum; this class owns selection, staged counts and the guardrails
 * around destructive removal modes. Nothing reaches the rule store until apply.
 */
@HiltViewModel
class AdGuardUpstreamImportViewModel @Inject constructor(
    private val repository: AdGuardSyncRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(AdGuardImportUiState())
    val state: StateFlow<AdGuardImportUiState> = _state.asStateFlow()

    init {
        loadSources()
    }

    fun loadSources() {
        viewModelScope.launch { refreshSources() }
    }

    fun selectSource(id: String) {
        val source = _state.value.sources.find { it.id == id }
        _state.update { current ->
            current.copy(
                selectedSourceId = id,
                displayName = source?.name.orEmpty(),
                enabled = source?.enabled ?: true,
                managedUpstream = source?.managedUpstream.orEmpty(),
                text = source?.text.orEmpty(),
                preview = null,
                selectedImport = emptySet(),
                selectedManaged = emptySet(),
                importTouched = false,
                result = null,
                error = null,
            )
        }
    }

    /** Load the selected saved source into the form and re-preview it. */
    fun loadSelectedSource() {
        val id = _state.value.selectedSourceId
        if (id.isBlank()) {
            _state.update { it.copy(error = "Select a saved source first.") }
            return
        }
        selectSource(id)
        preview(resetSelection = true)
    }

    fun newSource() = selectSource("")

    fun setDisplayName(value: String) = _state.update { it.copy(displayName = value, error = null) }

    fun setText(value: String) = _state.update { it.copy(text = value, error = null) }

    fun setEnabled(value: Boolean) = _state.update { it.copy(enabled = value) }

    fun setManagedUpstream(value: String) = _state.update { it.copy(managedUpstream = value) }

    fun setSearch(value: String) = _state.update { it.copy(search = value) }

    fun setSelectedOnly(value: Boolean) = _state.update { it.copy(selectedOnly = value) }

    fun setRemovalMode(value: String) = _state.update {
        it.copy(removalMode = value, confirmMirror = if (value == ADGUARD_REMOVAL_MIRROR) it.confirmMirror else false)
    }

    fun setConfirmMirror(value: Boolean) = _state.update { it.copy(confirmMirror = value) }

    fun setAcknowledgeRisk(value: Boolean) = _state.update { it.copy(acknowledgeRisk = value) }

    fun saveSource() {
        val current = _state.value
        if (current.text.isBlank()) {
            _state.update { it.copy(error = "Paste an AdGuard upstream list before saving.") }
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(saving = true, error = null, message = null) }
            try {
                val saved = repository.saveSource(
                    AdGuardSourceSaveRequest(
                        sourceId = current.selectedSourceId,
                        name = current.displayName.trim(),
                        text = current.text,
                        enabled = current.enabled,
                        managedUpstream = current.managedUpstream.trim(),
                    ),
                )
                refreshSources(saved.id)
                _state.update { it.copy(saving = false, message = "AdGuard source saved.") }
            } catch (e: Throwable) {
                _state.update { it.copy(saving = false, error = errorText(e)) }
            }
        }
    }

    fun deleteSource() {
        val id = _state.value.selectedSourceId
        if (id.isBlank()) {
            _state.update { it.copy(error = "Select a saved source before deleting.") }
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(saving = true, error = null, message = null) }
            try {
                repository.deleteSource(id)
                newSource()
                refreshSources()
                _state.update {
                    it.copy(saving = false, message = "AdGuard source deleted. Imported domains were kept.")
                }
            } catch (e: Throwable) {
                _state.update { it.copy(saving = false, error = errorText(e)) }
            }
        }
    }

    fun preview(resetSelection: Boolean = true) {
        viewModelScope.launch { fetchPreview(resetSelection) }
    }

    fun toggleImport(domain: String) = _state.update { current ->
        val next = if (domain in current.selectedImport) {
            current.selectedImport - domain
        } else {
            current.selectedImport + domain
        }
        current.copy(selectedImport = next, importTouched = true)
    }

    fun toggleManaged(domain: String) = _state.update { current ->
        val next = if (domain in current.selectedManaged) {
            current.selectedManaged - domain
        } else {
            current.selectedManaged + domain
        }
        current.copy(selectedManaged = next)
    }

    fun selectAllToAdd() = _state.update { current ->
        val preview = current.preview
        if (preview == null) {
            current
        } else {
            current.copy(selectedImport = importableDomains(preview), importTouched = true)
        }
    }

    fun clearImports() = _state.update { it.copy(selectedImport = emptySet(), importTouched = true) }

    fun selectAllToSendBack() = _state.update { current ->
        val preview = current.preview
        if (preview == null) {
            current
        } else {
            current.copy(selectedManaged = preview.managed.map { it.domain }.toSet())
        }
    }

    fun clearExports() = _state.update { it.copy(selectedManaged = emptySet()) }

    /** The list header's select/deselect acts on the currently visible importable rows. */
    fun toggleVisibleImports(domains: List<String>) = _state.update { current ->
        val visible = domains.toSet()
        val allSelected = visible.isNotEmpty() && visible.all { it in current.selectedImport }
        current.copy(
            selectedImport = if (allSelected) current.selectedImport - visible else current.selectedImport + visible,
            importTouched = true,
        )
    }

    @Suppress("ReturnCount")
    fun apply() {
        val current = _state.value
        val preview = current.preview ?: return
        if (current.text != preview.text) {
            _state.update { it.copy(error = "The source text changed. Preview it again before applying.") }
            return
        }
        if (current.selectedImport.isEmpty() && current.selectedManaged.isEmpty()) return
        if (current.removalMode == ADGUARD_REMOVAL_MIRROR && !current.confirmMirror) {
            _state.update { it.copy(error = "Confirm mirror mode before applying it.") }
            return
        }
        if (requiresAcknowledgement(preview) && !current.acknowledgeRisk) {
            _state.update { it.copy(error = "Acknowledge the conflicting or broad domains before applying.") }
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(applying = true, error = null, message = null) }
            try {
                val result = repository.apply(
                    AdGuardApplyRequest(
                        sourceId = preview.sourceId.ifBlank { current.selectedSourceId },
                        name = current.displayName.trim().ifBlank { preview.name },
                        text = preview.text,
                        checksum = preview.checksum,
                        enabled = current.enabled,
                        managedUpstream = current.managedUpstream.trim(),
                        managedSelection = current.selectedManaged.toList(),
                        importDomains = current.selectedImport.toList(),
                        managedDomains = current.selectedManaged.toList(),
                        removalMode = current.removalMode,
                        confirmMirror = current.confirmMirror,
                        acknowledgeRisk = current.acknowledgeRisk,
                    ),
                )
                _state.update {
                    it.copy(applying = false, result = result, message = "AdGuard sync applied to DigitDNS.")
                }
                refreshSources(result.source.id.ifBlank { preview.sourceId })
                fetchPreview(resetSelection = true)
            } catch (e: Throwable) {
                _state.update { it.copy(applying = false, error = errorText(e)) }
            }
        }
    }

    fun saveBlock(uri: Uri) {
        val block = _state.value.result?.generatedBlock?.takeIf { it.isNotBlank() }
            ?: _state.value.preview?.generatedBlock.orEmpty()
        if (block.isBlank()) {
            _state.update { it.copy(error = "Select DigitDNS-only domains before saving a block.") }
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(saving = true, error = null, message = null) }
            try {
                repository.saveBlock(uri, block)
                _state.update { it.copy(saving = false, message = "AdGuard block saved.") }
            } catch (e: Throwable) {
                _state.update { it.copy(saving = false, error = errorText(e)) }
            }
        }
    }

    fun blockCopied() = _state.update { it.copy(message = "AdGuard block copied.") }

    fun dismissMessage() = _state.update { it.copy(message = null) }

    fun dismissError() = _state.update { it.copy(error = null) }

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
                    enabled = selected?.enabled ?: current.enabled,
                    managedUpstream = selected?.managedUpstream?.takeIf { it.isNotBlank() } ?: current.managedUpstream,
                    // A refreshed source can omit text (for example a URL-backed one);
                    // never blank the form before the post-apply re-preview.
                    text = selected?.text?.takeIf { it.isNotBlank() } ?: current.text,
                )
            }
        } catch (e: Throwable) {
            _state.update { it.copy(loadingSources = false, error = errorText(e)) }
        }
    }

    private suspend fun fetchPreview(resetSelection: Boolean) {
        val current = _state.value
        if (current.text.isBlank()) {
            _state.update { it.copy(error = "Paste an AdGuard upstream list before previewing.") }
            return
        }
        _state.update { it.copy(previewing = true, error = null) }
        try {
            val preview = repository.preview(
                AdGuardPreviewRequest(
                    sourceId = current.selectedSourceId,
                    name = current.displayName.trim(),
                    text = current.text,
                    enabled = current.enabled,
                    managedUpstream = current.managedUpstream.trim(),
                    managedSelection = current.selectedManaged.toList(),
                ),
            )
            _state.update { state ->
                val importable = importableDomains(preview)
                val managedAllowed = preview.managed.map { it.domain }.toSet()
                state.copy(
                    previewing = false,
                    preview = preview,
                    // Default selection is every AdGuard-only row; a manual selection
                    // survives a re-preview but is pruned to what still importable.
                    selectedImport = if (resetSelection || !state.importTouched) {
                        importable
                    } else {
                        state.selectedImport intersect importable
                    },
                    selectedManaged = if (resetSelection) emptySet() else state.selectedManaged intersect managedAllowed,
                    importTouched = false,
                    result = null,
                )
            }
        } catch (e: Throwable) {
            _state.update { it.copy(previewing = false, error = errorText(e)) }
        }
    }

    private fun errorText(e: Throwable): String =
        e.message?.trim().takeUnless { it.isNullOrEmpty() } ?: "The server could not complete that request."
}

/** The four buckets share one row type; DigitDNS-only rows arrive in `managed`. */
internal fun adGuardDisplayRows(preview: AdGuardPreviewDto): List<AdGuardRowDto> = preview.rows + preview.managed

internal fun importableDomains(preview: AdGuardPreviewDto): Set<String> =
    preview.rows.filter { it.bucket == "adguardOnly" }.map { it.domain }.toSet()

internal fun filterAdGuardRows(
    rows: List<AdGuardRowDto>,
    query: String,
    selectedOnly: Boolean,
    selectedImport: Set<String>,
    selectedManaged: Set<String>,
): List<AdGuardRowDto> {
    val needle = query.trim().lowercase()
    return rows.filter { row ->
        val matches = needle.isBlank() || row.domain.contains(needle) ||
            row.group.contains(needle) || row.target.contains(needle)
        val isSelected = row.domain in selectedImport || row.domain in selectedManaged
        matches && (!selectedOnly || isSelected)
    }
}

internal fun requiresAcknowledgement(preview: AdGuardPreviewDto): Boolean {
    val rows = adGuardDisplayRows(preview)
    return preview.counts.conflict > 0 || preview.broadCount > 0 || rows.any { it.broad } ||
        preview.warnings.any { it.contains("broad domain", ignoreCase = true) }
}

/** Before apply, this is an estimate from this source's own provenance. */
internal fun estimatedRemovals(source: AdGuardSourceDto?, preview: AdGuardPreviewDto, removalMode: String): Int {
    if (removalMode == ADGUARD_REMOVAL_KEEP) return 0
    val named = adGuardDisplayRows(preview).map { it.domain }.toSet()
    return source?.imported?.count { it !in named } ?: 0
}

@Suppress("ReturnCount")
internal fun canApplyAdGuardSync(state: AdGuardImportUiState): Boolean {
    val preview = state.preview ?: return false
    if (state.applying || preview.checksum.isBlank() || state.text != preview.text) return false
    if (state.selectedImport.isEmpty() && state.selectedManaged.isEmpty()) return false
    if (state.removalMode == ADGUARD_REMOVAL_MIRROR && !state.confirmMirror) return false
    if (requiresAcknowledgement(preview) && !state.acknowledgeRisk) return false
    return true
}

/**
 * Prefer the server's structured notices. On older installs the additive fields are
 * absent, so a small set of friendly notices is derived from rows, targets and the
 * shape of the warnings — never by printing each raw warning string in the UI.
 */
// Several independent additive/legacy signals feed one short, grouped notice list.
@Suppress("CyclomaticComplexMethod")
internal fun friendlyAdGuardNotices(preview: AdGuardPreviewDto): List<AdGuardNoticeUi> {
    if (preview.notices.isNotEmpty()) {
        return preview.notices.map { notice ->
            AdGuardNoticeUi(
                // The server calls this `upstream`; the review pill calls it
                // `external_upstream`. Normalize once so the summary can rely on it.
                kind = if (notice.kind == "upstream") "external_upstream" else notice.kind,
                title = notice.title.ifBlank { "Review this source" },
                message = notice.message,
                targets = notice.targets,
                domains = notice.domains,
            )
        }
    }

    val result = mutableListOf<AdGuardNoticeUi>()
    val rows = adGuardDisplayRows(preview)
    val externalFromRows = rows.filter { !it.targetIsSelf && it.target.isNotBlank() }.map { it.target }
    val selfTargets = rows.filter { it.targetIsSelf }.map { it.target } +
        preview.groups.filter { it.targetIsSelf }.map { it.target }
    val externalTargets = (preview.foreignTargets + externalFromRows)
        .filter { it.isNotBlank() && it !in selfTargets }
        .distinct()
        .take(MAX_NOTICE_TARGETS)
    val shownTargets = externalTargets.ifEmpty {
        listOf(preview.upstream).filter { it.isNotBlank() && selfTargets.isEmpty() }
    }
    if (shownTargets.isNotEmpty()) {
        result += AdGuardNoticeUi(
            kind = "external_upstream",
            title = "AdGuard keeps its current routing",
            message = "Domains are copied to DigitDNS, but AdGuard will keep routing to " +
                shownTargets.joinToString(", ") + " until its config is changed.",
            targets = shownTargets,
        )
    }

    val broadFromRows = rows.count { it.broad }
    val broadCount = if (broadFromRows > 0) broadFromRows else preview.broadCount
    val broadWarning = preview.warnings.any { it.contains("broad domain", ignoreCase = true) }
    if (broadCount > 0 || broadWarning) {
        result += AdGuardNoticeUi(
            kind = "broad",
            title = "Broad domains need a second look",
            message = if (broadCount > 0) {
                "$broadCount broad domain${if (broadCount == 1) "" else "s"} are in this list. Import only what you intend."
            } else {
                "This list includes a broad domain. Import only what you intend."
            },
        )
    }

    val otherWarnings = preview.warnings.count { warning ->
        !warning.contains("broad domain", ignoreCase = true) &&
            !warning.contains("Target is not this DigitDNS server", ignoreCase = true)
    }
    if (otherWarnings > 0) {
        result += AdGuardNoticeUi(
            kind = "review",
            title = "A few details to review",
            message = "$otherWarnings additional item${if (otherWarnings == 1) "" else "s"} were reported for this source.",
        )
    }
    return result
}
