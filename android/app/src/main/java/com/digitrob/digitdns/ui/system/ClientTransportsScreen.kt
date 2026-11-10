@file:Suppress("TooManyFunctions")

package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.R
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.SettingsLoadingSkeleton
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono

/**
 * Every client transport, the phone counterpart of the web's Client Transports
 * card. Switches and completed fields apply as they are edited; incomplete
 * SNI-token or mTLS forms stay local.
 */
@Composable
fun ClientTransportsScreen(viewModel: ClientTransportsViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()

    PanelRefreshBox(
        isRefreshing = state.isRefreshing,
        onRefresh = viewModel::load,
        modifier = Modifier.fillMaxSize(),
    ) {
        BoxWithConstraints(modifier = Modifier.fillMaxSize()) {
            // On a wide panel the cards pair up two per row; phones keep the
            // stacked layout. The shared IP note and save action sit after the
            // cards, so they apply to every transport at once.
            val wide = maxWidth >= 700.dp
            Column(
                modifier = Modifier
                    .fillMaxSize()
                    .verticalScroll(rememberScrollState())
                    .padding(12.dp),
                verticalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                TransportsHeader(state, wide)
                if (state.isLoading) {
                    SettingsLoadingSkeleton(cards = 3)
                } else {
                    if (wide) {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                            verticalAlignment = Alignment.Top,
                        ) {
                            PlainDnsCard(state, viewModel, Modifier.weight(1f))
                            DotCard(state, viewModel, Modifier.weight(1f))
                        }
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                            verticalAlignment = Alignment.Top,
                        ) {
                            DohCard(state, viewModel, Modifier.weight(1f))
                            DoqCard(state, viewModel, Modifier.weight(1f))
                        }
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                            verticalAlignment = Alignment.Top,
                        ) {
                            Doh3Card(state, Modifier.weight(1f))
                            DnscryptCard(state, viewModel, Modifier.weight(1f))
                        }
                    } else {
                        PlainDnsCard(state, viewModel)
                        DotCard(state, viewModel)
                        DohCard(state, viewModel)
                        DoqCard(state, viewModel)
                        Doh3Card(state)
                        DnscryptCard(state, viewModel)
                    }
                    MtlsCard(state, viewModel)
                    IpAllowlistCard(state)
                    TransportSaveStatus(state)
                    state.error?.let {
                        Text(it, color = DigitDnsTheme.colors.danger, style = MaterialTheme.typography.bodySmall)
                    }
                }
            }
        }
    }
}

// The tab header already says Transports; a full card repeating the title only
// cost vertical space. The top-right line names every transport that is not
// currently serving, so a disabled switch is visible without scanning the cards.
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun TransportsHeader(state: ClientTransportsUiState, wide: Boolean) {
    val names = listOf(
        stringResource(R.string.transports_plain) to (state.plainDNSEnabled && state.plainDNSListening),
        "DoT" to (state.dotEnabled && state.dotListening),
        "DoH" to (state.dohEnabled && state.dohListening),
        "DoQ" to (state.doqEnabled && state.doqListening),
        "DoH3" to state.doh3Listening,
        "DNSCrypt" to (state.dnscryptEnabled && state.dnscryptListening),
    )
    val running = names.filter { it.second }.map { it.first }
    val missing = names.filterNot { it.second }.map { it.first }
    val allOn = missing.isEmpty()
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.Top,
    ) {
        Text(
            stringResource(R.string.transports_help),
            modifier = Modifier.weight(1f),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.width(8.dp))
        // Fixed width for the status area: the text column must not reflow when
        // a transport switches on or off and the badge text changes length.
        FlowRow(
            modifier = Modifier.width(if (wide) 276.dp else 168.dp),
            horizontalArrangement = Arrangement.spacedBy(4.dp, Alignment.End),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            if (!state.loaded) {
                StatusBadge(
                    text = stringResource(R.string.transports_status_loading),
                    color = DigitDnsTheme.colors.textMuted,
                )
            } else {
                if (running.isNotEmpty()) {
                    StatusBadge(
                        text = if (allOn) {
                            stringResource(R.string.transports_all_on)
                        } else {
                            stringResource(R.string.transports_on, running.joinToString(", "))
                        },
                        color = DigitDnsTheme.colors.success,
                        maxLines = 2,
                    )
                }
                if (missing.isNotEmpty()) {
                    StatusBadge(
                        text = stringResource(R.string.transports_off, missing.joinToString(", ")),
                        color = DigitDnsTheme.colors.warning,
                        maxLines = 2,
                    )
                }
            }
        }
    }
}

@Composable
private fun TransportHeader(
    title: String,
    help: String,
    enabled: Boolean,
    onToggle: (Boolean) -> Unit,
    toggleEnabled: Boolean,
    status: @Composable () -> Unit,
) {
    Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.Top) {
        Column(modifier = Modifier.weight(1f)) {
            Text(title, style = MaterialTheme.typography.titleSmall)
            Text(
                help,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        // Keep the status and switch as one vertically centred group flush with
        // the card's right edge. The status slot has a fixed width and the switch
        // keeps a fixed size, so "Listening" -> "Off" cannot move either one.
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Box(modifier = Modifier.width(132.dp), contentAlignment = Alignment.CenterEnd) {
                status()
            }
            Switch(checked = enabled, onCheckedChange = onToggle, enabled = toggleEnabled)
        }
    }
}

@Composable
private fun EnforcedStatus(enabled: Boolean, saving: Boolean) {
    val text = when {
        saving -> stringResource(R.string.transports_status_saving)
        enabled -> stringResource(R.string.transports_enforced)
        else -> stringResource(R.string.transports_status_off)
    }
    val color = when {
        saving -> DigitDnsTheme.colors.textMuted
        enabled -> DigitDnsTheme.colors.success
        else -> DigitDnsTheme.colors.textMuted
    }
    StatusBadge(text = text, color = color)
}

@Composable
private fun CopyBox(value: String, contentDescription: String) {
    val clipboard = LocalClipboardManager.current
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .border(
                width = 1.dp,
                color = MaterialTheme.colorScheme.outline.copy(alpha = 0.35f),
                shape = RoundedCornerShape(8.dp),
            )
            .padding(horizontal = 10.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = value.ifBlank { "—" },
            style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
            color = MaterialTheme.colorScheme.primary,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f),
        )
        if (value.isNotBlank()) {
            IconButton(onClick = { clipboard.setText(AnnotatedString(value)) }) {
                Icon(Icons.Filled.ContentCopy, contentDescription = contentDescription)
            }
        }
    }
}

private fun endpointURL(scheme: String, host: String, port: String, path: String = ""): String {
    if (host.isBlank()) return "Configure a domain and certificate first"
    val authority = if (scheme == "https" && port == "443") host else "$host:$port"
    return "$scheme://$authority$path"
}

@Composable
private fun PlainDnsCard(
    state: ClientTransportsUiState,
    viewModel: ClientTransportsViewModel,
    modifier: Modifier = Modifier,
) {
    PanelCard(modifier = modifier.fillMaxWidth(), contentPadding = 12.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            TransportHeader(
                title = stringResource(R.string.transports_plain),
                help = stringResource(R.string.transports_plain_help),
                enabled = state.plainDNSEnabled,
                onToggle = viewModel::setPlainDNSEnabled,
                toggleEnabled = true,
                status = {
                    TransportStatus(state.plainDNSEnabled, state.plainDNSListening, false, false)
                },
            )
            OutlinedTextField(
                value = state.plainDNSPort,
                onValueChange = viewModel::setPlainDNSPort,
                label = { Text(stringResource(R.string.transports_plain_port)) },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                modifier = Modifier.fillMaxWidth().saveTransportOnBlur(viewModel),
            )
            CopyBox(
                value = if (state.serverHost.isBlank()) {
                    "Configure a domain and certificate first"
                } else {
                    "${state.serverHost}:${state.plainDNSPort}"
                },
                contentDescription = stringResource(R.string.transports_copy_plain),
            )
            Text(
                stringResource(R.string.transports_plain_hint),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun DohCard(
    state: ClientTransportsUiState,
    viewModel: ClientTransportsViewModel,
    modifier: Modifier = Modifier,
) {
    PanelCard(modifier = modifier.fillMaxWidth(), contentPadding = 12.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            TransportHeader(
                title = stringResource(R.string.transports_doh),
                help = stringResource(R.string.transports_doh_help),
                enabled = state.dohEnabled,
                onToggle = viewModel::setDohEnabled,
                toggleEnabled = true,
                status = {
                    TransportStatus(state.dohEnabled, state.dohListening, false, false)
                },
            )
            OutlinedTextField(
                value = state.dohPort,
                onValueChange = viewModel::setDohPort,
                label = { Text(stringResource(R.string.transports_doh_port)) },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                modifier = Modifier.fillMaxWidth().saveTransportOnBlur(viewModel),
            )
            CopyBox(
                value = endpointURL("https", state.dotHost.ifBlank { state.serverHost }, state.dohPort, "/dns-query"),
                contentDescription = stringResource(R.string.transports_copy_doh),
            )
            CertificateRow()
            Text(
                stringResource(R.string.transports_doh_portal_note),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun Doh3Card(
    state: ClientTransportsUiState,
    modifier: Modifier = Modifier,
) {
    PanelCard(modifier = modifier.fillMaxWidth(), contentPadding = 12.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            TransportHeader(
                title = stringResource(R.string.transports_doh3),
                help = stringResource(R.string.transports_doh3_help),
                enabled = true,
                onToggle = {},
                toggleEnabled = false,
                status = {
                    TransportStatus(true, state.doh3Listening, false, false)
                },
            )
            OutlinedTextField(
                value = state.dohPort,
                onValueChange = {},
                enabled = false,
                label = { Text(stringResource(R.string.transports_doh3_port_shared)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            CopyBox(
                value = endpointURL("h3", state.dotHost.ifBlank { state.serverHost }, state.dohPort, "/dns-query"),
                contentDescription = stringResource(R.string.transports_copy_doh3),
            )
            CertificateRow()
            Text(
                stringResource(R.string.transports_doh3_shared_note),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun DoqCard(
    state: ClientTransportsUiState,
    viewModel: ClientTransportsViewModel,
    modifier: Modifier = Modifier,
) {
    PanelCard(modifier = modifier.fillMaxWidth(), contentPadding = 12.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            DoqHeader(state, viewModel)
            DoqPortAndAuth(state, viewModel)
            if (state.doqAuth == "sni-token") {
                DoqTokenEditor(state, viewModel)
            }
            val doqHost = if (state.doqAuth == "sni-token" && state.tokenDomain.isNotBlank()) {
                "<token>.${state.tokenDomain}"
            } else {
                state.dotHost.ifBlank { state.serverHost }
            }
            CopyBox(
                value = endpointURL("quic", doqHost, state.doqPort),
                contentDescription = stringResource(R.string.transports_copy_doq),
            )
            Text(
                stringResource(R.string.transports_firewall),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/** DoT serves whenever it is enabled and the DoH/DoT certificate is installed. */
@Composable
private fun DotCard(
    state: ClientTransportsUiState,
    viewModel: ClientTransportsViewModel,
    modifier: Modifier = Modifier,
) {
    PanelCard(modifier = modifier.fillMaxWidth(), contentPadding = 12.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            TransportHeader(
                title = stringResource(R.string.transports_dot),
                help = stringResource(R.string.transports_dot_help, state.dotPort),
                enabled = state.dotEnabled,
                onToggle = viewModel::setDotEnabled,
                toggleEnabled = true,
                status = {
                    TransportStatus(state.dotEnabled, state.dotListening, false, false)
                },
            )
            OutlinedTextField(
                value = state.dotPort,
                onValueChange = viewModel::setDotPort,
                label = { Text(stringResource(R.string.transports_tcp_port)) },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                modifier = Modifier.fillMaxWidth().saveTransportOnBlur(viewModel),
            )
            val dotHost = if (state.dotAuth == "sni-token" && state.dotTokenDomain.isNotBlank()) {
                "<token>.${state.dotTokenDomain}"
            } else {
                state.dotHost.ifBlank { state.serverHost }
            }
            CopyBox(
                value = endpointURL("tls", dotHost, state.dotPort),
                contentDescription = stringResource(R.string.transports_copy_dot),
            )
            Text(
                stringResource(R.string.transports_auth),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                FilterChip(
                    selected = state.dotAuth != "sni-token",
                    onClick = { viewModel.setDotAuth("ip") },
                    label = { Text(stringResource(R.string.transports_auth_ip)) },
                )
                FilterChip(
                    selected = state.dotAuth == "sni-token",
                    onClick = { viewModel.setDotAuth("sni-token") },
                    label = { Text(stringResource(R.string.transports_auth_token)) },
                )
            }
            if (state.dotAuth == "sni-token") {
                TokenEditor(
                    model = TokenEditorModel(
                        domain = state.dotTokenDomain,
                        domainHint = "dot.example.com",
                        newToken = state.newDotToken,
                        tokens = state.dotTokens,
                        emptyLabel = stringResource(R.string.transports_no_dot_tokens),
                    ),
                    onDomainChange = viewModel::setDotTokenDomain,
                    onNewTokenChange = viewModel::setNewDotToken,
                    onAdd = viewModel::addDotToken,
                    onRemove = viewModel::removeDotToken,
                    onBlur = viewModel::autoSaveIfReady,
                )
                Text(
                    stringResource(R.string.transports_cert_warning),
                    style = MaterialTheme.typography.bodySmall,
                    color = DigitDnsTheme.colors.warning,
                )
            }
            CertificateRow()
            Text(
                stringResource(R.string.transports_dot_firewall, state.dotPort),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun DnscryptCard(
    state: ClientTransportsUiState,
    viewModel: ClientTransportsViewModel,
    modifier: Modifier = Modifier,
) {
    PanelCard(modifier = modifier.fillMaxWidth(), contentPadding = 12.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            TransportHeader(
                title = stringResource(R.string.transports_dnscrypt),
                help = stringResource(R.string.transports_dnscrypt_help),
                enabled = state.dnscryptEnabled,
                onToggle = viewModel::setDnscryptEnabled,
                toggleEnabled = true,
                status = {
                    TransportStatus(state.dnscryptEnabled, state.dnscryptListening, false, false)
                },
            )
            OutlinedTextField(
                value = state.dnscryptPort,
                onValueChange = viewModel::setDnscryptPort,
                label = { Text(stringResource(R.string.transports_dnscrypt_port)) },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                modifier = Modifier.fillMaxWidth().saveTransportOnBlur(viewModel),
            )
            OutlinedTextField(
                value = state.dnscryptProviderName,
                onValueChange = viewModel::setDnscryptProviderName,
                label = { Text(stringResource(R.string.transports_dnscrypt_provider)) },
                placeholder = { Text(stringResource(R.string.transports_dnscrypt_provider_hint)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth().saveTransportOnBlur(viewModel),
            )
            Text(
                stringResource(R.string.transports_dnscrypt_stamp_label),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            CopyBox(
                value = state.dnscryptStamp,
                contentDescription = stringResource(R.string.transports_dnscrypt_copy_stamp),
            )
            if (state.dnscryptCertNotAfter > 0) {
                Text(
                    stringResource(
                        R.string.transports_dnscrypt_expires,
                        java.text.DateFormat.getDateInstance().format(java.util.Date(state.dnscryptCertNotAfter * 1000)),
                    ),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Text(
                stringResource(R.string.transports_dnscrypt_help_ports),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun MtlsCard(
    state: ClientTransportsUiState,
    viewModel: ClientTransportsViewModel,
    modifier: Modifier = Modifier,
) {
    PanelCard(modifier = modifier.fillMaxWidth(), contentPadding = 12.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            TransportHeader(
                title = stringResource(R.string.transports_mtls),
                help = stringResource(R.string.transports_mtls_help),
                enabled = state.mtlsEnabled,
                onToggle = viewModel::setMtlsEnabled,
                toggleEnabled = true,
                status = { EnforcedStatus(state.mtlsEnabled, false) },
            )
            OutlinedTextField(
                value = state.mtlsCaPem,
                onValueChange = viewModel::setMtlsCaPem,
                label = { Text(stringResource(R.string.transports_mtls_ca)) },
                minLines = 3,
                modifier = Modifier.fillMaxWidth().saveTransportOnBlur(viewModel),
            )
            if (state.mtlsEnabled && state.mtlsCaPem.isBlank()) {
                Text(
                    stringResource(R.string.transports_mtls_ca_required),
                    style = MaterialTheme.typography.bodySmall,
                    color = DigitDnsTheme.colors.warning,
                )
            }
            Text(
                stringResource(R.string.transports_mtls_note),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun TransportSaveStatus(state: ClientTransportsUiState) {
    val hint = stagedTransportHint(state)
    val text = when {
        state.transportSaving -> stringResource(R.string.transports_status_saving)
        hint != null -> hint
        state.stagedDirty -> "Applying…"
        else -> stringResource(R.string.transports_all_saved)
    }
    val color = when {
        state.transportSaving -> MaterialTheme.colorScheme.onSurfaceVariant
        hint != null -> DigitDnsTheme.colors.warning
        state.stagedDirty -> MaterialTheme.colorScheme.onSurfaceVariant
        else -> DigitDnsTheme.colors.success
    }
    Text(text, style = MaterialTheme.typography.bodySmall, color = color)
}

/** The reason a staged form is not applied yet, or null when it is complete. */
private fun stagedTransportHint(state: ClientTransportsUiState): String? =
    doqTokenHint(state) ?: dotTokenHint(state) ?: mtlsHint(state)

private fun doqTokenHint(state: ClientTransportsUiState): String? = when {
    !state.doqEnabled || state.doqAuth != "sni-token" -> null
    state.tokenDomain.isBlank() -> "DoQ SNI-token mode needs a token domain."
    state.tokens.isEmpty() -> "DoQ SNI-token mode needs at least one token."
    else -> null
}

private fun dotTokenHint(state: ClientTransportsUiState): String? = when {
    !state.dotEnabled || state.dotAuth != "sni-token" -> null
    state.dotTokenDomain.isBlank() -> "DoT SNI-token mode needs a token domain."
    state.dotTokens.isEmpty() -> "DoT SNI-token mode needs at least one token."
    else -> null
}

private fun mtlsHint(state: ClientTransportsUiState): String? =
    if (state.mtlsEnabled && state.mtlsCaPem.isBlank()) "Paste the client CA bundle to enable mTLS." else null

private fun Modifier.saveTransportOnBlur(viewModel: ClientTransportsViewModel): Modifier =
    onFocusChanged { if (!it.isFocused) viewModel.autoSaveIfReady() }

@Composable
private fun DoqHeader(state: ClientTransportsUiState, viewModel: ClientTransportsViewModel) {
    TransportHeader(
        title = stringResource(R.string.transports_doq),
        help = stringResource(R.string.transports_doq_help),
        enabled = state.doqEnabled,
        onToggle = viewModel::setDoqEnabled,
        toggleEnabled = true,
        status = {
            TransportStatus(state.doqEnabled, state.doqListening, false, false)
        },
    )
}

@Composable
private fun DoqPortAndAuth(state: ClientTransportsUiState, viewModel: ClientTransportsViewModel) {
    OutlinedTextField(
        value = state.doqPort,
        onValueChange = viewModel::setDoqPort,
        label = { Text(stringResource(R.string.transports_port)) },
        singleLine = true,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
        modifier = Modifier.fillMaxWidth().saveTransportOnBlur(viewModel),
    )
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        FilterChip(
            selected = state.doqAuth != "sni-token",
            onClick = { viewModel.setDoqAuth("ip") },
            label = { Text(stringResource(R.string.transports_auth_ip)) },
        )
        FilterChip(
            selected = state.doqAuth == "sni-token",
            onClick = { viewModel.setDoqAuth("sni-token") },
            label = { Text(stringResource(R.string.transports_auth_token)) },
        )
    }
}

/** The IP gate is shared by every IP-authenticated transport, so it is one
 *  note under the cards rather than repeated inside DoQ. */
@Composable
private fun IpAllowlistCard(state: ClientTransportsUiState) {
    PanelCard(modifier = Modifier.fillMaxWidth(), contentPadding = 12.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(3.dp)) {
            Text(
                stringResource(R.string.transports_ip_auth),
                style = MaterialTheme.typography.titleSmall,
            )
            Text(
                stringResource(R.string.transports_ip_shared_help),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                stringResource(
                    R.string.transports_doq_ip_summary,
                    state.allowlistClientAccounts,
                    state.allowlistClientEntries,
                    state.allowlistResolverEntries,
                ),
                style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            when {
                state.allowAll -> Text(
                    stringResource(R.string.transports_doq_ip_public_warning),
                    style = MaterialTheme.typography.bodySmall,
                    color = DigitDnsTheme.colors.warning,
                )
                state.allowlistClientEntries == 0 && state.allowlistResolverEntries == 0 -> Text(
                    stringResource(R.string.transports_doq_ip_empty_warning),
                    style = MaterialTheme.typography.bodySmall,
                    color = DigitDnsTheme.colors.warning,
                )
            }
        }
    }
}

@Composable
private fun CertificateRow() {
    Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Text(
            stringResource(R.string.transports_certificate),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.weight(1f),
        )
        Text(
            stringResource(R.string.transports_certificate_manager),
            style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun DoqTokenEditor(state: ClientTransportsUiState, viewModel: ClientTransportsViewModel) {
    TokenEditor(
        model = TokenEditorModel(
            domain = state.tokenDomain,
            domainHint = stringResource(R.string.transports_token_domain_hint),
            newToken = state.newToken,
            tokens = state.tokens,
            emptyLabel = stringResource(R.string.transports_no_tokens),
        ),
        onDomainChange = viewModel::setTokenDomain,
        onNewTokenChange = viewModel::setNewToken,
        onAdd = viewModel::addToken,
        onRemove = viewModel::removeToken,
        onBlur = viewModel::autoSaveIfReady,
    )
    Text(
        stringResource(R.string.transports_cert_warning),
        style = MaterialTheme.typography.bodySmall,
        color = DigitDnsTheme.colors.warning,
    )
}

private data class TokenEditorModel(
    val domain: String,
    val domainHint: String,
    val newToken: String,
    val tokens: List<String>,
    val emptyLabel: String,
)

@Composable
private fun TokenEditor(
    model: TokenEditorModel,
    onDomainChange: (String) -> Unit,
    onNewTokenChange: (String) -> Unit,
    onAdd: () -> Unit,
    onRemove: (String) -> Unit,
    onBlur: () -> Unit,
) {
    OutlinedTextField(
        value = model.domain,
        onValueChange = onDomainChange,
        label = { Text(stringResource(R.string.transports_token_domain)) },
        placeholder = { Text(model.domainHint) },
        singleLine = true,
        modifier = Modifier.fillMaxWidth().onFocusChanged { if (!it.isFocused) onBlur() },
    )
    Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        OutlinedTextField(
            value = model.newToken,
            onValueChange = onNewTokenChange,
            label = { Text(stringResource(R.string.transports_new_token)) },
            singleLine = true,
            modifier = Modifier.weight(1f),
        )
        Spacer(Modifier.width(8.dp))
        Button(onClick = onAdd) {
            Text(stringResource(R.string.transports_add_token))
        }
    }
    if (model.tokens.isEmpty()) {
        Text(
            model.emptyLabel,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    } else {
        // Bounded so ten tokens do not turn the card into a page of its own.
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .heightIn(max = 88.dp)
                .verticalScroll(rememberScrollState()),
        ) {
            model.tokens.forEach { token ->
                Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        token,
                        modifier = Modifier.weight(1f),
                        style = MaterialTheme.typography.bodyMedium.copy(fontFamily = PanelMono),
                        color = MaterialTheme.colorScheme.primary,
                    )
                    IconButton(onClick = { onRemove(token) }) {
                        Icon(Icons.Filled.Close, contentDescription = "Remove $token")
                    }
                }
            }
        }
    }
}

@Composable
private fun TransportStatus(enabled: Boolean, listening: Boolean, saving: Boolean, dirty: Boolean) {
    val text = when {
        saving -> stringResource(R.string.transports_status_saving)
        dirty -> stringResource(R.string.transports_status_unsaved)
        listening -> stringResource(R.string.transports_status_listening)
        enabled -> stringResource(R.string.transports_status_pending)
        else -> stringResource(R.string.transports_status_off)
    }
    val color: Color = when {
        saving -> DigitDnsTheme.colors.textMuted
        dirty -> DigitDnsTheme.colors.warning
        listening -> DigitDnsTheme.colors.success
        enabled -> DigitDnsTheme.colors.warning
        else -> DigitDnsTheme.colors.textMuted
    }
    StatusBadge(text = text, color = color)
}
