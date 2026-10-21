package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.data.SettingsRepository
import com.digitrob.digitdns.data.TwoFactorRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import javax.inject.Inject

/** Which step of the enrolment the screen is on. */
enum class EnrolStep { Idle, AwaitingConfirmation, Done }

data class SecurityUiState(
    val totpEnabled: Boolean = false,
    val totpConfigured: Boolean = false,
    val isLoading: Boolean = false,
    val error: String? = null,
    val message: String? = null,
    val username: String = "",
    val currentPassword: String = "",
    val code: String = "",
    val enrolmentSecret: String = "",
    val enrolmentUri: String = "",
    val step: EnrolStep = EnrolStep.Idle,
    val working: Boolean = false,
    val apiKey: String = "",
    val adminPath: String = "",
)

/**
 * Two-factor enrolment. The secret and URI exist only between [beginEnrolment] and a
 * successful [confirmEnrolment]/[cancel] — the status route never carries them, so
 * they are held in screen state and discarded when the flow ends.
 *
 * Enabling invalidates every session server-side, so the app re-derives its session on
 * the next call rather than assuming the current one survives.
 */
@HiltViewModel
class SecurityViewModel @Inject constructor(
    private val repository: TwoFactorRepository,
    private val settingsRepository: SettingsRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(SecurityUiState())
    val state: StateFlow<SecurityUiState> = _state.asStateFlow()

    init {
        loadStatus()
    }

    fun loadStatus() {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = true, error = null) }
            try {
                val status = repository.status()
                val server = runCatching { settingsRepository.load().server }.getOrNull()
                _state.update {
                    it.copy(
                        totpEnabled = status.totpEnabled,
                        totpConfigured = status.totpConfigured,
                        apiKey = server?.apiKey.orEmpty(),
                        adminPath = server?.adminPath.orEmpty(),
                        isLoading = false,
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(isLoading = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(isLoading = false, error = e.message ?: "Could not load 2FA status.") }
            }
        }
    }

    fun onUsernameChange(v: String) = _state.update { it.copy(username = v) }
    fun onPasswordChange(v: String) = _state.update { it.copy(currentPassword = v) }
    fun onCodeChange(v: String) = _state.update { it.copy(code = v.filter { c -> c.isDigit() }) }
    fun dismissMessage() = _state.update { it.copy(message = null) }

    fun beginEnrolment() {
        val s = _state.value
        viewModelScope.launch {
            _state.update { it.copy(working = true, error = null) }
            try {
                val enrolment = repository.startEnrolment(s.username.trim(), s.currentPassword)
                _state.update {
                    it.copy(
                        working = false,
                        step = EnrolStep.AwaitingConfirmation,
                        enrolmentSecret = enrolment.secret,
                        enrolmentUri = enrolment.otpauthUri,
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(working = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(working = false, error = e.message ?: "Could not start 2FA setup.") }
            }
        }
    }

    fun confirmEnrolment() {
        val s = _state.value
        viewModelScope.launch {
            _state.update { it.copy(working = true, error = null) }
            try {
                repository.enable(s.username.trim(), s.currentPassword, s.code)
                _state.update {
                    it.copy(
                        working = false,
                        step = EnrolStep.Done,
                        totpEnabled = true,
                        totpConfigured = true,
                        enrolmentSecret = "",
                        enrolmentUri = "",
                        currentPassword = "",
                        code = "",
                        message = "Two-factor is on. Every other session was signed out.",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(working = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(working = false, error = e.message ?: "Could not enable 2FA.") }
            }
        }
    }

    fun cancelEnrolment() = _state.update {
        it.copy(step = EnrolStep.Idle, enrolmentSecret = "", enrolmentUri = "", code = "", error = null)
    }

    fun disable() {
        val s = _state.value
        viewModelScope.launch {
            _state.update { it.copy(working = true, error = null) }
            try {
                repository.disable(s.username.trim(), s.currentPassword, s.code)
                _state.update {
                    it.copy(
                        working = false,
                        totpEnabled = false,
                        totpConfigured = false,
                        currentPassword = "",
                        code = "",
                        message = "Two-factor is off.",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(working = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(working = false, error = e.message ?: "Could not disable 2FA.") }
            }
        }
    }
}
