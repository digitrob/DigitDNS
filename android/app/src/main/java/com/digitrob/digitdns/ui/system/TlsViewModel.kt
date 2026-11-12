package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.CertView
import com.digitrob.digitdns.core.network.CertificateSourceView
import com.digitrob.digitdns.core.network.ConfigResponse
import com.digitrob.digitdns.core.network.SubscriptionSaveRequest
import com.digitrob.digitdns.core.network.TlsCertInstallRequest
import com.digitrob.digitdns.core.network.TlsIssueRequest
import com.digitrob.digitdns.data.ConfigRepository
import com.digitrob.digitdns.data.SubscriptionRepository
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
    /**
     * The subscriber portal certificate's renewal switch. It saves through
     * `POST settings/subscription` (the same endpoint the Subscription page used),
     * because that is the flag the daemon's subscription renewal loop reads.
     */
    val subscriptionAutoRenew: Boolean = true,
    val certAutoRenewBusy: Boolean = false,
    /** The settings snapshot, used for the on-disk certificates and domain prefill. */
    val config: ConfigResponse? = null,
    /** `acme-http01`, `acme-dns01`, `files-path` or `files-upload`. */
    val certSource: String = "acme-http01",
    val certName: String = "",
    val dnsZone: String = "",
    val dnsToken: String = "",
    val certPath: String = "",
    val keyPath: String = "",
    val allowSelfSigned: Boolean = false,
    val certPem: String = "",
    val keyPem: String = "",
    val certBusy: Boolean = false,
    val working: Boolean = false,
    /**
     * The pull-to-refresh indicator's own flag.
     *
     * It used to be bound to [working], which belongs to a certificate run and is
     * never set by a status refresh — so the pull indicator had nothing to animate
     * on, and a run in progress spun the pull arc instead of the run's own one.
     */
    val isRefreshing: Boolean = false,
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
            "subscription" to config?.subscription?.domain.orEmpty(),
            "dot" to config?.tls?.dotHostname.orEmpty(),
        ).mapValues { (_, value) -> value.ifBlank { config?.server?.domain.orEmpty() } }

    /** The certificate-source record for the selected purpose. */
    val currentCertSource: CertificateSourceView?
        get() = config?.certificates?.forPurpose(purpose)

    /** The certificate for the selected purpose, if any is on disk. */
    val selectedCert: CertView? get() = certs[purpose]

    /**
     * True when the selected purpose already has a certificate on disk but it has
     * expired — the panel relabels the action from "Issue" to "Renew" in that case,
     * because issuing over an expired cert is a renewal, not a first issuance.
     */
    val selectedCertExpired: Boolean get() = selectedCert?.expired == true
}

/** The UI source key for a stored certificate record. */
private fun certSourceKey(cfg: CertificateSourceView?): String = when {
    cfg == null -> "acme-http01"
    cfg.source == "files" -> "files-path"
    cfg.challenge == "dns-01" -> "acme-dns01"
    else -> "acme-http01"
}

/**
 * Certificate issuance. A run is asynchronous: [issue] only *starts* it (or applies an
 * existing certificate), then the screen polls [TlsRepository.acmeStatus] until the
 * run stops. The poll is a plain delay loop rather than SSE because the run is short
 * and the endpoint is a single small object — a stream would be more machinery than
 * the two-second poll it replaces.
 */
@Suppress("TooManyFunctions")
@HiltViewModel
class TlsViewModel @Inject constructor(
    private val repository: TlsRepository,
    private val configRepository: ConfigRepository,
    private val subscriptionRepository: SubscriptionRepository,
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
    fun onCertSourceChange(v: String) = _state.update { it.copy(certSource = v) }
    fun onCertNameChange(v: String) = _state.update { it.copy(certName = v) }
    fun onDnsZoneChange(v: String) = _state.update { it.copy(dnsZone = v) }
    fun onDnsTokenChange(v: String) = _state.update { it.copy(dnsToken = v) }
    fun onCertPathChange(v: String) = _state.update { it.copy(certPath = v) }
    fun onKeyPathChange(v: String) = _state.update { it.copy(keyPath = v) }
    fun onAllowSelfSignedChange(v: Boolean) = _state.update { it.copy(allowSelfSigned = v) }
    fun onCertPemChange(v: String) = _state.update { it.copy(certPem = v) }
    fun onKeyPemChange(v: String) = _state.update { it.copy(keyPem = v) }

    /**
     * Switching purpose re-seeds the domain and certificate-source fields from that
     * purpose's record. Without this the fields would keep the previous purpose's
     * values, which is the wrong name and often the wrong source half the time.
     */
    fun onPurposeChange(v: String) = _state.update { current ->
        val cfg = current.config?.certificates?.forPurpose(v)
        current.copy(
            domain = current.domainsByPurpose[v].orEmpty(),
            purpose = v,
            certSource = certSourceKey(cfg),
            certName = cfg?.certName.orEmpty(),
            dnsZone = cfg?.dnsZone.orEmpty(),
            certPath = cfg?.certPath.orEmpty(),
            keyPath = cfg?.keyPath.orEmpty(),
            allowSelfSigned = cfg?.allowSelfSigned == true,
            certPem = "",
            keyPem = "",
        )
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
                val cfg = config.certificates?.forPurpose(current.purpose)
                // Seed the domain only when the operator has not started typing, so a
                // slow config load never overwrites an in-progress entry.
                current.copy(
                    config = config,
                    domain = current.domain.ifBlank { current.domainsByPurpose[current.purpose].orEmpty() },
                    // Both renewal switches tell the truth from the same snapshot: the
                    // DoT flag used to default to "on" without ever being read back.
                    autoRenew = config.tls?.dotAutoRenew ?: current.autoRenew,
                    subscriptionAutoRenew = config.subscription?.autoRenewAcme ?: current.subscriptionAutoRenew,
                    certSource = certSourceKey(cfg),
                    certName = cfg?.certName.orEmpty(),
                    dnsZone = cfg?.dnsZone.orEmpty(),
                    certPath = cfg?.certPath.orEmpty(),
                    keyPath = cfg?.keyPath.orEmpty(),
                    allowSelfSigned = cfg?.allowSelfSigned == true,
                )
            }
        }
    }

    fun refreshStatus() {
        viewModelScope.launch {
            _state.update { it.copy(isRefreshing = true) }
            try {
                runCatching { repository.acmeStatus() }
                    .onSuccess { run -> _state.update { it.copy(run = run, error = null) } }
                    .onFailure { e ->
                        if (e is ApiException) _state.update { s -> s.copy(error = e.userMessage) }
                    }
            } finally {
                // Cleared whatever happened, so a failed status call cannot leave the
                // pull indicator spinning.
                _state.update { it.copy(isRefreshing = false) }
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
                        message = if (result.issuing) {
                            "Issuance started for ${result.domain}."
                        } else {
                            result.detail.ifBlank { "A valid certificate was already present and has been applied." }
                        },
                    )
                }
                if (result.issuing) awaitRun()
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not start issuance.") }
            } finally {
                // A run that failed to start must not leave the Issue button
                // disabled for the rest of the session.
                _state.update { it.copy(working = false) }
            }
        }
    }

    /**
     * The advanced certificate-source action: ACME HTTP-01, ACME DNS-01 wildcard, a
     * PEM pair on the server, or a PEM pair pasted/loaded from the phone.
     */
    fun applyCertificate() {
        val s = _state.value
        val problem = certValidationError(s)
        if (problem != null) {
            _state.update { it.copy(error = problem) }
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(certBusy = true, error = null, message = null) }
            try {
                if (s.certSource.startsWith("acme")) applyAcme(s) else applyFiles(s)
                loadConfig()
            } catch (e: ApiException) {
                _state.update { it.copy(error = e.userMessage) }
            } catch (e: IOException) {
                _state.update { it.copy(error = e.message ?: "Could not apply the certificate.") }
            } finally {
                _state.update { it.copy(certBusy = false) }
            }
        }
    }

    private fun certValidationError(s: TlsUiState): String? = when {
        s.domain.isBlank() -> "Enter the hostname this surface serves."
        s.certSource == "acme-dns01" && s.dnsToken.isBlank() ->
            "A Cloudflare API token is required for DNS-01."
        s.certSource == "files-path" && (s.certPath.isBlank() || s.keyPath.isBlank()) ->
            "Enter both the certificate and key paths."
        s.certSource == "files-upload" && (s.certPem.isBlank() || s.keyPem.isBlank()) ->
            "Paste or load both the certificate and the private key."
        else -> null
    }

    private suspend fun applyAcme(s: TlsUiState) {
        val dns01 = s.certSource == "acme-dns01"
        val domain = s.domain.trim()
        val result = repository.issue(
            TlsIssueRequest(
                domain = domain,
                email = s.email.trim(),
                purpose = s.purpose,
                force = s.selectedCertExpired,
                challenge = if (dns01) "dns-01" else "http-01",
                certName = if (dns01) s.certName.ifBlank { domain } else domain,
                dnsProvider = if (dns01) "cloudflare" else "",
                dnsZone = if (dns01) s.dnsZone.trim() else "",
                dnsToken = if (dns01) s.dnsToken.trim() else "",
            ),
        )
        _state.update {
            it.copy(
                message = if (result.issuing) {
                    "Issuance started for ${result.domain}."
                } else {
                    result.detail.ifBlank { "A valid certificate was already present and has been applied." }
                },
            )
        }
        if (result.issuing) awaitRun()
    }

    private suspend fun applyFiles(s: TlsUiState) {
        val result = repository.installCertificate(
            TlsCertInstallRequest(
                purpose = s.purpose,
                domain = s.domain.trim(),
                certPem = if (s.certSource == "files-upload") s.certPem else "",
                keyPem = if (s.certSource == "files-upload") s.keyPem else "",
                certPath = if (s.certSource == "files-path") s.certPath.trim() else "",
                keyPath = if (s.certSource == "files-path") s.keyPath.trim() else "",
                allowSelfSigned = s.allowSelfSigned,
            ),
        )
        _state.update { it.copy(message = result.detail.ifBlank { "Certificate installed and applied." }) }
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

    /**
     * The subscriber certificate's renewal switch. Its state lives in the subscription
     * settings, not the TLS record, so the flip re-saves that snapshot (the endpoint
     * replaces every field) with only this flag changed. The settings screen no longer
     * sends the flag at all, so the two pages can never disagree: this is the only
     * writer.
     */
    fun setSubscriptionAutoRenew(enabled: Boolean) {
        val sub = _state.value.config?.subscription ?: return
        val previous = _state.value.subscriptionAutoRenew
        viewModelScope.launch {
            // Flip immediately: the save is a round trip, and a switch that waits for
            // the server reads as "the tap did nothing" and invites a second tap.
            _state.update {
                it.copy(subscriptionAutoRenew = enabled, certAutoRenewBusy = true, error = null, message = null)
            }
            try {
                val result = subscriptionRepository.save(
                    SubscriptionSaveRequest(
                        enabled = sub.enabled,
                        domain = sub.domain.trim(),
                        port = sub.port,
                        uriPath = sub.uriPath.ifBlank { "/sub" },
                        title = sub.title.trim(),
                        themeCss = if (sub.themeCssSource == "inline") sub.themeCss else "",
                        themeCssSource = sub.themeCssSource,
                        themeCssPath = if (sub.themeCssSource == "local") sub.themeCssPath.trim() else "",
                        themeCssUrl = if (sub.themeCssSource == "url") sub.themeCssUrl.trim() else "",
                        autoRenewAcme = enabled,
                    ),
                )
                // Trust the value the server reports over the one we asked for; a
                // normaliser or an older daemon can answer differently.
                val applied = result.settings?.autoRenewAcme ?: enabled
                _state.update {
                    it.copy(
                        subscriptionAutoRenew = applied,
                        certAutoRenewBusy = false,
                        message = if (applied) "Certificate auto-renew is on." else "Certificate auto-renew is off.",
                    )
                }
            } catch (e: ApiException) {
                _state.update {
                    it.copy(subscriptionAutoRenew = previous, certAutoRenewBusy = false, error = e.userMessage)
                }
            } catch (e: IOException) {
                _state.update {
                    it.copy(
                        subscriptionAutoRenew = previous,
                        certAutoRenewBusy = false,
                        error = e.message ?: "Could not change certificate auto-renew.",
                    )
                }
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
