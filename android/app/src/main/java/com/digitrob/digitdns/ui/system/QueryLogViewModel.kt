package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.QueryLogItemDto
import com.digitrob.digitdns.core.network.QueryLogSettingsDto
import com.digitrob.digitdns.core.network.QueryLogUsageDto
import com.digitrob.digitdns.data.QueryLogRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import java.time.Instant
import javax.inject.Inject

data class QueryLogUiState(
    val settings: QueryLogSettingsDto = QueryLogSettingsDto(),
    val usage: QueryLogUsageDto = QueryLogUsageDto(),
    val items: List<QueryLogItemDto> = emptyList(),
    val domain: String = "",
    val client: String = "",
    val actions: Set<String> = emptySet(),
    val protos: Set<String> = emptySet(),
    /** Wire name ("A") or empty for any type. */
    val qtype: String = "",
    /** Wire name ("NXDOMAIN") or empty for any code. */
    val rcode: String = "",
    /** "", "true" (cached only) or "false" (uncached only). */
    val cached: String = "",
    /** Zero means no lower time bound; otherwise hours back from now. */
    val rangeHours: Int = 0,
    val nextCursor: String = "",
    val hasMore: Boolean = false,
    val isLoading: Boolean = false,
    /** False until the saved query-log policy lands; the settings card waits for it. */
    val settingsLoaded: Boolean = false,
    val isLoadingMore: Boolean = false,
    val isSaving: Boolean = false,
    val error: String? = null,
    val message: String? = null,
)

@HiltViewModel
class QueryLogViewModel @Inject constructor(
    private val repository: QueryLogRepository,
) : ViewModel() {
    private val _state = MutableStateFlow(QueryLogUiState())
    val state: StateFlow<QueryLogUiState> = _state.asStateFlow()

    private var searchJob: Job? = null

    init { loadSettings() }

    fun loadSettings() {
        viewModelScope.launch {
            try {
                val envelope = repository.settings()
                _state.update { it.copy(settings = envelope.settings, usage = envelope.usage, settingsLoaded = true) }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not load query log settings.") }
            }
        }
    }

    fun setEnabled(v: Boolean) = _state.update { it.copy(settings = it.settings.copy(enabled = v)) }
    fun setRetention(days: Int) = _state.update { it.copy(settings = it.settings.copy(retentionDays = days)) }
    fun setAnonymize(v: Boolean) = _state.update { it.copy(settings = it.settings.copy(anonymize = v)) }
    fun setMaxMb(mb: Int) = _state.update { it.copy(settings = it.settings.copy(maxMb = mb)) }
    fun setDomain(v: String) = _state.update { it.copy(domain = v) }
    fun setClient(v: String) = _state.update { it.copy(client = v) }
    fun setQtype(v: String) = _state.update { it.copy(qtype = v) }
    fun setRcode(v: String) = _state.update { it.copy(rcode = v) }
    fun setCached(v: String) = _state.update { it.copy(cached = v) }
    fun setRangeHours(v: Int) = _state.update { it.copy(rangeHours = v) }
    fun toggleAction(v: String) = _state.update {
        it.copy(actions = if (it.actions.contains(v)) it.actions - v else it.actions + v)
    }

    fun toggleProto(v: String) = _state.update {
        it.copy(protos = if (it.protos.contains(v)) it.protos - v else it.protos + v)
    }

    fun save() {
        viewModelScope.launch {
            _state.update { it.copy(isSaving = true, error = null) }
            try {
                val envelope = repository.save(_state.value.settings)
                _state.update {
                    it.copy(
                        settings = envelope.settings,
                        usage = envelope.usage,
                        isSaving = false,
                        message = "Query log settings saved.",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(isSaving = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isSaving = false, error = e.message ?: "Could not save query log settings.") }
            }
        }
    }

    /** A new search replaces the page and forgets the old cursor. */
    fun search() = load(reset = true)

    /** Load older continues from the cursor the previous page returned. */
    fun loadMore() {
        val s = _state.value
        // Split so the condition stays readable and under the complexity threshold:
        // "busy" and "nothing to load" are different reasons to do nothing.
        if (s.isLoading || s.isLoadingMore) return
        if (!s.hasMore || s.nextCursor.isBlank()) return
        load(reset = false)
    }

    private fun load(reset: Boolean) {
        val s = _state.value
        searchJob?.cancel()
        searchJob = viewModelScope.launch {
            _state.update {
                it.copy(
                    isLoading = reset,
                    isLoadingMore = !reset,
                    error = null,
                    items = if (reset) emptyList() else it.items,
                    nextCursor = if (reset) "" else it.nextCursor,
                    hasMore = if (reset) false else it.hasMore,
                )
            }
            try {
                val from = if (s.rangeHours > 0) {
                    Instant.now().minusSeconds(s.rangeHours.toLong() * SECONDS_PER_HOUR).toString()
                } else {
                    ""
                }
                val response = repository.search(
                    domain = s.domain,
                    client = s.client,
                    action = s.actions.joinToString(","),
                    proto = s.protos.joinToString(","),
                    qtype = s.qtype,
                    rcode = s.rcode,
                    cached = s.cached,
                    from = from,
                    before = if (reset) "" else s.nextCursor,
                )
                _state.update {
                    it.copy(
                        items = if (reset) response.items else it.items + response.items,
                        nextCursor = response.nextCursor,
                        hasMore = response.hasMore,
                        isLoading = false,
                        isLoadingMore = false,
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(isLoading = false, isLoadingMore = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update {
                    it.copy(
                        isLoading = false,
                        isLoadingMore = false,
                        error = e.message ?: "Could not search the query log.",
                    )
                }
            }
        }
    }

    fun clear() {
        viewModelScope.launch {
            try {
                repository.clear()
                _state.update {
                    it.copy(
                        items = emptyList(),
                        nextCursor = "",
                        hasMore = false,
                        usage = QueryLogUsageDto(),
                        message = "Query history cleared.",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not clear query history.") }
            }
        }
    }
}

private const val SECONDS_PER_HOUR = 3600L
