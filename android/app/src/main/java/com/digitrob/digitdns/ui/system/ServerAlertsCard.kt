package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.core.network.AlertChannelDto
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

private val ALERT_KINDS = listOf(
    "ntfy" to "ntfy",
    "telegram" to "Telegram",
    "discord" to "Discord",
    "slack" to "Slack",
    "gotify" to "Gotify",
    "webhook" to "Webhook",
)

private val ALERT_EVENTS = listOf(
    "agents" to "Relay agents",
    "egresses" to "Egresses",
    "relay_mode" to "Relay mode",
    "clients" to "Accounts",
    "upstreams" to "Upstreams",
    "certificates" to "Certificates",
)

@Suppress("LongMethod", "CyclomaticComplexMethod")
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun ServerAlertsCard(viewModel: ServerAlertsViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val s = state.settings

    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(modifier = Modifier.weight(1f)) {
                    Text("Server alerts", style = MaterialTheme.typography.titleMedium)
                    Text(
                        "Send relay, egress, account and upstream events to your own channels.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Switch(checked = s.enabled, onCheckedChange = viewModel::setEnabled)
            }

            s.channels.forEachIndexed { index, channel ->
                AlertChannelRow(
                    channel = channel,
                    onChange = { transform -> viewModel.updateChannel(index, transform) },
                    onRemove = { viewModel.removeChannel(index) },
                )
            }

            TextButton(onClick = viewModel::addChannel) {
                Icon(Icons.Filled.Add, contentDescription = null, modifier = Modifier.size(16.dp))
                Spacer(Modifier.width(4.dp))
                Text("Add channel")
            }

            OutlinedTextField(
                value = s.cooldownMinutes.toString(),
                onValueChange = viewModel::setCooldown,
                label = { Text("Repeat cooldown (minutes)") },
                singleLine = true,
                keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(keyboardType = KeyboardType.Number),
                modifier = Modifier.fillMaxWidth(),
            )

            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                ThresholdField(
                    value = s.thresholds.agentOfflineMinutes.toString(),
                    label = "Agent offline (min)",
                    modifier = Modifier.weight(1f),
                    onChange = { v -> viewModel.setThreshold { it.copy(agentOfflineMinutes = v.toIntOrNull() ?: 2) } },
                )
                ThresholdField(
                    value = s.thresholds.egressOfflineMinutes.toString(),
                    label = "Egress offline (min)",
                    modifier = Modifier.weight(1f),
                    onChange = { v -> viewModel.setThreshold { it.copy(egressOfflineMinutes = v.toIntOrNull() ?: 2) } },
                )
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                ThresholdField(
                    value = s.thresholds.certExpiryDays.toString(),
                    label = "Cert warning (days)",
                    modifier = Modifier.weight(1f),
                    onChange = { v -> viewModel.setThreshold { it.copy(certExpiryDays = v.toIntOrNull() ?: 30) } },
                )
                ThresholdField(
                    value = s.thresholds.upstreamErrorPercent.toString(),
                    label = "Upstream error %",
                    modifier = Modifier.weight(1f),
                    onChange = { v -> viewModel.setThreshold { it.copy(upstreamErrorPercent = v.toIntOrNull() ?: 50) } },
                )
            }

            Text("Events", style = MaterialTheme.typography.labelMedium)
            FlowRow(
                horizontalArrangement = Arrangement.spacedBy(6.dp),
                verticalArrangement = Arrangement.spacedBy(4.dp),
            ) {
                ALERT_EVENTS.forEach { (key, label) ->
                    FilterChip(
                        selected = s.events[key] != false,
                        onClick = { viewModel.setEvent(key, s.events[key] == false) },
                        label = { Text(label) },
                    )
                }
            }

            if (state.history.isNotEmpty()) {
                Text("Recent deliveries", style = MaterialTheme.typography.labelMedium)
                state.history.take(5).forEach { h ->
                    Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                        Text(
                            if (h.ok) "OK" else "FAIL",
                            style = MaterialTheme.typography.labelSmall.copy(fontFamily = com.digitrob.digitdns.ui.theme.PanelMono),
                            color = if (h.ok) DigitDnsTheme.colors.success else DigitDnsTheme.colors.danger,
                        )
                        Text(
                            "${h.channel} · ${h.title.ifBlank { h.event }}",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier = Modifier.weight(1f),
                            maxLines = 1,
                        )
                        Text(
                            if (h.attempts > 1) "${h.attempts}x" else "",
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    if (h.error.isNotBlank()) {
                        Text(h.error, style = MaterialTheme.typography.labelSmall, color = DigitDnsTheme.colors.danger, maxLines = 1)
                    }
                }
            }

            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = viewModel::save, enabled = !state.isSaving && !state.isTesting) {
                    if (state.isSaving) {
                        CircularProgressIndicator(modifier = Modifier.size(14.dp), strokeWidth = 2.dp)
                        Spacer(Modifier.width(6.dp))
                    }
                    Text("Save alerts")
                }
                OutlinedButton(onClick = viewModel::test, enabled = !state.isSaving && !state.isTesting) {
                    if (state.isTesting) {
                        CircularProgressIndicator(modifier = Modifier.size(14.dp), strokeWidth = 2.dp)
                        Spacer(Modifier.width(6.dp))
                    }
                    Text("Send test")
                }
            }
            state.error?.let {
                Text(it, style = MaterialTheme.typography.bodySmall, color = DigitDnsTheme.colors.danger)
            }
            state.message?.let {
                Text(it, style = MaterialTheme.typography.bodySmall, color = DigitDnsTheme.colors.success)
            }
        }
    }
}

@Composable
private fun ThresholdField(
    value: String,
    label: String,
    modifier: Modifier,
    onChange: (String) -> Unit,
) {
    OutlinedTextField(
        value = value,
        onValueChange = { onChange(it.filter { c -> c.isDigit() }) },
        label = { Text(label) },
        singleLine = true,
        keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(keyboardType = KeyboardType.Number),
        modifier = modifier,
    )
}

@Composable
private fun AlertChannelRow(
    channel: AlertChannelDto,
    onChange: ((AlertChannelDto) -> AlertChannelDto) -> Unit,
    onRemove: () -> Unit,
) {
    var expanded by remember { mutableStateOf(false) }
    Column(
        modifier = Modifier.fillMaxWidth().padding(8.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        // The channel's own controls: kind selector, channel name, enable switch and
        // delete. The row fills the card's width so the weighted name field has real
        // space to take; a wrap-content row gave the weight nothing to distribute and
        // the field collapsed to nothing on narrow screens.
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            androidx.compose.foundation.layout.Box {
                OutlinedButton(onClick = { expanded = true }) {
                    Text(ALERT_KINDS.firstOrNull { it.first == channel.kind }?.second ?: channel.kind)
                    Icon(Icons.Filled.ArrowDropDown, contentDescription = null)
                }
                DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
                    ALERT_KINDS.forEach { (kind, label) ->
                        DropdownMenuItem(
                            text = { Text(label) },
                            onClick = {
                                expanded = false
                                onChange { it.copy(kind = kind) }
                            },
                        )
                    }
                }
            }
            OutlinedTextField(
                value = channel.name,
                onValueChange = { value -> onChange { it.copy(name = value) } },
                label = { Text("Channel name") },
                singleLine = true,
                modifier = Modifier.weight(1f),
            )
            Switch(checked = channel.enabled, onCheckedChange = { value -> onChange { it.copy(enabled = value) } })
            IconButton(onClick = onRemove) {
                Icon(Icons.Filled.Delete, contentDescription = "Remove channel", tint = DigitDnsTheme.colors.danger)
            }
        }
        OutlinedTextField(
            value = channel.url,
            onValueChange = { value -> onChange { it.copy(url = value) } },
            label = { Text("Webhook / ntfy / Gotify URL") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        OutlinedTextField(
            value = if (channel.token.startsWith("••")) "" else channel.token,
            onValueChange = { value -> onChange { it.copy(token = value) } },
            label = { Text("Bot token / Gotify token") },
            placeholder = { Text(if (channel.token.startsWith("••")) "Saved — leave blank to keep" else "") },
            singleLine = true,
            visualTransformation = PasswordVisualTransformation(),
            modifier = Modifier.fillMaxWidth(),
        )
        OutlinedTextField(
            value = channel.chatId,
            onValueChange = { value -> onChange { it.copy(chatId = value) } },
            label = { Text("Telegram chat ID") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
    }
}
