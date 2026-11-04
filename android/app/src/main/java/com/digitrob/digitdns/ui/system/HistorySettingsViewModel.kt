package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.StatsSettings
import com.digitrob.digitdns.data.HistoryRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import javax.inject.Inject

private const val DEFAULT_RETENTION_DAYS = 90
private const val DEFAULT_TOP_DOMAINS = 1000
private const val DEFAULT_TOP_CLIENTS = 500
private const val MIN_RETENTION_DAYS = 7
private const val MAX_RETENTION_DAYS = 365
private const val MIN_TOP_DOMAINS = 100
private const val MAX_TOP_DOMAINS = 10_000
private const val MIN_TOP_CLIENTS = 50
private const val MAX_TOP_CLIENTS = 5_000
private const val MAX_EXCLUDED_CLIENTS = 500
private const val HTTP_NOT_FOUND = 404
private const val HTTP_UNAVAILABLE = 503

data class HistorySettingsUiState(
    val enabled: Boolean = true,
    val anonymizeClients: Boolean = false,
    val retentionDays: String = DEFAULT_RETENTION_DAYS.toString(),
    val topDomains: String = DEFAULT_TOP_DOMAINS.toString(),
    val topClients: String = DEFAULT_TOP_CLIENTS.toString(),
    val excludeClients: String = "",
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    val unsupported: Boolean = false,
    val saved: Boolean = false,
    val error: String? = null,
)

/**
 * The server-wide aggregate policy. It is not an app preference: every install
 * has its own `stats.db` and restart-scoped configuration, so this screen reads
 * and writes through the v3 API rather than DataStore.
 */
@HiltViewModel
class HistorySettingsViewModel @Inject constructor(
    private val repository: HistoryRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(HistorySettingsUiState())
    val state: StateFlow<HistorySettingsUiState> = _state.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = true, error = null, saved = false) }
            try {
                val settings = repository.settings()
                _state.update { it.apply(settings).copy(isLoading = false, unsupported = false) }
            } catch (e: ApiException) {
                _state.update {
                    if (e.httpStatus == HTTP_NOT_FOUND || e.httpStatus == HTTP_UNAVAILABLE) {
                        it.copy(isLoading = false, unsupported = true, error = null)
                    } else {
                        it.copy(isLoading = false, error = e.userMessage)
                    }
                }
            } catch (e: IOException) {
                _state.update { it.copy(isLoading = false, error = e.message ?: "Could not load history settings.") }
            }
        }
    }

    fun onEnabledChange(value: Boolean) = _state.update { it.copy(enabled = value, saved = false) }
    fun onAnonymizeChange(value: Boolean) = _state.update { it.copy(anonymizeClients = value, saved = false) }
    fun onRetentionChange(value: String) = _state.update {
        it.copy(retentionDays = value.filter(Char::isDigit), saved = false)
    }
    fun onTopDomainsChange(value: String) = _state.update {
        it.copy(topDomains = value.filter(Char::isDigit), saved = false)
    }
    fun onTopClientsChange(value: String) = _state.update {
        it.copy(topClients = value.filter(Char::isDigit), saved = false)
    }
    fun onExcludeClientsChange(value: String) = _state.update { it.copy(excludeClients = value, saved = false) }

    fun save() {
        val s = _state.value
        val retention = s.retentionDays.toIntOrNull()
        val topDomains = s.topDomains.toIntOrNull()
        val topClients = s.topClients.toIntOrNull()
        val excluded = s.excludeClients
            .split('\n')
            .map { it.trim() }
            .filter { it.isNotEmpty() }
        val error = validate(retention, topDomains, topClients, excluded.size)
        if (error != null) {
            _state.update { it.copy(error = error, saved = false) }
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(isSaving = true, error = null, saved = false) }
            try {
                val response = repository.saveSettings(
                    StatsSettings(
                        enabled = s.enabled,
                        retentionDays = retention!!,
                        topDomains = topDomains!!,
                        topClients = topClients!!,
                        anonymizeClients = s.anonymizeClients,
                        excludeClients = excluded,
                    ),
                )
                _state.update {
                    it.apply(response.settings).copy(
                        isSaving = false,
                        saved = response.success,
                        unsupported = false,
                        error = null,
                    )
                }
            } catch (e: ApiException) {
                _state.update {
                    if (e.httpStatus == HTTP_NOT_FOUND || e.httpStatus == HTTP_UNAVAILABLE) {
                        it.copy(isSaving = false, unsupported = true)
                    } else {
                        it.copy(isSaving = false, error = e.userMessage)
                    }
                }
            } catch (e: IOException) {
                _state.update { it.copy(isSaving = false, error = e.message ?: "Could not save history settings.") }
            }
        }
    }
}

private fun validate(retention: Int?, topDomains: Int?, topClients: Int?, excluded: Int): String? = when {
    retention == null || retention !in MIN_RETENTION_DAYS..MAX_RETENTION_DAYS ->
        "Retention must be between $MIN_RETENTION_DAYS and $MAX_RETENTION_DAYS days."
    topDomains == null || topDomains !in MIN_TOP_DOMAINS..MAX_TOP_DOMAINS ->
        "Top domains must be between $MIN_TOP_DOMAINS and $MAX_TOP_DOMAINS."
    topClients == null || topClients !in MIN_TOP_CLIENTS..MAX_TOP_CLIENTS ->
        "Top clients must be between $MIN_TOP_CLIENTS and $MAX_TOP_CLIENTS."
    excluded > MAX_EXCLUDED_CLIENTS ->
        "Excluded clients cannot contain more than $MAX_EXCLUDED_CLIENTS entries."
    else -> null
}

private fun HistorySettingsUiState.apply(settings: StatsSettings): HistorySettingsUiState = copy(
    enabled = settings.enabled,
    anonymizeClients = settings.anonymizeClients,
    retentionDays = settings.retentionDays.toString(),
    topDomains = settings.topDomains.toString(),
    topClients = settings.topClients.toString(),
    excludeClients = settings.excludeClients.orEmpty().joinToString("\n"),
)
