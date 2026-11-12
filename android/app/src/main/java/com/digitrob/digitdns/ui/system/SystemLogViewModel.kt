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
    /** First load / background fetch; drives the empty-state spinner only. */
    val isLoading: Boolean = false,
    /** True only while the operator pressed Refresh, so auto-refresh never animates the button. */
    val isManualRefreshing: Boolean = false,
    val error: String? = null,
) {
    val text: String get() = lines.joinToString("\n")
}

private const val SYSTEM_LOG_LIMIT = 2000

@HiltViewModel
class SystemLogViewModel @Inject constructor(
    private val api: SystemLogApi,
) : ViewModel() {
    private val _state = MutableStateFlow(SystemLogUiState())
    val state: StateFlow<SystemLogUiState> = _state.asStateFlow()

    init {
        refresh()
    }

    fun refresh(manual: Boolean = false) {
        if (_state.value.isLoading || _state.value.isManualRefreshing) return
        viewModelScope.launch {
            _state.update {
                it.copy(
                    isLoading = it.lines.isEmpty(),
                    isManualRefreshing = manual,
                    error = null,
                )
            }
            try {
                val response = api.tail(SYSTEM_LOG_LIMIT)
                _state.update {
                    it.copy(
                        lines = response.lines,
                        isLoading = false,
                        isManualRefreshing = false,
                    )
                }
            } catch (e: ApiException) {
                _state.update {
                    it.copy(isLoading = false, isManualRefreshing = false, error = e.userMessage)
                }
            } catch (e: IOException) {
                _state.update {
                    it.copy(
                        isLoading = false,
                        isManualRefreshing = false,
                        error = e.message ?: "Could not load the system log.",
                    )
                }
            }
        }
    }
}
