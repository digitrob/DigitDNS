package com.digitrob.digitdns.ui.connect

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.R
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.SettingsLoadingSkeleton
import com.digitrob.digitdns.ui.components.QrCode
import com.digitrob.digitdns.ui.components.clickableText
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono

/**
 * The Connect tab: choose a client, then show the subscription link and a QR to
 * share. This is the phone-native equivalent of the panel's Connect Guide.
 */
@Composable
fun ConnectScreen(viewModel: ConnectViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val clipboard = LocalClipboardManager.current

    PanelRefreshBox(
        isRefreshing = false,
        onRefresh = viewModel::load,
        modifier = Modifier.fillMaxSize(),
    ) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
        when {
            state.isLoading -> SettingsLoadingSkeleton(cards = 2)
            state.error != null -> Text(state.error!!, color = DigitDnsTheme.colors.danger)
            else -> {
                if (state.clients.isEmpty()) {
                    Text(
                        "No clients yet. Create one in the Users tab to share a subscription link.",
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        style = MaterialTheme.typography.bodySmall,
                    )
                } else {
                    ClientSelector(state, viewModel)

                    val url = state.subscriptionUrl
                    if (url.isBlank()) {
                        Text(
                            text = stringResource(R.string.connect_no_origin),
                            color = DigitDnsTheme.colors.warning,
                            style = MaterialTheme.typography.bodySmall,
                        )
                    } else {
                        PanelCard(modifier = Modifier.fillMaxWidth()) {
                            Column(
                                horizontalAlignment = Alignment.CenterHorizontally,
                                verticalArrangement = Arrangement.spacedBy(12.dp),
                                modifier = Modifier.fillMaxWidth(),
                            ) {
                                QrCode(content = url)
                                Text(
                                    text = url,
                                    style = MaterialTheme.typography.bodySmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                                Text(
                                    text = stringResource(R.string.connect_tap_to_copy),
                                    style = MaterialTheme.typography.labelLarge,
                                    color = MaterialTheme.colorScheme.primary,
                                    modifier = Modifier.clickableText { clipboard.setText(AnnotatedString(url)) },
                                )
                            }
                        }
                    }
                }

                ConnectGuides(state, clipboard)
            }
        }
    }
    }
}

/** The four panel guide cards, shown once the server address data has loaded. */
@Composable
private fun ConnectGuides(
    state: ConnectUiState,
    clipboard: androidx.compose.ui.platform.ClipboardManager,
) {
    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(
            text = stringResource(R.string.connect_how_to),
            style = MaterialTheme.typography.titleMedium,
            color = MaterialTheme.colorScheme.onBackground,
        )
        GuideCard(
            title = "Windows 10 / 11",
            steps = listOf(
                "Settings › Network & Internet › Wi-Fi / Ethernet.",
                "Edit DNS assignment › Manual.",
                "Turn on IPv4 and enter your server IP:",
            ),
            code = state.serverIp,
            clipboard = clipboard,
        )
        GuideCard(
            title = "PlayStation 5 / Xbox",
            steps = listOf(
                "Settings › Network › Set Up Internet Connection.",
                "Select Wi-Fi/LAN › Advanced Settings › DNS: Manual.",
                "Set Primary DNS to your server IP:",
            ),
            code = state.serverIp,
            clipboard = clipboard,
        )
        GuideCard(
            title = "Android Private DNS (DoT)",
            steps = listOf(
                "Settings › Network & Internet › Private DNS.",
                "Select Private DNS provider hostname and enter:",
            ),
            code = state.dotHostname,
            clipboard = clipboard,
        )
        GuideCard(
            title = "Browser Secure DNS (DoH)",
            steps = listOf(
                "Browser Settings › Privacy & Security › Use secure DNS.",
                "Choose Custom and paste your DoH URL:",
            ),
            code = state.dohUrl,
            note = if (state.doh3Enabled) {
                "HTTP/3 (DoH3) is enabled on this server. Clients that support it negotiate it automatically with the same URL."
            } else null,
            clipboard = clipboard,
        )
        if (state.doqEnabled && state.doqUrl.isNotBlank()) {
            GuideCard(
                title = "DNS-over-QUIC (DoQ)",
                steps = listOf(
                    "Use a client or router that supports RFC 9250 DoQ.",
                    "Enter this endpoint:",
                ),
                code = state.doqUrl,
                note = if (state.doqAuth == "sni-token" && state.doqTokenDomain.isNotBlank()) {
                    "SNI-token mode uses <token>.${state.doqTokenDomain} in place of the hostname."
                } else null,
                clipboard = clipboard,
            )
        }
    }
}

/**
 * One of the panel's four Connect Guide cards: a numbered recipe and a copyable value.
 * The value is only shown once it is known — an empty code box reads as "your server
 * has no address", which is worse than no box at all.
 */
@Composable
private fun GuideCard(
    title: String,
    steps: List<String>,
    code: String,
    note: String? = null,
    clipboard: androidx.compose.ui.platform.ClipboardManager,
) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(title, style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.onSurface)
            steps.forEachIndexed { index, step ->
                Text(
                    text = "${index + 1}. $step",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (code.isNotBlank()) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(
                        text = code,
                        style = MaterialTheme.typography.bodyMedium.copy(fontFamily = PanelMono),
                        color = MaterialTheme.colorScheme.primary,
                        modifier = Modifier.weight(1f),
                    )
                    Text(
                        text = stringResource(R.string.action_copy),
                        style = MaterialTheme.typography.labelLarge,
                        color = MaterialTheme.colorScheme.primary,
                        modifier = Modifier.clickableText { clipboard.setText(AnnotatedString(code)) },
                    )
                }
            }
            note?.takeIf { it.isNotBlank() }?.let {
                Text(
                    text = it,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@OptIn(androidx.compose.foundation.layout.ExperimentalLayoutApi::class)
@Composable
private fun ClientSelector(state: ConnectUiState, viewModel: ConnectViewModel) {
    FlowRow(
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        state.clients.forEach { client ->
            FilterChip(
                selected = client.id == state.selectedClientId,
                onClick = { viewModel.selectClient(client.id) },
                label = { Text(client.name.ifBlank { client.id }) },
            )
        }
    }
}
