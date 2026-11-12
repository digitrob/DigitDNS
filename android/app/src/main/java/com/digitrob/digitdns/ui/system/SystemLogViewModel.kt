package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.SystemLogApi
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import javax.inject.Inject

data class SystemLogUiState(
    val lines: List<String> = emptyList(),
    val isLoading: Boolean = false,
    val error: String? = null,
) {
    val text: String get() = lines.joinToString("\n")
}

@HiltViewModel
class SystemLogViewModel @Inject constructor(
    private val api: SystemLogApi,
) : ViewModel() {
    private val _state = MutableStateFlow(SystemLogUiState())
    val state: StateFlow<SystemLogUiState> = _state.asStateFlow()

    init {
        refresh()
    }

    fun refresh() {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = true, error = null) }
            try {
                val response = api.tail(2000)
                _state.update { it.copy(lines = response.lines, isLoading = false) }
            } catch (e: ApiException) {
                _state.update { it.copy(isLoading = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update {
                    it.copy(isLoading = false, error = e.message ?: "Could not load the system log.")
                }
            }
        }
    }
}
