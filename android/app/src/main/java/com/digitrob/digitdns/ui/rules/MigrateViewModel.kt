@file:Suppress("TooGenericExceptionCaught")

package com.digitrob.digitdns.ui.rules

import android.net.Uri
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.MigrateApplyResultDto
import com.digitrob.digitdns.core.network.MigratePreviewDto
import com.digitrob.digitdns.data.MigrateRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import javax.inject.Inject

data class MigrateUiState(
    val fileName: String = "",
    val analyzing: Boolean = false,
    val applying: Boolean = false,
    val undoing: Boolean = false,
    val savingReport: Boolean = false,
    val preview: MigratePreviewDto? = null,
    val result: MigrateApplyResultDto? = null,
    val undoToken: String = "",
    val upstreamsOptIn: Boolean = false,
    val error: String? = null,
    val message: String? = null,
)

/**
 * The Android importer's state machine. Analyze and apply are separate calls on
 * purpose: the operator sees the normalized counts and skipped items before any
 * live rule, client, list or resolver is touched.
 */
@HiltViewModel
class MigrateViewModel @Inject constructor(
    private val repository: MigrateRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(MigrateUiState())
    val state: StateFlow<MigrateUiState> = _state.asStateFlow()

    fun analyze(uri: Uri, fileName: String) {
        viewModelScope.launch {
            _state.update {
                it.copy(
                    fileName = fileName,
                    analyzing = true,
                    preview = null,
                    result = null,
                    undoToken = "",
                    error = null,
                    message = null,
                )
            }
            try {
                val preview = repository.analyze(uri)
                _state.update {
                    it.copy(
                        analyzing = false,
                        preview = preview,
                        upstreamsOptIn = preview.upstreamsOptIn,
                    )
                }
            } catch (e: Throwable) {
                _state.update { it.copy(analyzing = false, error = errorText(e)) }
            }
        }
    }

    fun setUpstreamsOptIn(value: Boolean) {
        _state.update { it.copy(upstreamsOptIn = value) }
    }

    fun apply() {
        val preview = _state.value.preview ?: return
        viewModelScope.launch {
            _state.update { it.copy(applying = true, error = null, message = null) }
            try {
                val result = repository.apply(preview.token, _state.value.upstreamsOptIn)
                _state.update {
                    it.copy(
                        applying = false,
                        preview = null,
                        result = result,
                        undoToken = result.undoToken,
                        message = "Import applied",
                    )
                }
            } catch (e: Throwable) {
                _state.update { it.copy(applying = false, error = errorText(e)) }
            }
        }
    }

    fun saveReport(uri: Uri) {
        val token = _state.value.preview?.token.orEmpty()
        if (token.isEmpty()) return
        viewModelScope.launch {
            _state.update { it.copy(savingReport = true, error = null, message = null) }
            try {
                repository.saveReport(uri, token)
                _state.update { it.copy(savingReport = false, message = "Report saved") }
            } catch (e: Throwable) {
                _state.update { it.copy(savingReport = false, error = errorText(e)) }
            }
        }
    }

    fun undo() {
        val token = _state.value.undoToken
        if (token.isEmpty()) return
        viewModelScope.launch {
            _state.update { it.copy(undoing = true, error = null, message = null) }
            try {
                repository.undo(token)
                _state.update {
                    it.copy(
                        undoing = false,
                        result = null,
                        preview = null,
                        undoToken = "",
                        message = "Import undone",
                    )
                }
            } catch (e: Throwable) {
                _state.update { it.copy(undoing = false, error = errorText(e)) }
            }
        }
    }

    fun reset() {
        _state.value = MigrateUiState()
    }

    fun dismissMessage() = _state.update { it.copy(message = null) }
    fun dismissError() = _state.update { it.copy(error = null) }

    private fun errorText(e: Throwable): String {
        val message = e.message?.trim().orEmpty()
        return message.ifEmpty { "The server could not complete that request." }
    }
}
