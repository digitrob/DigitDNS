package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.QueryLogItemDto
import com.digitrob.digitdns.core.network.QueryLogSettingsDto
import com.digitrob.digitdns.data.QueryLogRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import javax.inject.Inject

data class QueryLogUiState(
    val settings: QueryLogSettingsDto = QueryLogSettingsDto(),
    val items: List<QueryLogItemDto> = emptyList(),
    val domain: String = "",
    val client: String = "",
    val actions: Set<String> = emptySet(),
    val protos: Set<String> = emptySet(),
    val isLoading: Boolean = false,
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

    init { loadSettings() }

    fun loadSettings() {
        viewModelScope.launch {
            try {
                val settings = repository.settings()
                _state.update { it.copy(settings = settings) }
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
    fun setDomain(v: String) = _state.update { it.copy(domain = v) }
    fun setClient(v: String) = _state.update { it.copy(client = v) }
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
                val settings = repository.save(_state.value.settings)
                _state.update { it.copy(settings = settings, isSaving = false, message = "Query log settings saved.") }
            } catch (e: ApiException) {
                _state.update { it.copy(isSaving = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isSaving = false, error = e.message ?: "Could not save query log settings.") }
            }
        }
    }

    fun search() {
        val s = _state.value
        viewModelScope.launch {
            _state.update { it.copy(isLoading = true, error = null) }
            try {
                val items = repository.search(s.domain, s.client, s.actions.joinToString(","), s.protos.joinToString(","))
                _state.update { it.copy(items = items, isLoading = false) }
            } catch (e: ApiException) {
                _state.update { it.copy(isLoading = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isLoading = false, error = e.message ?: "Could not search the query log.") }
            }
        }
    }

    fun clear() {
        viewModelScope.launch {
            try {
                repository.clear()
                _state.update { it.copy(items = emptyList(), message = "Query history cleared.") }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not clear query history.") }
            }
        }
    }
}
