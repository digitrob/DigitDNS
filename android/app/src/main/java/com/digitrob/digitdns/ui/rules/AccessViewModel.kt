package com.digitrob.digitdns.ui.rules

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.data.AccessRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import javax.inject.Inject

data class AccessUiState(
    val allowAll: Boolean = false,
    val allowedIps: String = "",
    val dohTokens: String = "",
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    val error: String? = null,
    val savedMessage: String? = null,
)

/**
 * The access gate. Mode and the rest are saved by different routes on purpose: the
 * mode toggle saves on the spot ([setMode]), while the allowlist and tokens save
 * together ([save]) and never touch the mode.
 */
@HiltViewModel
class AccessViewModel @Inject constructor(
    private val repository: AccessRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(AccessUiState())
    val state: StateFlow<AccessUiState> = _state.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = true, error = null) }
            try {
                val settings = repository.load()
                _state.update {
                    it.copy(
                        allowAll = settings.allowAll,
                        allowedIps = settings.allowedIps.joinToString("\n"),
                        dohTokens = settings.dohTokens.joinToString("\n"),
                        isLoading = false,
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(isLoading = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isLoading = false, error = e.message ?: "Could not load access settings.") }
            }
        }
    }

    fun onAllowAllChange(value: Boolean) {
        // Optimistic: flip the switch, then persist; revert if the server refuses.
        val previous = _state.value.allowAll
        _state.update { it.copy(allowAll = value) }
        viewModelScope.launch {
            try {
                repository.setMode(value)
            } catch (e: ApiException) {
                _state.update { it.copy(allowAll = previous, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(allowAll = previous, error = e.message ?: "Could not change the access mode.") }
            }
        }
    }

    fun onAllowedIpsChange(v: String) = _state.update { it.copy(allowedIps = v) }
    fun onDohTokensChange(v: String) = _state.update { it.copy(dohTokens = v) }
    fun dismissMessage() = _state.update { it.copy(savedMessage = null) }

    fun save() {
        viewModelScope.launch {
            _state.update { it.copy(isSaving = true, error = null) }
            try {
                repository.save(
                    allowedIps = parseLines(_state.value.allowedIps),
                    dohTokens = parseLines(_state.value.dohTokens),
                )
                _state.update { it.copy(isSaving = false, savedMessage = "Access settings saved.") }
            } catch (e: ApiException) {
                _state.update { it.copy(isSaving = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isSaving = false, error = e.message ?: "Could not save access settings.") }
            }
        }
    }

    private fun parseLines(text: String): List<String> =
        text.split('\n', ',', ' ').map { it.trim() }.filter { it.isNotEmpty() }.distinct()
}
