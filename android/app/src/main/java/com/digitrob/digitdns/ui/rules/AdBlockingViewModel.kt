@file:Suppress("TooGenericExceptionCaught")

package com.digitrob.digitdns.ui.rules

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.FilterListDto
import com.digitrob.digitdns.core.network.FilterPresetDto
import com.digitrob.digitdns.core.network.FilteringSettingsDto
import com.digitrob.digitdns.core.network.FilteringSettingsRequest
import com.digitrob.digitdns.core.network.FilteringStatusDto
import com.digitrob.digitdns.data.FilteringRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import java.io.IOException
import javax.inject.Inject
import java.util.Locale

/** One preset row, merged with its installed record when it is active. */
data class PresetRow(
    val preset: FilterPresetDto,
    val installed: FilterListDto?,
    val domainCount: Int?,
    val counting: Boolean,
    val countUnavailable: Boolean,
    val adding: Boolean,
    val updating: Boolean,
) {
    val isInstalled: Boolean get() = installed != null
    val isEnabled: Boolean get() = installed?.enabled == true
}

/** A catalogue group, in the same order the web page uses. */
data class FilteringGroup(
    val key: String,
    val title: String,
    val rows: List<PresetRow>,
)

data class AdBlockingUiState(
    val groups: List<FilteringGroup> = emptyList(),
    val customLists: List<FilterListDto> = emptyList(),
    val settings: FilteringSettingsDto = FilteringSettingsDto(),
    val enabled: Boolean = true,
    val blockMode: String = "null_ip",
    val blockTtl: String = "10",
    val updateIntervalHours: String = "24",
    val allowlist: String = "",
    val totalDomains: Int = 0,
    val activeCount: Int = 0,
    val listCount: Int = 0,
    val countsRunning: Boolean = false,
    val updating: Set<String> = emptySet(),
    val addingPresetIds: Set<String> = emptySet(),
    val search: String = "",
    val isLoading: Boolean = false,
    val isSavingSettings: Boolean = false,
    val error: String? = null,
    val message: String? = null,
    val customDialogOpen: Boolean = false,
    val customName: String = "",
    val customUrl: String = "",
    val customHomepage: String = "",
    val customContent: String = "",
    val customFileName: String = "",
    val customSaving: Boolean = false,
    val deleteTarget: FilterListDto? = null,
    // Read-only rule probe. Kept separate from the roster state so a failed test
    // never looks like a failed list update.
    val testDomain: String = "",
    val testLoading: Boolean = false,
    val testResult: com.digitrob.digitdns.core.network.RuleTestDto? = null,
    val testError: String? = null,
)

private val GROUP_ORDER = listOf(
    "base" to "General protection",
    "ads" to "Ads & trackers",
    "security" to "Malware, phishing & scams",
    "telemetry" to "Device & vendor telemetry",
    "extras" to "Optional extras",
)

/**
 * The Ad Blocking page. Roster changes are one-click: ticking a preset installs it,
 * ticking an installed row enables or disables it, and the trash icon deletes it.
 * The settings card keeps an explicit Save, matching the web page.
 */
@HiltViewModel
@Suppress("TooManyFunctions")
class AdBlockingViewModel @Inject constructor(
    private val repository: FilteringRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(AdBlockingUiState())
    val state: StateFlow<AdBlockingUiState> = _state.asStateFlow()

    private var lastStatus: FilteringStatusDto? = null
    private var pollJob: Job? = null

    init {
        load(includeSettings = true)
    }

    fun refresh() = load(includeSettings = true)

    fun load(includeSettings: Boolean = true) {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = true, error = null) }
            try {
                val status = repository.status()
                lastStatus = status
                applyStatus(status, includeSettings)
            } catch (e: Throwable) {
                _state.update { it.copy(isLoading = false, error = errorText(e)) }
            }
        }
    }

    private fun applyStatus(status: FilteringStatusDto, includeSettings: Boolean) {
        val current = _state.value
        val settings = if (includeSettings) status.settings else current.settings
        _state.update {
            it.copy(
                settings = settings,
                enabled = if (includeSettings) settings.enabled else it.enabled,
                blockMode = if (includeSettings) settings.blockMode else it.blockMode,
                blockTtl = if (includeSettings) settings.blockTtl.toString() else it.blockTtl,
                updateIntervalHours = if (includeSettings) {
                    settings.updateIntervalHours.toString()
                } else {
                    it.updateIntervalHours
                },
                allowlist = if (includeSettings) {
                    settings.allowlist.orEmpty().joinToString("\n")
                } else {
                    it.allowlist
                },
                groups = buildGroups(status, it.addingPresetIds, it.search),
                customLists = buildCustomLists(status, it.search),
                totalDomains = status.totalDomains,
                activeCount = status.lists.count { list -> list.enabled },
                listCount = status.lists.size,
                countsRunning = status.presetCountsRunning,
                updating = status.updating.toSet(),
                isLoading = false,
            )
        }
        schedulePoll()
    }

    /** Rebuild the roster from the last status without another round trip. */
    private fun recompute() {
        val status = lastStatus ?: return
        val current = _state.value
        _state.update {
            it.copy(
                groups = buildGroups(status, current.addingPresetIds, current.search),
                customLists = buildCustomLists(status, current.search),
            )
        }
    }

    fun onSearchChange(value: String) {
        _state.update { it.copy(search = value) }
        recompute()
    }

    fun onTestDomainChange(value: String) {
        _state.update { it.copy(testDomain = value, testError = null) }
    }

    /**
     * Ask the daemon how it would classify the typed name. Nothing is added or
     * recompiled: the answer is the same one the query log would record.
     */
    fun testRule() {
        val domain = _state.value.testDomain.trim().trimEnd('.')
        if (domain.isEmpty()) {
            _state.update { it.copy(testError = "Type a domain to test.") }
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(testLoading = true, testError = null, testResult = null) }
            try {
                val result = repository.testRule(domain)
                _state.update { it.copy(testLoading = false, testResult = result) }
            } catch (e: Throwable) {
                _state.update { it.copy(testLoading = false, testError = errorText(e)) }
            }
        }
    }

    fun setEnabled(value: Boolean) = _state.update { it.copy(enabled = value) }
    fun setBlockMode(value: String) = _state.update { it.copy(blockMode = value) }
    fun setBlockTtl(value: String) = _state.update { it.copy(blockTtl = value.filter { c -> c.isDigit() }) }
    fun setUpdateInterval(value: String) = _state.update { it.copy(updateIntervalHours = value.filter { c -> c.isDigit() }) }
    fun setAllowlist(value: String) = _state.update { it.copy(allowlist = value) }
    fun dismissMessage() = _state.update { it.copy(message = null) }
    fun dismissError() = _state.update { it.copy(error = null) }

    fun saveSettings() {
        val current = _state.value
        val ttl = current.blockTtl.toIntOrNull()
        val interval = current.updateIntervalHours.toIntOrNull()
        if (ttl == null || ttl !in MIN_BLOCK_TTL..MAX_BLOCK_TTL) {
            _state.update { it.copy(error = "Block TTL must be between 1 and 3600 seconds.") }
            return
        }
        if (interval == null || interval !in MIN_UPDATE_HOURS..MAX_UPDATE_HOURS) {
            _state.update { it.copy(error = "Update interval must be between 1 and 168 hours.") }
            return
        }
        val allowlist = parseLines(current.allowlist)
        viewModelScope.launch {
            _state.update { it.copy(isSavingSettings = true, error = null) }
            try {
                val status = repository.saveSettings(
                    FilteringSettingsRequest(
                        enabled = current.enabled,
                        blockMode = current.blockMode,
                        blockTtl = ttl,
                        updateIntervalHours = interval,
                        allowlist = allowlist,
                    ),
                )
                lastStatus = status
                applyStatus(status, includeSettings = true)
                _state.update { it.copy(isSavingSettings = false, message = "Ad blocking settings saved.") }
            } catch (e: Throwable) {
                _state.update { it.copy(isSavingSettings = false, error = errorText(e)) }
            }
        }
    }

    fun addPreset(id: String) {
        val current = _state.value
        if (id in current.addingPresetIds) return
        _state.update { it.copy(addingPresetIds = it.addingPresetIds + id) }
        recompute()
        viewModelScope.launch {
            try {
                val result = repository.addPreset(id)
                val failure = result.failed.firstOrNull()
                val skipped = result.skipped.firstOrNull()
                _state.update {
                    it.copy(
                        message = when {
                            failure != null -> failure.reason.ifBlank { "Could not add the preset." }
                            skipped != null -> "${skipped.name.ifBlank { "That preset" }} is already installed."
                            else -> "Preset added; downloading in the background."
                        },
                    )
                }
                reloadRoster()
            } catch (e: Throwable) {
                _state.update { it.copy(error = errorText(e)) }
                reloadRoster()
            } finally {
                _state.update { it.copy(addingPresetIds = it.addingPresetIds - id) }
                recompute()
            }
        }
    }

    fun toggleList(list: FilterListDto, enabled: Boolean) {
        setLocalEnabled(list.id, enabled)
        viewModelScope.launch {
            try {
                repository.setEnabled(list.id, enabled)
                _state.update {
                    it.copy(message = if (enabled) "List enabled." else "List disabled.")
                }
                reloadRoster()
            } catch (e: Throwable) {
                _state.update { it.copy(error = errorText(e)) }
                reloadRoster()
            }
        }
    }

    fun updateList(list: FilterListDto) {
        viewModelScope.launch {
            try {
                repository.update(list.id)
                _state.update { it.copy(message = "Updating ${list.name}…") }
                reloadRoster()
            } catch (e: Throwable) {
                _state.update { it.copy(error = errorText(e)) }
            }
        }
    }

    fun refreshAll() {
        viewModelScope.launch {
            try {
                repository.refreshAll()
                _state.update { it.copy(message = "Refreshing enabled lists…") }
                reloadRoster()
            } catch (e: Throwable) {
                _state.update { it.copy(error = errorText(e)) }
            }
        }
    }

    fun requestDelete(list: FilterListDto) = _state.update { it.copy(deleteTarget = list) }
    fun cancelDelete() = _state.update { it.copy(deleteTarget = null) }

    fun confirmDelete() {
        val target = _state.value.deleteTarget ?: return
        _state.update { it.copy(deleteTarget = null) }
        setLocalRemoved(target.id)
        viewModelScope.launch {
            try {
                repository.delete(target.id)
                _state.update { it.copy(message = "Deleted ${target.name}.") }
                reloadRoster()
            } catch (e: Throwable) {
                _state.update { it.copy(error = errorText(e)) }
                reloadRoster()
            }
        }
    }

    fun openCustomDialog() {
        _state.update {
            it.copy(
                customDialogOpen = true,
                customName = "",
                customUrl = "",
                customHomepage = "",
                customContent = "",
                customFileName = "",
                customSaving = false,
                error = null,
            )
        }
    }

    fun closeCustomDialog() = _state.update { it.copy(customDialogOpen = false) }
    fun setCustomName(value: String) = _state.update { it.copy(customName = value) }
    fun setCustomUrl(value: String) = _state.update { it.copy(customUrl = value) }
    fun setCustomHomepage(value: String) = _state.update { it.copy(customHomepage = value) }

    fun setCustomFile(name: String, content: String) {
        _state.update {
            it.copy(
                customFileName = name,
                customContent = content,
                customName = it.customName.ifBlank { name.substringBeforeLast('.') },
            )
        }
    }

    fun clearCustomFile() = _state.update { it.copy(customFileName = "", customContent = "") }

    fun reportFileError() = _state.update {
        it.copy(error = "Could not read that file. Choose a text file up to 16 MB.")
    }

    fun addCustomList() {
        val current = _state.value
        val name = current.customName.trim().ifBlank {
            current.customFileName.ifBlank { current.customUrl.trim() }
        }
        if (current.customUrl.isBlank() && current.customContent.isBlank()) {
            _state.update { it.copy(error = "Enter a list URL or choose a file.") }
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(customSaving = true, error = null) }
            try {
                repository.addList(
                    name = name,
                    url = current.customUrl.trim(),
                    homepage = current.customHomepage.trim(),
                    content = current.customContent,
                )
                _state.update {
                    it.copy(
                        customSaving = false,
                        customDialogOpen = false,
                        message = "Custom list added; compiling in the background.",
                    )
                }
                reloadRoster()
            } catch (e: Throwable) {
                _state.update { it.copy(customSaving = false, error = errorText(e)) }
            }
        }
    }

    private suspend fun reloadRoster() {
        try {
            val status = repository.status()
            lastStatus = status
            applyStatus(status, includeSettings = false)
        } catch (e: Throwable) {
            _state.update { it.copy(error = errorText(e)) }
        }
    }

    private fun setLocalEnabled(id: String, enabled: Boolean) {
        val status = lastStatus ?: return
        lastStatus = status.copy(
            lists = status.lists.map { if (it.id == id) it.copy(enabled = enabled) else it },
        )
        recompute()
    }

    private fun setLocalRemoved(id: String) {
        val status = lastStatus ?: return
        lastStatus = status.copy(lists = status.lists.filterNot { it.id == id })
        recompute()
    }

    private fun schedulePoll() {
        val current = _state.value
        if (!current.countsRunning && current.updating.isEmpty()) {
            pollJob?.cancel()
            pollJob = null
            return
        }
        if (pollJob?.isActive == true) return
        pollJob = viewModelScope.launch {
            var polling = true
            while (polling) {
                delay(POLL_MILLIS)
                val status = runCatching { repository.status() }.getOrNull()
                if (status == null) {
                    polling = false
                } else {
                    lastStatus = status
                    applyStatus(status, includeSettings = false)
                    polling = status.presetCountsRunning || status.updating.isNotEmpty()
                }
            }
        }
    }

    private fun errorText(e: Throwable): String = when (e) {
        is ApiException -> if (e.httpStatus == HTTP_NOT_FOUND) {
            "This server does not expose ad blocking. Update the server first."
        } else {
            e.userMessage
        }
        is IOException -> e.message ?: "Could not reach the server."
        else -> e.message ?: "Request failed."
    }

    private fun parseLines(text: String): List<String> =
        text.split('\n', ',', ' ')
            .map { it.trim() }
            .filter { it.isNotEmpty() }
            .distinct()

    private companion object {
        const val POLL_MILLIS = 3_000L
        const val MIN_BLOCK_TTL = 1
        const val MAX_BLOCK_TTL = 3_600
        const val MIN_UPDATE_HOURS = 1
        const val MAX_UPDATE_HOURS = 168
        const val HTTP_NOT_FOUND = 404
    }
}

/** Build the catalogue groups from the last status, applying the search filter. */
internal fun buildGroups(
    status: FilteringStatusDto,
    adding: Set<String>,
    search: String,
): List<FilteringGroup> {
    val installedByPreset = status.lists
        .filter { it.presetId.isNotBlank() }
        .associateBy { it.presetId }
    val installedByUrl = status.lists
        .filter { it.url.isNotBlank() }
        .associateBy { it.url }
    val counts = status.presetCounts.associateBy { it.presetId }
    val query = search.trim().lowercase(Locale.ROOT)
    val matches = { preset: FilterPresetDto, list: FilterListDto? ->
        query.isEmpty() || listOfNotNull(
            preset.name,
            preset.description,
            preset.license,
            preset.warning,
            list?.name,
            list?.url,
        ).any { it.lowercase(Locale.ROOT).contains(query) }
    }
    return GROUP_ORDER.mapNotNull { (key, title) ->
        val rows = status.presets
            .filter { it.group == key }
            .mapNotNull { preset ->
                val installed = installedByPreset[preset.id] ?: installedByUrl[preset.url]
                if (!matches(preset, installed)) return@mapNotNull null
                val count = counts[preset.id]
                PresetRow(
                    preset = preset,
                    installed = installed,
                    domainCount = installed?.domainCount ?: count?.domainCount?.takeIf { it > 0 },
                    counting = installed == null && count == null && status.presetCountsRunning,
                    countUnavailable = installed == null && count?.lastError?.isNotBlank() == true,
                    adding = preset.id in adding,
                    updating = installed != null && installed.id in status.updating,
                )
            }
        if (rows.isEmpty()) null else FilteringGroup(key = key, title = title, rows = rows)
    }
}

/** Installed lists that are not represented by a catalogue preset. */
internal fun buildCustomLists(status: FilteringStatusDto, search: String): List<FilterListDto> {
    val represented = buildGroups(status, emptySet(), "").flatMap { group ->
        group.rows.mapNotNull { it.installed?.id }
    }.toSet()
    val query = search.trim().lowercase(Locale.ROOT)
    return status.lists
        .filter { it.id !in represented }
        .filter {
            query.isEmpty() || listOf(it.name, it.url, it.license)
                .any { value -> value.lowercase(Locale.ROOT).contains(query) }
        }
}
