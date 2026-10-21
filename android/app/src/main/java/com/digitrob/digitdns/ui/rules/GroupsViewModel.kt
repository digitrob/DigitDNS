package com.digitrob.digitdns.ui.rules

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.data.GroupsRepository
import com.digitrob.digitdns.data.PendingGroup
import com.digitrob.digitdns.domain.model.CustomGroup
import com.digitrob.digitdns.domain.model.GroupAction
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import javax.inject.Inject

/**
 * Groups list plus the editor sheet's draft. The draft lives here rather than in the
 * composable so a rotation does not lose a half-typed domain set, and so "is this a
 * create or an edit" is one nullable id in one place.
 */
data class GroupsUiState(
    val groups: List<CustomGroup> = emptyList(),
    val isLoading: Boolean = false,
    /**
     * The pull-to-refresh indicator's own flag.
     *
     * [isLoading] is only raised for a first load with nothing to show, so binding
     * the pull indicator to it meant the arc never animated on a screen that already
     * had rows.
     */
    val isRefreshing: Boolean = false,
    val error: String? = null,
    val editorOpen: Boolean = false,
    /** The id being edited, or null when the editor is creating a new group. */
    val editingId: String? = null,
    val draftName: String = "",
    val draftAction: GroupAction = GroupAction.Proxy,
    val draftDomains: String = "",
    val draftEnabled: Boolean = true,
    val isSaving: Boolean = false,
    val draftError: String? = null,
)

@HiltViewModel
class GroupsViewModel @Inject constructor(
    private val repository: GroupsRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(GroupsUiState())
    val state: StateFlow<GroupsUiState> = _state.asStateFlow()

    init {
        refresh()
    }

    fun refresh() {
        viewModelScope.launch {
            _state.update { it.copy(isRefreshing = true) }
            // The full-screen spinner is only for a first load with nothing to show;
            // a pull over existing rows must not replace them.
            if (_state.value.groups.isEmpty()) _state.update { it.copy(isLoading = true) }
            try {
                val groups = repository.list()
                _state.update { it.copy(groups = groups, isLoading = false, error = null) }
            } catch (e: ApiException) {
                _state.update { it.copy(isLoading = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isLoading = false, error = e.message ?: "Could not load groups.") }
            } finally {
                // Cleared whatever happened, so a failed load cannot leave the pull
                // indicator spinning.
                _state.update { it.copy(isRefreshing = false) }
            }
        }
    }

    fun openCreate() = _state.update {
        it.copy(
            editorOpen = true,
            editingId = null,
            draftName = "",
            draftAction = GroupAction.Proxy,
            draftDomains = "",
            draftEnabled = true,
            draftError = null,
        )
    }

    fun openEdit(group: CustomGroup) = _state.update {
        it.copy(
            editorOpen = true,
            editingId = group.id,
            draftName = group.name,
            draftAction = group.action,
            draftDomains = group.domains.joinToString("\n"),
            draftEnabled = group.enabled,
            draftError = null,
        )
    }

    fun closeEditor() = _state.update { it.copy(editorOpen = false) }

    fun onNameChange(v: String) = _state.update { it.copy(draftName = v) }
    fun onActionChange(v: GroupAction) = _state.update { it.copy(draftAction = v) }
    fun onDomainsChange(v: String) = _state.update { it.copy(draftDomains = v) }
    fun onEnabledChange(v: Boolean) = _state.update { it.copy(draftEnabled = v) }

    fun saveDraft() {
        val s = _state.value
        val domains = s.draftDomains.split('\n', ',', ' ').map { it.trim() }.filter { it.isNotEmpty() }.distinct()
        if (s.draftName.isBlank()) {
            _state.update { it.copy(draftError = "A name is required.") }
            return
        }
        if (domains.isEmpty()) {
            _state.update { it.copy(draftError = "At least one domain is required.") }
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(isSaving = true, draftError = null) }
            try {
                val pending = PendingGroup(
                    name = s.draftName.trim(),
                    action = s.draftAction,
                    domains = domains,
                    enabled = s.draftEnabled,
                )
                if (s.editingId == null) {
                    repository.create(pending)
                } else {
                    repository.update(s.editingId, pending)
                }
                _state.update { it.copy(isSaving = false, editorOpen = false) }
                refresh()
            } catch (e: ApiException) {
                _state.update { it.copy(isSaving = false, draftError = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isSaving = false, draftError = e.message ?: "Could not save the group.") }
            }
        }
    }

    fun delete(id: String) {
        viewModelScope.launch {
            try {
                repository.delete(id)
                _state.update { it.copy(groups = it.groups.filterNot { g -> g.id == id }) }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not delete the group.") }
            }
        }
    }
}
