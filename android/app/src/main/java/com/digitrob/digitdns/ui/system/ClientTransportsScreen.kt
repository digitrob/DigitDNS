package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
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
 * Client-facing DoH3/DoQ settings, the phone counterpart of the web's
 * Client Encrypted Transports card. The switches are staged and saved together;
 * DoQ's port, auth mode and SNI-token list are included because a token-mode
 * install cannot start without them.
 */
@Composable
fun ClientTransportsScreen(viewModel: ClientTransportsViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()

    PanelRefreshBox(
        isRefreshing = state.isLoading || state.isRefreshing,
        onRefresh = viewModel::load,
        modifier = Modifier.fillMaxSize(),
    ) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            PanelCard(modifier = Modifier.fillMaxWidth()) {
                Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Text(stringResource(R.string.transports_title), style = MaterialTheme.typography.titleMedium)
                    Text(
                        stringResource(R.string.transports_help),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }

            when {
                state.isLoading -> CircularProgressIndicator(color = MaterialTheme.colorScheme.primary)
                else -> {
                    Doh3Card(state, viewModel)
                    DoqCard(state, viewModel)

                    state.error?.let {
                        Text(it, color = DigitDnsTheme.colors.danger, style = MaterialTheme.typography.bodySmall)
                    }

                    Button(
                        onClick = viewModel::save,
                        enabled = !state.isSaving,
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        if (state.isSaving) {
                            CircularProgressIndicator(
                                modifier = Modifier.padding(end = 8.dp),
                                strokeWidth = 2.dp,
                                color = MaterialTheme.colorScheme.onPrimary,
                            )
                        }
                        Text(stringResource(R.string.transports_save))
                    }
                    if (state.saved) {
                        Text(
                            stringResource(R.string.transports_saved),
                            color = MaterialTheme.colorScheme.primary,
                            style = MaterialTheme.typography.bodySmall,
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun Doh3Card(state: ClientTransportsUiState, viewModel: ClientTransportsViewModel) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Column(modifier = Modifier.weight(1f)) {
                    Text(stringResource(R.string.transports_doh3), style = MaterialTheme.typography.titleSmall)
                    Text(
                        stringResource(R.string.transports_doh3_help),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Switch(checked = state.doh3Enabled, onCheckedChange = viewModel::setDoh3Enabled)
            }
            TransportStatus(state.doh3Enabled, state.doh3Listening)
            Text(
                stringResource(R.string.transports_doh3_port),
                style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun DoqCard(state: ClientTransportsUiState, viewModel: ClientTransportsViewModel) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            DoqHeader(state, viewModel)
            DoqPortAndAuth(state, viewModel)
            if (state.doqAuth == "sni-token") {
                DoqTokenEditor(state, viewModel)
            }
            TransportStatus(state.doqEnabled, state.doqListening)
            Text(
                stringResource(R.string.transports_firewall),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

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
        Switch(checked = state.doqEnabled, onCheckedChange = viewModel::setDoqEnabled)
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
        modifier = Modifier.fillMaxWidth(),
    )
    Text(stringResource(R.string.transports_auth), style = MaterialTheme.typography.labelMedium)
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

@Composable
private fun DoqTokenEditor(state: ClientTransportsUiState, viewModel: ClientTransportsViewModel) {
    OutlinedTextField(
        value = state.tokenDomain,
        onValueChange = viewModel::setTokenDomain,
        label = { Text(stringResource(R.string.transports_token_domain)) },
        placeholder = { Text(stringResource(R.string.transports_token_domain_hint)) },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
    Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        OutlinedTextField(
            value = state.newToken,
            onValueChange = viewModel::setNewToken,
            label = { Text(stringResource(R.string.transports_new_token)) },
            singleLine = true,
            modifier = Modifier.weight(1f),
        )
        Spacer(Modifier.width(8.dp))
        Button(onClick = viewModel::addToken) {
            Text(stringResource(R.string.transports_add_token))
        }
    }
    if (state.tokens.isEmpty()) {
        Text(
            stringResource(R.string.transports_no_tokens),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    } else {
        state.tokens.forEach { token ->
            Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Text(
                    token,
                    modifier = Modifier.weight(1f),
                    style = MaterialTheme.typography.bodyMedium.copy(fontFamily = PanelMono),
                    color = MaterialTheme.colorScheme.primary,
                )
                IconButton(onClick = { viewModel.removeToken(token) }) {
                    Icon(Icons.Filled.Close, contentDescription = "Remove $token")
                }
            }
        }
    }
    Text(
        stringResource(R.string.transports_cert_warning),
        style = MaterialTheme.typography.bodySmall,
        color = DigitDnsTheme.colors.warning,
    )
}

@Composable
private fun TransportStatus(enabled: Boolean, listening: Boolean) {
    val text = when {
        listening -> stringResource(R.string.transports_status_listening)
        enabled -> stringResource(R.string.transports_status_pending)
        else -> stringResource(R.string.transports_status_off)
    }
    val color: Color = when {
        listening -> DigitDnsTheme.colors.success
        enabled -> DigitDnsTheme.colors.warning
        else -> DigitDnsTheme.colors.textMuted
    }
    StatusBadge(text = text, color = color)
}
