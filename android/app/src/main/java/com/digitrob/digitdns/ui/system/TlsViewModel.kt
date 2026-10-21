package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.CertView
import com.digitrob.digitdns.core.network.ConfigResponse
import com.digitrob.digitdns.data.ConfigRepository
import com.digitrob.digitdns.data.TlsRepository
import com.digitrob.digitdns.domain.model.AcmeRun
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import javax.inject.Inject

data class TlsUiState(
    val domain: String = "",
    val email: String = "",
    val purpose: String = "panel",
    val run: AcmeRun? = null,
    val autoRenew: Boolean = true,
    /** The settings snapshot, used for the on-disk certificates and domain prefill. */
    val config: ConfigResponse? = null,
    val working: Boolean = false,
    val message: String? = null,
    val error: String? = null,
) {
    /** The certificate currently on disk for each purpose. */
    val certs: Map<String, CertView?>
        get() = mapOf(
            "panel" to config?.panelCert,
            "subscription" to config?.subscriptionCert,
            "dot" to config?.dotCert,
        )

    /** The configured domain per purpose, used to pre-seed the domain field. */
    val domainsByPurpose: Map<String, String>
        get() = mapOf(
            "panel" to config?.tls?.domain.orEmpty(),
            "subscription" to config?.tls?.domain.orEmpty(),
            "dot" to config?.tls?.dotHostname.orEmpty(),
        ).mapValues { (_, value) -> value.ifBlank { config?.server?.domain.orEmpty() } }

    /** The certificate for the selected purpose, if any is on disk. */
    val selectedCert: CertView? get() = certs[purpose]

    /**
     * True when the selected purpose already has a certificate on disk but it has
     * expired — the panel relabels the action from "Issue" to "Renew" in that case,
     * because issuing over an expired cert is a renewal, not a first issuance.
     */
    val selectedCertExpired: Boolean get() = selectedCert?.expired == true
}

/**
 * Certificate issuance. A run is asynchronous: [issue] only *starts* it (or applies an
 * existing certificate), then the screen polls [TlsRepository.acmeStatus] until the
 * run stops. The poll is a plain delay loop rather than SSE because the run is short
 * and the endpoint is a single small object — a stream would be more machinery than
 * the two-second poll it replaces.
 */
@HiltViewModel
class TlsViewModel @Inject constructor(
    private val repository: TlsRepository,
    private val configRepository: ConfigRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(TlsUiState())
    val state: StateFlow<TlsUiState> = _state.asStateFlow()

    private var pollJob: Job? = null

    init {
        refreshStatus()
        loadConfig()
    }

    fun onDomainChange(v: String) = _state.update { it.copy(domain = v) }
    fun onEmailChange(v: String) = _state.update { it.copy(email = v) }

    /**
     * Switching purpose re-seeds the domain field from that purpose's configured
     * domain, unless the operator has already typed something for it. Without this the
     * field would keep whatever the previous purpose had, which is the wrong name half
     * the time.
     */
    fun onPurposeChange(v: String) = _state.update { current ->
        current.copy(domain = current.domainsByPurpose[v].orEmpty(), purpose = v)
    }

    fun dismissMessage() = _state.update { it.copy(message = null) }

    /**
     * Loads the settings snapshot to show each purpose's on-disk certificate and to
     * pre-seed the domain fields. A failure here is not fatal — issuance still works
     * with a hand-typed domain — so it is swallowed rather than surfaced as an error.
     */
    private fun loadConfig() {
        viewModelScope.launch {
            val config = runCatching { configRepository.config() }.getOrNull() ?: return@launch
            _state.update { current ->
                // Seed the domain only when the operator has not started typing, so a
                // slow config load never overwrites an in-progress entry.
                current.copy(
                    config = config,
                    domain = current.domain.ifBlank { current.domainsByPurpose[current.purpose].orEmpty() },
                )
            }
        }
    }

    fun refreshStatus() {
        viewModelScope.launch {
            runCatching { repository.acmeStatus() }
                .onSuccess { run -> _state.update { it.copy(run = run, error = null) } }
                .onFailure { e ->
                    if (e is ApiException) _state.update { s -> s.copy(error = e.userMessage) }
                }
        }
    }

    fun issue() {
        val s = _state.value
        viewModelScope.launch {
            _state.update { it.copy(working = true, error = null, message = null) }
            try {
                val result = repository.issue(s.domain.trim(), s.email.trim(), s.purpose, force = s.selectedCertExpired)
                _state.update {
                    it.copy(
                        working = false,
                        message = if (result.issuing) {
                            "Issuance started for ${result.domain}."
                        } else {
                            result.detail.ifBlank { "A valid certificate was already present and has been applied." }
                        },
                    )
                }
                if (result.issuing) awaitRun()
            } catch (e: ApiException) {
                _state.update { it.copy(working = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(working = false, error = e.message ?: "Could not start issuance.") }
            }
        }
    }

    fun setAutoRenew(enabled: Boolean) {
        viewModelScope.launch {
            try {
                val applied = repository.setAutoRenew(enabled)
                _state.update {
                    it.copy(
                        autoRenew = applied,
                        message = if (applied) "DoT/DoH auto-renew is on." else "DoT/DoH auto-renew is off.",
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not change auto-renew.") }
            }
        }
    }

    /** Poll until the run reports not-running. A transient status failure is skipped. */
    private fun awaitRun() {
        pollJob?.cancel()
        pollJob = viewModelScope.launch {
            var running = true
            while (running) {
                delay(POLL_INTERVAL_MS)
                val run = runCatching { repository.acmeStatus() }.getOrNull()
                if (run != null) {
                    _state.update { it.copy(run = run) }
                    running = run.running
                }
            }
        }
    }

    override fun onCleared() {
        pollJob?.cancel()
        super.onCleared()
    }

    private companion object {
        const val POLL_INTERVAL_MS = 2_000L
    }
}
