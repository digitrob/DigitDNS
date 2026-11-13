package com.digitrob.digitdns.ui.system

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.FilterChip
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.R
import com.digitrob.digitdns.domain.model.AcmeRun
import com.digitrob.digitdns.ui.components.DigitDnsDropdownMenu
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.PanelSnackbarHost
import com.digitrob.digitdns.ui.components.PanelToggle
import com.digitrob.digitdns.ui.components.showPanelToast
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono

/** A purpose this certificate is being requested for. */
private val PURPOSES = listOf(
    "panel" to "Panel",
    "subscription" to "Subscription",
    "dot" to "DoT/DoH",
)

/** The certificate-source choices, matching the web card's four modes. */
private val CERT_SOURCES = listOf(
    "acme-http01" to "HTTP-01",
    "acme-dns01" to "DNS-01 wildcard",
    "files-path" to "PEM path",
    "files-upload" to "PEM upload",
)

/**
 * TLS / ACME certificate issuance. The purpose chip narrows which endpoint the
 * certificate is for; a run's progress is polled and shown inline.
 */
@Composable
fun TlsScreen(viewModel: TlsViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }

    LaunchedEffect(state.message) {
        val message = state.message ?: return@LaunchedEffect
        snackbar.showPanelToast(message)
        viewModel.dismissMessage()
    }

    Scaffold(snackbarHost = { PanelSnackbarHost(snackbar) }, contentWindowInsets = WindowInsets(0)) { padding ->
    PanelRefreshBox(
        isRefreshing = state.isRefreshing,
        onRefresh = viewModel::refreshStatus,
        modifier = Modifier.fillMaxSize().padding(padding),
    ) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            state.error?.let {
                Text(it, color = DigitDnsTheme.colors.danger, style = MaterialTheme.typography.bodySmall)
            }

            IssueCard(state, viewModel)
            CertificateSourceCard(state, viewModel)
            state.run?.takeIf { it.running }?.let { RunCard(it) }
            SubscriptionAutoRenewCard(
                enabled = state.subscriptionAutoRenew,
                busy = state.certAutoRenewBusy,
                ready = state.config?.subscription != null,
                onToggle = viewModel::setSubscriptionAutoRenew,
            )
            AutoRenewCard(state.autoRenew, viewModel::setAutoRenew)
        }
    }
    }
}

/**
 * The on-disk certificate for the selected purpose, when one exists. Shows the validity
 * window and the server's own expiry verdict — the same facts the panel's SSL/TLS card
 * puts next to the Issue/Renew button, so the operator can see whether a click is even
 * needed.
 */
@Composable
private fun CertDetails(cert: com.digitrob.digitdns.core.network.CertView) {
    val daysLeft = daysUntil(cert.notAfter)
    val range = stringResource(
        R.string.tls_valid_range,
        formatCertDate(cert.notBefore),
        formatCertDate(cert.notAfter),
    )
    val detail = when {
        cert.expired -> "Expired ${formatCertDate(cert.notAfter)}"
        daysLeft != null -> "$range · $daysLeft day${if (daysLeft == 1L) "" else "s"} left"
        else -> range
    }
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(
            text = cert.domain.ifBlank { cert.subject }.ifBlank { "Certificate" },
            style = MaterialTheme.typography.bodyMedium.copy(fontFamily = PanelMono),
            color = MaterialTheme.colorScheme.onSurface,
        )
        Text(
            text = detail,
            style = MaterialTheme.typography.bodySmall,
            color = if (cert.expired) DigitDnsTheme.colors.danger else DigitDnsTheme.colors.success,
        )
    }
}

/** Whole days until an RFC 3339 timestamp, or null when it cannot be parsed. */
private fun daysUntil(raw: String): Long? {
    if (raw.isBlank()) return null
    return runCatching {
        java.time.Duration.between(java.time.Instant.now(), java.time.Instant.parse(raw)).toDays()
            .coerceAtLeast(0)
    }.getOrNull()
}

/** Renders an RFC 3339 timestamp as `YYYY-MM-DD`, leaving anything unparseable as-is. */
private fun formatCertDate(raw: String): String {
    if (raw.isBlank()) return "unknown"
    val date = raw.substringBefore('T')
    return if (date.length == ISO_DATE_LENGTH) date else raw
}

/** The length of a `YYYY-MM-DD` date, i.e. an RFC 3339 timestamp's date half. */
private const val ISO_DATE_LENGTH = 10

@Composable
private fun IssueCard(state: TlsUiState, viewModel: TlsViewModel) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Text("Issue a Let's Encrypt certificate", style = MaterialTheme.typography.titleMedium)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                PURPOSES.forEach { (value, label) ->
                    FilterChip(
                        selected = state.purpose == value,
                        onClick = { viewModel.onPurposeChange(value) },
                        label = { Text(label) },
                    )
                }
            }
            // The certificate already on disk for this purpose, if there is one. An
            // expired cert relabels the button to "Renew" below.
            state.selectedCert?.let { CertDetails(it) }
            OutlinedTextField(
                value = state.domain,
                onValueChange = viewModel::onDomainChange,
                label = { Text("Domain") },
                supportingText = { Hint("The DNS name to issue for; it must already resolve here.") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = state.email,
                onValueChange = viewModel::onEmailChange,
                label = { Text("Contact email (optional)") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            Button(
                onClick = viewModel::issue,
                enabled = !state.working && state.domain.isNotBlank(),
                modifier = Modifier.fillMaxWidth(),
            ) {
                if (state.working) {
                    CircularProgressIndicator(
                        modifier = Modifier.padding(end = 8.dp),
                        strokeWidth = 2.dp,
                        color = MaterialTheme.colorScheme.onPrimary,
                    )
                }
                Text(if (state.selectedCertExpired) "Renew certificate" else "Issue certificate")
            }
        }
    }
}

/**
 * The advanced certificate path: Let's Encrypt HTTP-01, a wildcard through Cloudflare
 * DNS-01, a PEM pair already on the server, or a PEM pair loaded from the phone.
 * It shares the purpose and served-hostname fields with [IssueCard] above.
 */
@Composable
private fun CertificateSourceCard(state: TlsUiState, viewModel: TlsViewModel) {
    val context = LocalContext.current
    var pendingFile by remember { mutableStateOf("") }
    val picker = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri != null) {
            val text = runCatching {
                context.contentResolver.openInputStream(uri)?.bufferedReader()?.use { it.readText() }.orEmpty()
            }.getOrDefault("")
            if (pendingFile == "cert") viewModel.onCertPemChange(text) else viewModel.onKeyPemChange(text)
        }
        pendingFile = ""
    }

    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Text("Certificate source", style = MaterialTheme.typography.titleMedium)
            Text(
                "Use Let's Encrypt, a wildcard through Cloudflare DNS-01, or a PEM pair you already hold.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )

            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Box(modifier = Modifier.weight(1f)) {
                    CertDropdown(
                        label = "Surface",
                        value = state.purpose,
                        options = PURPOSES,
                        onSelect = viewModel::onPurposeChange,
                    )
                }
                Box(modifier = Modifier.weight(1f)) {
                    CertDropdown(
                        label = "Source",
                        value = state.certSource,
                        options = CERT_SOURCES,
                        onSelect = viewModel::onCertSourceChange,
                    )
                }
            }

            OutlinedTextField(
                value = state.domain,
                onValueChange = viewModel::onDomainChange,
                label = { Text("Served hostname") },
                supportingText = { Hint("The name clients connect to; it must already resolve here.") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )

            when (state.certSource) {
                "acme-dns01" -> Dns01Fields(state, viewModel)
                "files-path" -> FilesPathFields(state, viewModel)
                "files-upload" -> FilesUploadFields(
                    state = state,
                    viewModel = viewModel,
                    onLoadCert = {
                        pendingFile = "cert"
                        picker.launch(arrayOf("*/*"))
                    },
                    onLoadKey = {
                        pendingFile = "key"
                        picker.launch(arrayOf("*/*"))
                    },
                )
            }

            Button(
                onClick = viewModel::applyCertificate,
                enabled = !state.certBusy && state.domain.isNotBlank(),
                modifier = Modifier.fillMaxWidth(),
            ) {
                if (state.certBusy) {
                    CircularProgressIndicator(
                        modifier = Modifier.padding(end = 8.dp),
                        strokeWidth = 2.dp,
                        color = MaterialTheme.colorScheme.onPrimary,
                    )
                }
                Text("Apply certificate")
            }
        }
    }
}

@Composable
private fun CertDropdown(
    label: String,
    value: String,
    options: List<Pair<String, String>>,
    onSelect: (String) -> Unit,
) {
    var expanded by remember { mutableStateOf(false) }
    val selected = options.firstOrNull { it.first == value }?.second ?: value
    Column {
        Text(
            label,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Box {
            OutlinedButton(
                onClick = { expanded = true },
                modifier = Modifier.fillMaxWidth(),
            ) {
                Text(selected, modifier = Modifier.weight(1f))
                Text("▾")
            }
            DigitDnsDropdownMenu(
                expanded = expanded,
                onDismissRequest = { expanded = false },
            ) {
                options.forEach { (key, text) ->
                    DropdownMenuItem(
                        text = { Text(text) },
                        onClick = {
                            expanded = false
                            onSelect(key)
                        },
                    )
                }
            }
        }
    }
}

@Composable
private fun Dns01Fields(state: TlsUiState, viewModel: TlsViewModel) {
    OutlinedTextField(
        value = state.certName,
        onValueChange = viewModel::onCertNameChange,
        label = { Text("Certificate name (wildcard allowed)") },
        supportingText = { Hint("e.g. *.example.com — it must cover the served hostname.") },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
    OutlinedTextField(
        value = state.dnsZone,
        onValueChange = viewModel::onDnsZoneChange,
        label = { Text("Cloudflare zone (optional)") },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
    OutlinedTextField(
        value = state.dnsToken,
        onValueChange = viewModel::onDnsTokenChange,
        label = { Text("Cloudflare API token") },
        supportingText = { Hint("Needs Zone:Zone:Read and Zone:DNS:Edit.") },
        singleLine = true,
        visualTransformation = PasswordVisualTransformation(),
        modifier = Modifier.fillMaxWidth(),
    )
}

@Composable
private fun FilesPathFields(state: TlsUiState, viewModel: TlsViewModel) {
    OutlinedTextField(
        value = state.certPath,
        onValueChange = viewModel::onCertPathChange,
        label = { Text("Certificate path (fullchain.pem)") },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
    OutlinedTextField(
        value = state.keyPath,
        onValueChange = viewModel::onKeyPathChange,
        label = { Text("Private key path (privkey.pem)") },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
    SelfSignedRow(state.allowSelfSigned, viewModel::onAllowSelfSignedChange)
}

@Composable
private fun FilesUploadFields(
    state: TlsUiState,
    viewModel: TlsViewModel,
    onLoadCert: () -> Unit,
    onLoadKey: () -> Unit,
) {
    OutlinedTextField(
        value = state.certPem,
        onValueChange = viewModel::onCertPemChange,
        label = { Text("Certificate PEM") },
        minLines = 3,
        maxLines = 6,
        modifier = Modifier.fillMaxWidth(),
    )
    TextButton(onClick = onLoadCert) { Text("Load certificate file") }
    OutlinedTextField(
        value = state.keyPem,
        onValueChange = viewModel::onKeyPemChange,
        label = { Text("Private key PEM") },
        minLines = 3,
        maxLines = 6,
        modifier = Modifier.fillMaxWidth(),
    )
    TextButton(onClick = onLoadKey) { Text("Load key file") }
    SelfSignedRow(state.allowSelfSigned, viewModel::onAllowSelfSignedChange)
}

@Composable
private fun SelfSignedRow(checked: Boolean, onCheckedChange: (Boolean) -> Unit) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Column(modifier = Modifier.weight(1f).padding(end = 12.dp)) {
            Text("Allow self-signed", style = MaterialTheme.typography.bodyMedium)
            Text(
                "Private CA or LAN only.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        PanelToggle(checked = checked, onCheckedChange = onCheckedChange)
    }
}

/**
 * The subscriber portal certificate's renewal switch. It sits next to the DoT/DoH
 * switch because both are certificate-renewal policy, even though this one persists in
 * the subscription settings rather than the TLS record.
 *
 * [ready] is false until the settings snapshot lands: the save has to send the whole
 * subscription record, so a flip before then has nothing to write back. The switch is
 * disabled rather than silently ignoring the tap.
 */
@Composable
private fun SubscriptionAutoRenewCard(
    enabled: Boolean,
    busy: Boolean,
    ready: Boolean,
    onToggle: (Boolean) -> Unit,
) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(modifier = Modifier.weight(1f).padding(end = 12.dp)) {
                Text("Certificate Auto-renew", style = MaterialTheme.typography.titleMedium)
                Text(
                    text = stringResource(R.string.tls_renew_le),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            PanelToggle(
                checked = enabled,
                onCheckedChange = onToggle,
                enabled = ready && !busy,
            )
        }
    }
}

@Composable
private fun AutoRenewCard(autoRenew: Boolean, onToggle: (Boolean) -> Unit) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(modifier = Modifier.weight(1f)) {
                Text("Let's Encrypt Auto-renew", style = MaterialTheme.typography.titleMedium)
                Text(
                    text = stringResource(R.string.tls_renew_auto),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            PanelToggle(checked = autoRenew, onCheckedChange = onToggle)
        }
    }
}

/** The one-line supporting text shared by the fields on this screen. */
@Composable
private fun Hint(text: String) {
    Text(text, style = MaterialTheme.typography.bodySmall)
}

@Composable
private fun RunCard(run: AcmeRun) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    text = run.purpose.ifBlank { "Certificate run" },
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.weight(1f),
                )
                Text(
                    text = if (run.running) "Running" else "Idle",
                    style = MaterialTheme.typography.labelSmall,
                    color = if (run.running) DigitDnsTheme.colors.warning else DigitDnsTheme.colors.success,
                )
            }
            if (run.domain.isNotBlank()) {
                Text(run.domain, style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono))
            }
            if (run.running) {
                LinearProgressIndicator(
                    progress = { (run.progress.coerceIn(0, 100)) / 100f },
                    modifier = Modifier.fillMaxWidth(),
                )
                Text(
                    text = stringResource(R.string.tls_run_progress, run.stage, run.progress),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (run.detail.isNotBlank()) {
                Text(run.detail, style = MaterialTheme.typography.bodySmall)
            }
            if (run.error.isNotBlank()) {
                Text(run.error, style = MaterialTheme.typography.bodySmall, color = DigitDnsTheme.colors.danger)
            }
        }
    }
}
