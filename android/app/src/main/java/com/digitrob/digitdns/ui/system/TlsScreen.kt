package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilterChip
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.domain.model.AcmeRun
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.PanelSnackbarHost
import com.digitrob.digitdns.ui.components.showPanelToast
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono

/** A purpose this certificate is being requested for. */
private val PURPOSES = listOf(
    "panel" to "Panel",
    "subscription" to "Subscription",
    "dot" to "DoT/DoH",
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
        isRefreshing = state.working,
        onRefresh = viewModel::refreshStatus,
        modifier = Modifier.fillMaxSize().padding(padding),
    ) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            state.error?.let {
                Text(it, color = DigitDnsTheme.colors.danger, style = MaterialTheme.typography.bodySmall)
            }

            IssueCard(state, viewModel)
            state.run?.takeIf { it.running }?.let { RunCard(it) }
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
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(
            text = cert.domain.ifBlank { cert.subject }.ifBlank { "Certificate" },
            style = MaterialTheme.typography.bodyMedium.copy(fontFamily = PanelMono),
            color = MaterialTheme.colorScheme.onSurface,
        )
        Text(
            text = "Valid ${formatCertDate(cert.notBefore)} → ${formatCertDate(cert.notAfter)}",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Text(
            text = if (cert.expired) "Expired" else "Valid",
            style = MaterialTheme.typography.labelSmall,
            color = if (cert.expired) DigitDnsTheme.colors.danger else DigitDnsTheme.colors.success,
        )
    }
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
            Text("Issue a certificate", style = MaterialTheme.typography.titleMedium)
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
                Text(if (state.selectedCertExpired) "Renew" else "Issue certificate")
            }
        }
    }
}

@Composable
private fun AutoRenewCard(autoRenew: Boolean, onToggle: (Boolean) -> Unit) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(modifier = Modifier.weight(1f)) {
                Text("DoT/DoH auto-renew", style = MaterialTheme.typography.titleMedium)
                Text(
                    text = "Renew the DNS-over-TLS/HTTPS certificate automatically.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Switch(checked = autoRenew, onCheckedChange = onToggle)
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
                    text = "${run.stage} · ${run.progress}%",
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
