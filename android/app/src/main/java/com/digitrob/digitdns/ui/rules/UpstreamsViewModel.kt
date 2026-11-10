package com.digitrob.digitdns.ui.rules

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.UpstreamPresetProvider
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
    val presets: List<UpstreamPresetProvider> = emptyList(),
    val presetsOpen: Boolean = false,
    val presetsLoading: Boolean = false,
    val presetsLoaded: Boolean = false,
    val presetError: String? = null,
    val expandedPresetProviders: Set<String> = emptySet(),
    val addingPresetAddress: String? = null,
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
    fun dismissMessage() = _state.update { it.copy(message = null) }

    /** Open the inline provider dropdown; fetch the catalogue only once. */
    fun openPresets() {
        _state.update { it.copy(presetsOpen = true) }
        if (!_state.value.presetsLoaded) loadPresets()
    }

    fun closePresets() = _state.update { it.copy(presetsOpen = false) }

    fun togglePresets() {
        if (_state.value.presetsOpen) closePresets() else openPresets()
    }

    fun togglePresetProvider(id: String) {
        _state.update { current ->
            val expanded = current.expandedPresetProviders
            current.copy(
                expandedPresetProviders = if (expanded.contains(id)) expanded - id else expanded + id,
            )
        }
    }

    /** Fetch the curated catalogue. A failure leaves custom entry working. */
    fun loadPresets() {
        if (_state.value.presetsLoading) return
        _state.update { it.copy(presetsLoading = true, presetError = null) }
        viewModelScope.launch {
            try {
                val providers = repository.presets()
                _state.update {
                    it.copy(
                        presets = providers,
                        presetsLoading = false,
                        presetsLoaded = true,
                        expandedPresetProviders = providers.firstOrNull()
                            ?.let { first -> setOf(first.id) } ?: emptySet(),
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(presetsLoading = false, presetsLoaded = true, presetError = e.userMessage) }
            } catch (e: IOException) {
                _state.update {
                    it.copy(
                        presetsLoading = false,
                        presetsLoaded = true,
                        presetError = e.message ?: "Could not load the provider list.",
                    )
                }
            }
        }
    }

    /** Add one endpoint from the provider browser, keeping the browser open. */
    fun addPreset(address: String) {
        if (address.isBlank() || _state.value.addingPresetAddress != null) return
        viewModelScope.launch {
            _state.update { it.copy(addingPresetAddress = address, error = null) }
            try {
                val list = repository.add(address)
                _state.update {
                    it.copy(
                        upstreams = list,
                        addingPresetAddress = null,
                        message = "Added $address.",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(addingPresetAddress = null, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update {
                    it.copy(
                        addingPresetAddress = null,
                        error = e.message ?: "Could not add the upstream.",
                    )
                }
            }
        }
    }

    fun add() {
        val address = _state.value.newAddress.trim()
        if (address.isEmpty()) return
        viewModelScope.launch {
            _state.update { it.copy(isSaving = true, error = null) }
            try {
                val list = repository.add(address)
                _state.update {
                    it.copy(
                        upstreams = list,
                        isSaving = false,
                        newAddress = "",
                        presetsOpen = false,
                        message = "Added $address.",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(isSaving = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isSaving = false, error = e.message ?: "Could not add the upstream.") }
            }
        }
    }

    /** Remove one address from the pool. The UI confirms before this is ever called. */
    fun remove(address: String) {
        viewModelScope.launch {
            try {
                val list = repository.remove(address)
                _state.update { it.copy(upstreams = list, message = "Removed $address.") }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not remove the upstream.") }
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
        _state.update { it.copy(upstreams = reordered) }
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
