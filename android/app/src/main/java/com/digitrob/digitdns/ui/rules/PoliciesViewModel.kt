package com.digitrob.digitdns.ui.rules

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.data.PoliciesRepository
import com.digitrob.digitdns.domain.model.Policy
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import javax.inject.Inject

data class PoliciesUiState(
    val policies: List<Policy> = emptyList(),
    val isLoading: Boolean = false,
    val error: String? = null,
    val editorOpen: Boolean = false,
    /** True when the open editor is editing an existing key rather than creating one. */
    val editingExisting: Boolean = false,
    val draftKey: String = "",
    val draftName: String = "",
    val draftDomains: String = "",
    val draftEnabled: Boolean = true,
    val isSaving: Boolean = false,
    /** A key whose enable toggle is in flight, so its switch can show as busy. */
    val pendingKey: String? = null,
    val draftError: String? = null,
)

/**
 * The stored policy records. The five built-in rule keys show up here too — they are
 * the same rows the rules screen writes — so any row can be edited (name, domains,
 * enabled) and switched on or off. The v3 surface has no delete: a built-in row cannot
 * be removed, so "off" is how a record is taken out of circulation.
 */
@HiltViewModel
class PoliciesViewModel @Inject constructor(
    private val repository: PoliciesRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(PoliciesUiState())
    val state: StateFlow<PoliciesUiState> = _state.asStateFlow()

    init {
        refresh()
    }

    fun refresh() {
        viewModelScope.launch {
            if (_state.value.policies.isEmpty()) _state.update { it.copy(isLoading = true) }
            try {
                val policies = repository.list()
                _state.update { it.copy(policies = policies, isLoading = false, error = null) }
            } catch (e: ApiException) {
                _state.update { it.copy(isLoading = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isLoading = false, error = e.message ?: "Could not load policies.") }
            }
        }
    }

    fun openCreate() = _state.update {
        it.copy(
            editorOpen = true,
            editingExisting = false,
            draftKey = "",
            draftName = "",
            draftDomains = "",
            draftEnabled = true,
            draftError = null,
        )
    }

    /** Open the editor pre-filled for an existing policy. */
    fun openEdit(policy: Policy) = _state.update {
        it.copy(
            editorOpen = true,
            editingExisting = true,
            draftKey = policy.key,
            draftName = policy.name,
            draftDomains = policy.customDomains.joinToString("\n"),
            draftEnabled = policy.enabled,
            draftError = null,
        )
    }

    fun closeEditor() = _state.update { it.copy(editorOpen = false) }

    fun onKeyChange(v: String) = _state.update { it.copy(draftKey = v) }
    fun onNameChange(v: String) = _state.update { it.copy(draftName = v) }
    fun onDomainsChange(v: String) = _state.update { it.copy(draftDomains = v) }
    fun onEnabledChange(v: Boolean) = _state.update { it.copy(draftEnabled = v) }

    /** Flip a policy's enabled flag, writing the whole record back (POST is an upsert). */
    fun toggleEnabled(policy: Policy) {
        viewModelScope.launch {
            _state.update { it.copy(pendingKey = policy.key, error = null) }
            try {
                repository.save(policy.copy(enabled = !policy.enabled))
                _state.update { it.copy(pendingKey = null) }
                refresh()
            } catch (e: ApiException) {
                _state.update { it.copy(pendingKey = null, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(pendingKey = null, error = e.message ?: "Could not update the policy.") }
            }
        }
    }

    fun saveDraft() {
        val s = _state.value
        val key = s.draftKey.trim()
        if (key.isEmpty()) {
            _state.update { it.copy(draftError = "A key is required.") }
            return
        }
        val domains = s.draftDomains.split('\n', ',', ' ').map { it.trim() }.filter { it.isNotEmpty() }.distinct()
        val category = s.policies.firstOrNull { it.key == key }?.category ?: "custom"
        viewModelScope.launch {
            _state.update { it.copy(isSaving = true, draftError = null) }
            try {
                repository.save(
                    Policy(
                        key = key,
                        name = s.draftName.trim(),
                        category = category,
                        enabled = s.draftEnabled,
                        customDomains = domains,
                    ),
                )
                _state.update { it.copy(isSaving = false, editorOpen = false) }
                refresh()
            } catch (e: ApiException) {
                _state.update { it.copy(isSaving = false, draftError = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isSaving = false, draftError = e.message ?: "Could not save the policy.") }
            }
        }
    }
}
