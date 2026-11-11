package com.digitrob.digitdns.ui.security

import android.os.SystemClock
import androidx.fragment.app.FragmentActivity
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.R
import com.digitrob.digitdns.core.security.BiometricAuthenticator
import com.digitrob.digitdns.data.AppMode
import com.digitrob.digitdns.data.AppPreferences
import com.digitrob.digitdns.data.AuthRepository
import com.digitrob.digitdns.data.SubscriberSession
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import javax.inject.Inject

/**
 * The app-lock state machine.
 *
 * The lock is a local gate over a credential the app already holds. It is not a
 * replacement for the server password: if the session has lapsed and no saved
 * password exists, unlocking only gets the operator to the login screen.
 *
 * Cold start locks when biometric login is on and a protected session exists.
 * Returning from the background locks after [GRACE_MS]; a quick app switch does
 * not. The ViewModel survives rotation, so a rotate does not re-prompt.
 */
@HiltViewModel
class AppLockViewModel @Inject constructor(
    private val preferences: AppPreferences,
    private val authRepository: AuthRepository,
    private val subscriberSession: SubscriberSession,
    private val biometric: BiometricAuthenticator,
) : ViewModel() {
    private val _locked = MutableStateFlow(false)
    val locked: StateFlow<Boolean> = _locked.asStateFlow()

    private val _message = MutableStateFlow<String?>(null)
    val message: StateFlow<String?> = _message.asStateFlow()

    private val _authenticating = MutableStateFlow(false)
    val authenticating: StateFlow<Boolean> = _authenticating.asStateFlow()

    private var initialized = false
    private var lastBackgroundAt = 0L

    /**
     * Called once after the activity's bootstrap has loaded the active credential.
     * Idempotent: rotation and recomposition must not re-lock an unlocked app.
     */
    fun onReady() {
        if (initialized) return
        initialized = true
        viewModelScope.launch {
            val enabled = preferences.biometricEnabled.first()
            if (enabled && hasProtectedSession()) {
                _locked.value = true
            }
        }
    }

    fun onBackground() {
        lastBackgroundAt = SystemClock.elapsedRealtime()
    }

    fun onForeground() {
        if (_locked.value) return
        viewModelScope.launch {
            val enabled = preferences.biometricEnabled.first()
            if (!enabled || !hasProtectedSession()) {
                _locked.value = false
                return@launch
            }
            val awayFor = SystemClock.elapsedRealtime() - lastBackgroundAt
            if (lastBackgroundAt > 0 && awayFor >= GRACE_MS) {
                _locked.value = true
            }
        }
    }

    fun unlock(activity: FragmentActivity) {
        if (!_locked.value) return
        if (!hasProtectedSession()) {
            _locked.value = false
            return
        }
        viewModelScope.launch {
            _authenticating.value = true
            try {
                val ok = biometric.authenticate(
                    activity,
                    activity.getString(R.string.biometric_unlock_title),
                    activity.getString(R.string.biometric_unlock_subtitle),
                )
                if (ok) {
                    _message.value = null
                    _locked.value = false
                    // The prompt can briefly pause the activity; do not let that
                    // timestamp immediately re-lock the app after a successful unlock.
                    lastBackgroundAt = 0L
                } else {
                    _message.value = activity.getString(R.string.biometric_failed)
                }
            } finally {
                _authenticating.value = false
            }
        }
    }

    /**
     * The last-resort path when biometrics are unavailable or permanently locked.
     * Signing out clears the credential and the subscriber session, then the
     * nav host starts at the login screen.
     */
    fun signOut() {
        viewModelScope.launch {
            authRepository.logout()
            subscriberSession.forget()
            preferences.setSubscriberActive(false)
            preferences.setAppMode(AppMode.ADMIN)
            _message.value = null
            _locked.value = false
        }
    }

    private fun hasProtectedSession(): Boolean =
        authRepository.hasCredential() || subscriberSession.currentLink() != null

    companion object {
        /** Returning within a minute of leaving the app does not re-prompt. */
        const val GRACE_MS = 60_000L
    }
}
