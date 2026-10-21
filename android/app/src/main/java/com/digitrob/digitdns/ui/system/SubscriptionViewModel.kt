package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.SubscriptionSaveRequest
import com.digitrob.digitdns.data.SubscriptionRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import javax.inject.Inject

data class SubscriptionUiState(
    val enabled: Boolean = false,
    val domain: String = "",
    val port: String = "",
    val uriPath: String = "/sub",
    val title: String = "",
    val themeCssSource: String = "inline",
    val themeCssPath: String = "",
    val themeCssUrl: String = "",
    val themeCss: String = "",
    val usePanelCertificate: Boolean = true,
    val autoRenewAcme: Boolean = true,
    val origin: String = "",
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    val error: String? = null,
    val savedMessage: String? = null,
)

/**
 * The subscriber portal. `uri_path` is fixed at `/sub` by the server, so it is shown
 * but not edited; the theme source picks which of the three theme inputs is sent.
 */
@HiltViewModel
class SubscriptionViewModel @Inject constructor(
    private val repository: SubscriptionRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(SubscriptionUiState())
    val state: StateFlow<SubscriptionUiState> = _state.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = true, error = null) }
            try {
                val s = repository.load()
                _state.update {
                    it.copy(
                        enabled = s.enabled,
                        domain = s.domain,
                        port = s.port.takeIf { p -> p > 0 }?.toString().orEmpty(),
                        uriPath = s.uriPath.ifBlank { "/sub" },
                        title = s.title,
                        themeCssSource = s.themeCssSource.ifBlank { "inline" },
                        themeCssPath = s.themeCssPath,
                        themeCssUrl = s.themeCssUrl,
                        themeCss = s.themeCss,
                        usePanelCertificate = s.usePanelCertificate,
                        autoRenewAcme = s.autoRenewAcme,
                        isLoading = false,
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(isLoading = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isLoading = false, error = e.message ?: "Could not load subscription settings.") }
            }
        }
    }

    fun onEnabledChange(v: Boolean) = _state.update { it.copy(enabled = v) }
    fun onDomainChange(v: String) = _state.update { it.copy(domain = v) }
    fun onPortChange(v: String) = _state.update { it.copy(port = v.filter { c -> c.isDigit() }) }
    fun onTitleChange(v: String) = _state.update { it.copy(title = v) }
    fun onThemeSourceChange(v: String) = _state.update { it.copy(themeCssSource = v) }
    fun onThemePathChange(v: String) = _state.update { it.copy(themeCssPath = v) }
    fun onThemeUrlChange(v: String) = _state.update { it.copy(themeCssUrl = v) }
    fun onThemeCssChange(v: String) = _state.update { it.copy(themeCss = v) }
    fun onAutoRenewChange(v: Boolean) = _state.update { it.copy(autoRenewAcme = v) }
    fun dismissMessage() = _state.update { it.copy(savedMessage = null) }

    fun save() {
        val s = _state.value
        viewModelScope.launch {
            _state.update { it.copy(isSaving = true, error = null) }
            try {
                val result = repository.save(
                    SubscriptionSaveRequest(
                        enabled = s.enabled,
                        domain = s.domain.trim(),
                        port = s.port.toIntOrNull() ?: 0,
                        uriPath = s.uriPath.ifBlank { "/sub" },
                        title = s.title.trim(),
                        themeCss = if (s.themeCssSource == "inline") s.themeCss else "",
                        themeCssSource = s.themeCssSource,
                        themeCssPath = if (s.themeCssSource == "local") s.themeCssPath.trim() else "",
                        themeCssUrl = if (s.themeCssSource == "url") s.themeCssUrl.trim() else "",
                        autoRenewAcme = s.autoRenewAcme,
                    ),
                )
                _state.update {
                    it.copy(
                        isSaving = false,
                        origin = result.origin,
                        savedMessage = if (result.listenerError.isBlank()) {
                            "Subscription settings saved."
                        } else {
                            "Saved, but the portal listener reported: ${result.listenerError}"
                        },
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(isSaving = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isSaving = false, error = e.message ?: "Could not save subscription settings.") }
            }
        }
    }
}
