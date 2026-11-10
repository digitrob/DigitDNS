package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
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
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono

/**
 * Client-facing encrypted transports, the phone counterpart of the web's
 * Client Encrypted Transports card: DoH3, DoQ, DoT authentication, DNSCrypt
 * and mTLS. DoH3 applies as soon as its switch moves; everything else is staged
 * and committed together by one Save action.
 */
@Composable
fun ClientTransportsScreen(viewModel: ClientTransportsViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()

    PanelRefreshBox(
        isRefreshing = state.isLoading || state.isRefreshing,
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
                TransportsIntro()
                if (state.isLoading) {
                    CircularProgressIndicator(color = MaterialTheme.colorScheme.primary)
                } else {
                    if (wide) {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                            verticalAlignment = Alignment.Top,
                        ) {
                            Doh3Card(state, viewModel, Modifier.weight(1f))
                            DoqCard(state, viewModel, Modifier.weight(1f))
                        }
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                            verticalAlignment = Alignment.Top,
                        ) {
                            DotCard(state, viewModel, Modifier.weight(1f))
                            DnscryptCard(state, viewModel, Modifier.weight(1f))
                        }
                    } else {
                        Doh3Card(state, viewModel)
                        DoqCard(state, viewModel)
                        DotCard(state, viewModel)
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
// cost vertical space. One muted line keeps the certificate dependency visible.
@Composable
private fun TransportsIntro() {
    Text(
        stringResource(R.string.transports_help),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

@Composable
private fun Doh3Card(
    state: ClientTransportsUiState,
    viewModel: ClientTransportsViewModel,
    modifier: Modifier = Modifier,
) {
    PanelCard(modifier = modifier.fillMaxWidth(), contentPadding = 12.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Text(
                    stringResource(R.string.transports_doh3),
                    style = MaterialTheme.typography.titleSmall,
                    modifier = Modifier.weight(1f),
                )
                TransportStatus(state.appliedDoh3Enabled, state.doh3Listening, state.doh3Saving, false)
                Spacer(Modifier.width(8.dp))
                Switch(
                    checked = state.doh3Enabled,
                    onCheckedChange = viewModel::setDoh3Enabled,
                    enabled = !state.doh3Saving && !state.transportSaving,
                )
            }
            Text(
                stringResource(R.string.transports_doh3_help),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                stringResource(R.string.transports_doh3_port, state.doh3Port),
                style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            CertificateRow()
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
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Column(modifier = Modifier.weight(1f)) {
                    Text(
                        stringResource(R.string.transports_firewall),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
    }
}

/** DoT has no on/off switch: it serves whenever the DoH/DoT certificate is installed. */
@Composable
private fun DotCard(
    state: ClientTransportsUiState,
    viewModel: ClientTransportsViewModel,
    modifier: Modifier = Modifier,
) {
    PanelCard(modifier = modifier.fillMaxWidth(), contentPadding = 12.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(stringResource(R.string.transports_dot), style = MaterialTheme.typography.titleSmall)
            Text(
                stringResource(R.string.transports_dot_help, state.dotPort),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
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
                    domain = state.dotTokenDomain,
                    onDomainChange = viewModel::setDotTokenDomain,
                    domainHint = "dot.example.com",
                    newToken = state.newDotToken,
                    onNewTokenChange = viewModel::setNewDotToken,
                    onAdd = viewModel::addDotToken,
                    tokens = state.dotTokens,
                    onRemove = viewModel::removeDotToken,
                    emptyLabel = stringResource(R.string.transports_no_dot_tokens),
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
    val clipboard = LocalClipboardManager.current
    PanelCard(modifier = modifier.fillMaxWidth(), contentPadding = 12.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Column(modifier = Modifier.weight(1f)) {
                    Text(stringResource(R.string.transports_dnscrypt), style = MaterialTheme.typography.titleSmall)
                    Text(
                        stringResource(R.string.transports_dnscrypt_help),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                TransportStatus(state.dnscryptEnabled, state.dnscryptListening, state.transportSaving, state.stagedDirty)
                Spacer(Modifier.width(8.dp))
                Switch(
                    checked = state.dnscryptEnabled,
                    onCheckedChange = viewModel::setDnscryptEnabled,
                    enabled = !state.transportSaving && !state.doh3Saving,
                )
            }
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
                    text = state.dnscryptStamp.ifBlank { stringResource(R.string.transports_dnscrypt_stamp_empty) },
                    style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
                    color = MaterialTheme.colorScheme.primary,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f),
                )
                if (state.dnscryptStamp.isNotBlank()) {
                    IconButton(
                        onClick = { clipboard.setText(AnnotatedString(state.dnscryptStamp)) },
                    ) {
                        Icon(
                            Icons.Filled.ContentCopy,
                            contentDescription = stringResource(R.string.transports_dnscrypt_copy_stamp),
                        )
                    }
                }
            }
            Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Text(
                    stringResource(R.string.transports_status),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.weight(1f),
                )
                Text(
                    stringResource(
                        if (state.dnscryptListening) R.string.transports_status_listening else R.string.transports_status_off,
                    ),
                    style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
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
            Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Column(modifier = Modifier.weight(1f)) {
                    Text(stringResource(R.string.transports_mtls), style = MaterialTheme.typography.titleSmall)
                    Text(
                        stringResource(R.string.transports_mtls_help),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Switch(
                    checked = state.mtlsEnabled,
                    onCheckedChange = viewModel::setMtlsEnabled,
                    enabled = !state.transportSaving && !state.doh3Saving,
                )
            }
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
private fun stagedTransportHint(state: ClientTransportsUiState): String? = when {
    state.doqAuth == "sni-token" && state.tokenDomain.isBlank() ->
        "DoQ SNI-token mode needs a token domain."
    state.doqAuth == "sni-token" && state.tokens.isEmpty() ->
        "DoQ SNI-token mode needs at least one token."
    state.dotAuth == "sni-token" && state.dotTokenDomain.isBlank() ->
        "DoT SNI-token mode needs a token domain."
    state.dotAuth == "sni-token" && state.dotTokens.isEmpty() ->
        "DoT SNI-token mode needs at least one token."
    state.mtlsEnabled && state.mtlsCaPem.isBlank() ->
        "Paste the client CA bundle to enable mTLS."
    else -> null
}

private fun Modifier.saveTransportOnBlur(viewModel: ClientTransportsViewModel): Modifier =
    onFocusChanged { if (!it.isFocused) viewModel.autoSaveIfReady() }

@Composable
private fun DoqHeader(state: ClientTransportsUiState, viewModel: ClientTransportsViewModel) {
    Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Column(modifier = Modifier.weight(1f)) {
            Text(stringResource(R.string.transports_doq), style = MaterialTheme.typography.titleSmall)
            Text(
                stringResource(R.string.transports_doq_help),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        TransportStatus(state.appliedDoqEnabled, state.doqListening, state.transportSaving, state.stagedDirty)
        Spacer(Modifier.width(8.dp))
        Switch(
            checked = state.doqEnabled,
            onCheckedChange = viewModel::setDoqEnabled,
            enabled = !state.transportSaving && !state.doh3Saving,
        )
    }
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
        domain = state.tokenDomain,
        onDomainChange = viewModel::setTokenDomain,
        domainHint = stringResource(R.string.transports_token_domain_hint),
        newToken = state.newToken,
        onNewTokenChange = viewModel::setNewToken,
        onAdd = viewModel::addToken,
        tokens = state.tokens,
        onRemove = viewModel::removeToken,
        emptyLabel = stringResource(R.string.transports_no_tokens),
        onBlur = viewModel::autoSaveIfReady,
    )
    Text(
        stringResource(R.string.transports_cert_warning),
        style = MaterialTheme.typography.bodySmall,
        color = DigitDnsTheme.colors.warning,
    )
}

@Composable
private fun TokenEditor(
    domain: String,
    onDomainChange: (String) -> Unit,
    domainHint: String,
    newToken: String,
    onNewTokenChange: (String) -> Unit,
    onAdd: () -> Unit,
    tokens: List<String>,
    onRemove: (String) -> Unit,
    emptyLabel: String,
    onBlur: () -> Unit,
) {
    OutlinedTextField(
        value = domain,
        onValueChange = onDomainChange,
        label = { Text(stringResource(R.string.transports_token_domain)) },
        placeholder = { Text(domainHint) },
        singleLine = true,
        modifier = Modifier.fillMaxWidth().onFocusChanged { if (!it.isFocused) onBlur() },
    )
    Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        OutlinedTextField(
            value = newToken,
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
    if (tokens.isEmpty()) {
        Text(
            emptyLabel,
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
            tokens.forEach { token ->
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
