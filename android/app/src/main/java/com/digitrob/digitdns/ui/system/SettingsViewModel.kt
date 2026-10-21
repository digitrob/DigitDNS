package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.AuthState
import com.digitrob.digitdns.data.ConnectionStore
import com.digitrob.digitdns.data.SettingsRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import javax.inject.Inject

data class SettingsUiState(
    val publicIp: String = "",
    val apiBind: String = "",
    val apiBindList: String = "",
    val sessionIdleMinutes: String = "",
    val adminUsername: String = "",
    val adminPath: String = "",
    val apiKey: String = "",
    val bindHost: String = "",
    val webPort: Int = 0,
    val adminPasswordWeak: Boolean = false,
    val tlsDomain: String = "",
    val tlsPanelHttps: Boolean = false,
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    val error: String? = null,
    val savedMessage: String? = null,
    // Regenerate key / path dialogs.
    val regenKeyOpen: Boolean = false,
    val regenPathOpen: Boolean = false,
    val currentPassword: String = "",
    val totpCode: String = "",
    val regenerating: Boolean = false,
    val regeneratedApiKey: String? = null,
    val regeneratedAdminPath: String? = null,
)

/**
 * Server settings, including the two credential rotations.
 *
 * The rotations are destructive and handled with care: regenerating the admin path
 * invalidates every session, so on success the new path is written to the connection
 * store immediately, or the app would keep talking to a path the server has retired.
 */
@HiltViewModel
class SettingsViewModel @Inject constructor(
    private val repository: SettingsRepository,
    private val authState: AuthState,
    private val connectionStore: ConnectionStore,
) : ViewModel() {

    private val _state = MutableStateFlow(SettingsUiState())
    val state: StateFlow<SettingsUiState> = _state.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = true, error = null) }
            try {
                val bundle = repository.load()
                val server = bundle.server
                val tls = bundle.tls
                _state.update {
                    it.copy(
                        publicIp = server?.publicIp.orEmpty(),
                        apiBind = server?.apiBind.orEmpty(),
                        apiBindList = server?.apiBindList?.joinToString("\n").orEmpty(),
                        sessionIdleMinutes = server?.sessionIdleMinutes?.toString().orEmpty(),
                        adminUsername = server?.adminUsername.orEmpty(),
                        adminPath = server?.adminPath.orEmpty(),
                        apiKey = server?.apiKey.orEmpty(),
                        bindHost = server?.bindHost.orEmpty(),
                        webPort = server?.webPort ?: 0,
                        adminPasswordWeak = server?.adminPasswordWeak == true,
                        tlsDomain = tls?.domain.orEmpty(),
                        tlsPanelHttps = tls?.panelHttps == true,
                        isLoading = false,
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(isLoading = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isLoading = false, error = e.message ?: "Could not load settings.") }
            }
        }
    }

    fun onPublicIpChange(v: String) = _state.update { it.copy(publicIp = v) }
    fun onApiBindChange(v: String) = _state.update { it.copy(apiBind = v) }
    fun onApiBindListChange(v: String) = _state.update { it.copy(apiBindList = v) }
    fun onSessionIdleChange(v: String) = _state.update { it.copy(sessionIdleMinutes = v.filter { c -> c.isDigit() }) }
    fun dismissMessage() = _state.update { it.copy(savedMessage = null) }

    fun save() {
        val s = _state.value
        viewModelScope.launch {
            _state.update { it.copy(isSaving = true, error = null) }
            try {
                val saved = repository.save(
                    publicIp = s.publicIp.trim(),
                    apiBind = s.apiBind.trim(),
                    apiBindList = parseLines(s.apiBindList),
                    sessionIdleMinutes = s.sessionIdleMinutes.toIntOrNull(),
                )
                _state.update {
                    it.copy(
                        isSaving = false,
                        savedMessage = "Settings saved.",
                        apiBind = saved.apiBind,
                        apiBindList = saved.apiBindList.joinToString("\n"),
                        sessionIdleMinutes = saved.sessionIdleMinutes.toString(),
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(isSaving = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isSaving = false, error = e.message ?: "Could not save settings.") }
            }
        }
    }

    fun openRegenKey() = _state.update { it.copy(regenKeyOpen = true, currentPassword = "", totpCode = "", error = null) }
    fun openRegenPath() = _state.update { it.copy(regenPathOpen = true, currentPassword = "", totpCode = "", error = null) }
    fun closeRegen() = _state.update { it.copy(regenKeyOpen = false, regenPathOpen = false) }
    fun onCurrentPasswordChange(v: String) = _state.update { it.copy(currentPassword = v) }
    fun onTotpCodeChange(v: String) = _state.update { it.copy(totpCode = v.filter { c -> c.isDigit() }) }

    fun regenerateApiKey() {
        val s = _state.value
        viewModelScope.launch {
            _state.update { it.copy(regenerating = true, error = null) }
            try {
                val newKey = repository.regenerateApiKey(s.currentPassword, s.totpCode.takeIf { it.isNotBlank() })
                authState.updateApiKey(newKey)
                _state.update {
                    it.copy(
                        regenerating = false,
                        regenKeyOpen = false,
                        apiKey = newKey,
                        regeneratedApiKey = newKey,
                        currentPassword = "",
                        totpCode = "",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(regenerating = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(regenerating = false, error = e.message ?: "Could not regenerate the API key.") }
            }
        }
    }

    fun regenerateAdminPath() {
        val s = _state.value
        viewModelScope.launch {
            _state.update { it.copy(regenerating = true, error = null) }
            try {
                val newPath = repository.regenerateAdminPath(confirm = true)
                // The server deleted every session; the app must adopt the new path
                // at once or its next call would hit a retired URL.
                connectionStore.updateAdminPath(newPath)
                _state.update {
                    it.copy(
                        regenerating = false,
                        regenPathOpen = false,
                        adminPath = newPath,
                        regeneratedAdminPath = newPath,
                        currentPassword = "",
                        totpCode = "",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(regenerating = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(regenerating = false, error = e.message ?: "Could not regenerate the admin path.") }
            }
        }
    }

    fun dismissRegenerated() = _state.update { it.copy(regeneratedApiKey = null, regeneratedAdminPath = null) }

    private fun parseLines(text: String): List<String> =
        text.split('\n', ',', ' ').map { it.trim() }.filter { it.isNotEmpty() }.distinct()
}
