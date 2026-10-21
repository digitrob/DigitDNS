package com.digitrob.digitdns.ui.rules

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.data.UpstreamsRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import javax.inject.Inject

data class UpstreamsUiState(
    val upstreams: List<String> = emptyList(),
    val newAddress: String = "",
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    val error: String? = null,
    val message: String? = null,
    /** The address removed by the last delete, offered back through the Undo action. */
    val undoAddress: String? = null,
)

/**
 * The upstream pool. Reordering persists the *complete* set in the new order, because
 * the server rejects anything else; a drag move sends the whole list, exactly as the
 * pool requires. The benchmark lives on the dashboard's Upstream Servers card, so it is
 * deliberately absent here.
 */
@HiltViewModel
class UpstreamsViewModel @Inject constructor(
    private val repository: UpstreamsRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(UpstreamsUiState())
    val state: StateFlow<UpstreamsUiState> = _state.asStateFlow()

    init {
        load()
    }

    fun refresh() = load()

    fun load() {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = true, error = null) }
            try {
                val list = repository.list()
                _state.update { it.copy(upstreams = list, isLoading = false) }
            } catch (e: ApiException) {
                _state.update { it.copy(isLoading = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isLoading = false, error = e.message ?: "Could not load upstreams.") }
            }
        }
    }

    fun onNewAddressChange(v: String) = _state.update { it.copy(newAddress = v) }
    fun dismissMessage() = _state.update { it.copy(message = null, undoAddress = null) }

    fun add() {
        val address = _state.value.newAddress.trim()
        if (address.isEmpty()) return
        viewModelScope.launch {
            _state.update { it.copy(isSaving = true, error = null) }
            try {
                val list = repository.add(address)
                _state.update {
                    it.copy(upstreams = list, isSaving = false, newAddress = "", message = "Added $address.", undoAddress = null)
                }
            } catch (e: ApiException) {
                _state.update { it.copy(isSaving = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isSaving = false, error = e.message ?: "Could not add the upstream.") }
            }
        }
    }

    /** Remove one address from the pool, remembering it so Undo can put it back. */
    fun remove(address: String) {
        viewModelScope.launch {
            try {
                val list = repository.remove(address)
                _state.update { it.copy(upstreams = list, message = "Removed $address.", undoAddress = address) }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not remove the upstream.") }
            }
        }
    }

    /** Re-add the address the last delete removed. A no-op when there is nothing to undo. */
    fun undoLastDelete() {
        val address = _state.value.undoAddress ?: return
        _state.update { it.copy(undoAddress = null) }
        viewModelScope.launch {
            try {
                val list = repository.add(address)
                _state.update { it.copy(upstreams = list, message = "Restored $address.", undoAddress = null) }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not restore the upstream.") }
            }
        }
    }

    /** Move the entry at [from] to slot [to], as a drag-reorder lands it. */
    fun moveTo(from: Int, to: Int) = swap(from, to)

    private fun swap(from: Int, to: Int) {
        val list = _state.value.upstreams
        if (from == to || from !in list.indices || to !in list.indices) return
        val reordered = list.toMutableList().apply { add(to, removeAt(from)) }
        val previous = list
        // Show the new order at once, then persist; a rejected reorder reverts.
        _state.update { it.copy(upstreams = reordered, undoAddress = null) }
        viewModelScope.launch {
            try {
                val confirmed = repository.reorder(reordered)
                _state.update { it.copy(upstreams = confirmed) }
            } catch (e: ApiException) {
                _state.update { it.copy(upstreams = previous, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(upstreams = previous, error = e.message ?: "Could not reorder upstreams.") }
            }
        }
    }
}
