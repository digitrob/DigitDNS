package com.digitrob.digitdns.ui.subscriber

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.SubscriberStatusDto
import com.digitrob.digitdns.data.AppMode
import com.digitrob.digitdns.data.AppPreferences
import com.digitrob.digitdns.data.AuthRepository
import com.digitrob.digitdns.data.SubscriberRepository
import com.digitrob.digitdns.data.SubscriberResult
import com.digitrob.digitdns.data.SubscriberSession
import com.digitrob.digitdns.data.ThemeMode
import com.digitrob.digitdns.ui.auth.TrustPrompt
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import javax.inject.Inject

/** How the subscriber left the surface; the shell navigates once this is set. */
enum class SubscriberExit { SIGNED_OUT, FORGOTTEN, SWITCHED_TO_ADMIN }

data class SubscriberUiState(
    val status: SubscriberStatusDto? = null,
    val loading: Boolean = false,
    val refreshing: Boolean = false,
    val error: String? = null,
    val message: String? = null,
    val registerBusy: Boolean = false,
    val secretSaved: Boolean = true,
    val now: Long = System.currentTimeMillis(),
    val trustPrompt: TrustPrompt? = null,
    val themeMode: ThemeMode = ThemeMode.SYSTEM,
    /** True when an admin session is on this device, so "Switch to Admin" can return. */
    val adminAvailable: Boolean = false,
    val exit: SubscriberExit? = null,
)

/**
 * The subscriber surface's state. One instance backs all three tabs, so a refresh on
 * Status updates Connect and Settings without three copies of the fetch.
 */
@HiltViewModel
class SubscriberViewModel @Inject constructor(
    private val repository: SubscriberRepository,
    private val session: SubscriberSession,
    private val appPreferences: AppPreferences,
    private val authRepository: AuthRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(
        SubscriberUiState(
            secretSaved = session.hasSavedSecret(),
            adminAvailable = authRepository.hasCredential(),
        ),
    )
    val state: StateFlow<SubscriberUiState> = _state.asStateFlow()

    init {
        refresh(silent = true)
        viewModelScope.launch {
            appPreferences.themeMode.collect { mode -> _state.update { it.copy(themeMode = mode) } }
        }
        // The portal lets a subscriber leave the page open; the countdown and the
        // traffic figure stay truthful by ticking locally and polling periodically.
        viewModelScope.launch {
            while (isActive) {
                delay(COUNTDOWN_TICK_MS)
                _state.update { it.copy(now = System.currentTimeMillis()) }
            }
        }
        viewModelScope.launch {
            while (isActive) {
                delay(REFRESH_INTERVAL_MS)
                refresh(silent = true)
            }
        }
    }

    fun refresh(silent: Boolean = false) {
        val link = session.currentLink()
        if (link == null) {
            _state.update { it.copy(error = "No subscription is loaded. Sign in again.") }
            return
        }
        _state.update {
            it.copy(loading = !silent && it.status == null, refreshing = silent, error = if (silent) it.error else null)
        }
        viewModelScope.launch {
            when (val result = repository.loadStatus(link)) {
                is SubscriberResult.Ok -> {
                    // The portal names the account; keep it for the Servers list.
                    session.updateLabel(result.value.client.name)
                    _state.update {
                        it.copy(
                            status = result.value,
                            loading = false,
                            refreshing = false,
                            error = null,
                            now = System.currentTimeMillis(),
                        )
                    }
                }

                is SubscriberResult.TrustRequired -> _state.update {
                    it.copy(
                        loading = false,
                        refreshing = false,
                        trustPrompt = TrustPrompt(result.fingerprint, result.changedFrom),
                    )
                }

                is SubscriberResult.Failed -> _state.update {
                    it.copy(loading = false, refreshing = false, error = result.message)
                }
            }
        }
    }

    /** Bind this device's address (ip null) or a custom one, using the secret. */
    fun register(secret: String, ip: String? = null, saveSecret: Boolean) {
        val link = session.currentLink() ?: return
        val effective = secret.trim().ifBlank { session.secret() }
        if (effective.isBlank()) {
            _state.update { it.copy(error = "Enter the registration secret to change this device's IP.") }
            return
        }
        _state.update { it.copy(registerBusy = true, error = null) }
        viewModelScope.launch {
            when (val result = repository.register(link, effective, ip)) {
                is SubscriberResult.Ok -> {
                    session.updateSecret(effective, saveSecret || secret.isBlank())
                    val text = if (result.value.alreadyBound) {
                        "That address is already registered."
                    } else {
                        "Registered ${result.value.boundIp.ifBlank { ip ?: "this device" }}."
                    }
                    _state.update {
                        it.copy(
                            registerBusy = false,
                            message = text,
                            secretSaved = session.hasSavedSecret(),
                        )
                    }
                    refresh(silent = true)
                }

                is SubscriberResult.TrustRequired -> _state.update {
                    it.copy(
                        registerBusy = false,
                        trustPrompt = TrustPrompt(result.fingerprint, result.changedFrom),
                    )
                }

                is SubscriberResult.Failed -> _state.update {
                    it.copy(registerBusy = false, error = result.message)
                }
            }
        }
    }

    fun trustCurrentHost() {
        val prompt = _state.value.trustPrompt ?: return
        val link = session.currentLink() ?: return
        repository.trust(link.host, prompt.fingerprint)
        _state.update { it.copy(trustPrompt = null) }
        refresh(silent = true)
    }

    fun dismissTrust() = _state.update { it.copy(trustPrompt = null) }

    fun consumeMessage() = _state.update { it.copy(message = null) }

    fun setTheme(mode: ThemeMode) {
        viewModelScope.launch { appPreferences.setThemeMode(mode) }
    }

    /** The full portal link, for the browser and the copy action. */
    fun portalUrl(): String? = session.currentLink()?.portalUrl

    /** The parsed link, for the settings summary. */
    fun currentLink() = session.currentLink()

    /**
     * Return to the admin surface without touching the subscription. The link, its
     * secret and the session stay saved, so Servers can reopen it in one tap.
     */
    fun switchToAdmin() {
        viewModelScope.launch {
            appPreferences.setSubscriberActive(false)
            appPreferences.setAppMode(AppMode.ADMIN)
            _state.update { it.copy(exit = SubscriberExit.SWITCHED_TO_ADMIN) }
        }
    }

    /** Leave the subscriber surface but keep the saved link for a quick return. */
    fun signOut() {
        viewModelScope.launch {
            session.endSession()
            appPreferences.setSubscriberActive(false)
            _state.update { it.copy(exit = SubscriberExit.SIGNED_OUT) }
        }
    }

    /** Drop the link, the secret and the pin; return the app to Admin. */
    fun forget() {
        viewModelScope.launch {
            session.forget()
            appPreferences.setSubscriberActive(false)
            appPreferences.setAppMode(AppMode.ADMIN)
            _state.update { it.copy(exit = SubscriberExit.FORGOTTEN) }
        }
    }

    private companion object {
        const val COUNTDOWN_TICK_MS = 30_000L
        const val REFRESH_INTERVAL_MS = 60_000L
    }
}
