package com.digitrob.digitdns.ui.rules

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.PanelSwitch
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

/**
 * The resolver access gate. The mode switch is the headline: open (allow_all) lets
 * any client resolve, while allowlist mode restricts to the CIDRs below. The DoH
 * token list gates the DNS-over-HTTPS endpoint the same box serves.
 */
@Composable
fun AccessScreen(viewModel: AccessViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()

    PanelRefreshBox(
        isRefreshing = state.isLoading,
        onRefresh = viewModel::load,
        modifier = Modifier.fillMaxSize(),
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
        state.savedMessage?.let { message ->
            PanelCard {
                Row(modifier = Modifier.fillMaxWidth()) {
                    Text(message, color = MaterialTheme.colorScheme.primary, modifier = Modifier.weight(1f))
                    TextButton(onClick = viewModel::dismissMessage) { Text("OK") }
                }
            }
        }

        if (state.isLoading) {
            CircularProgressIndicator(color = MaterialTheme.colorScheme.primary)
            return@Column
        }

        ModeCard(allowAll = state.allowAll, onChange = viewModel::onAllowAllChange)

        OutlinedTextField(
            value = state.allowedIps,
            onValueChange = viewModel::onAllowedIpsChange,
            label = { Text("Allowed IPs / CIDRs") },
            supportingText = {
                Text(
                    text = "One per line. Entries are stored in canonical CIDR form.",
                    style = MaterialTheme.typography.bodySmall,
                )
            },
            enabled = !state.allowAll,
            minLines = 3,
            modifier = Modifier.fillMaxWidth().heightIn(min = 96.dp),
        )

        OutlinedTextField(
            value = state.dohTokens,
            onValueChange = viewModel::onDohTokensChange,
            label = { Text("DoH tokens") },
            supportingText = {
                Text(
                    text = "One per line. Required by the DoH endpoint when set.",
                    style = MaterialTheme.typography.bodySmall,
                )
            },
            minLines = 2,
            modifier = Modifier.fillMaxWidth().heightIn(min = 72.dp),
        )

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
            Text("Save access settings")
        }
    }
    }
}

/** The mode switch: the one control that changes who may query the resolver at all. */
@Composable
private fun ModeCard(allowAll: Boolean, onChange: (Boolean) -> Unit) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        PanelSwitch(
            label = "Allow all clients",
            checked = allowAll,
            onCheckedChange = onChange,
            supporting = if (allowAll) {
                "Any client that can reach this server may resolve."
            } else {
                "Only the allowlisted addresses below may resolve."
            },
        )
    }
}
