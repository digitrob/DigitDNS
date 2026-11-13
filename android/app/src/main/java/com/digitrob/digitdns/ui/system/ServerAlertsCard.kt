package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Forum
import androidx.compose.material.icons.filled.Link
import androidx.compose.material.icons.filled.Notifications
import androidx.compose.material.icons.filled.NotificationsActive
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.filled.Tag
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
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.core.network.AlertChannelDto
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

private data class AlertKind(
    val key: String,
    val label: String,
    val icon: ImageVector,
)

private val ALERT_KINDS = listOf(
    AlertKind("ntfy", "ntfy", Icons.Filled.Notifications),
    AlertKind("telegram", "Telegram", Icons.AutoMirrored.Filled.Send),
    AlertKind("discord", "Discord", Icons.Filled.Forum),
    AlertKind("slack", "Slack", Icons.Filled.Tag),
    AlertKind("gotify", "Gotify", Icons.Filled.NotificationsActive),
    AlertKind("webhook", "Webhook", Icons.Filled.Link),
)

private fun alertKind(key: String): AlertKind =
    ALERT_KINDS.firstOrNull { it.key == key } ?: ALERT_KINDS.first()

/** One tint per destination, taken from the panel palette so the chips match the theme. */
@Composable
private fun alertKindTint(kind: AlertKind): Color = when (kind.key) {
    "ntfy" -> DigitDnsTheme.colors.info
    "telegram" -> DigitDnsTheme.colors.blue
    "discord" -> DigitDnsTheme.colors.discord
    "slack" -> DigitDnsTheme.colors.purple
    "gotify" -> DigitDnsTheme.colors.warning
    else -> DigitDnsTheme.colors.success
}

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

            if (s.channels.isEmpty()) {
                Text(
                    "No channels yet — add one to receive alerts.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
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
    val colors = DigitDnsTheme.colors
    val kind = alertKind(channel.kind)
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(colors.bgTile)
            .border(1.dp, colors.borderColor, RoundedCornerShape(12.dp))
            .padding(10.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        // Header: the destination pictogram opens the type menu, then the enable
        // switch and delete. The channel name keeps its own full-width row below, so
        // it can never be squeezed by the controls beside it on a narrow phone.
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            AlertKindPicker(
                selected = kind,
                onPick = { key -> onChange { it.copy(kind = key) } },
            )
            Spacer(Modifier.weight(1f))
            Text(
                "Enabled",
                style = MaterialTheme.typography.labelSmall,
                color = colors.textMuted,
            )
            Switch(
                checked = channel.enabled,
                onCheckedChange = { value -> onChange { it.copy(enabled = value) } },
            )
            IconButton(onClick = onRemove) {
                Icon(Icons.Filled.Delete, contentDescription = "Remove channel", tint = colors.danger)
            }
        }
        AlertChannelFields(channel = channel, onChange = onChange)
    }
}

/**
 * The channel-type control: a pictogram once a type is chosen, and a menu that
 * pairs every option with the same pictogram so the row never relies on a text
 * label that a narrow phone would clip.
 */
@Composable
private fun AlertKindPicker(
    selected: AlertKind,
    onPick: (String) -> Unit,
) {
    var expanded by remember { mutableStateOf(false) }
    val colors = DigitDnsTheme.colors
    Box {
        OutlinedButton(
            onClick = { expanded = true },
            contentPadding = PaddingValues(horizontal = 10.dp, vertical = 6.dp),
        ) {
            Icon(
                imageVector = selected.icon,
                contentDescription = "Channel type: ${selected.label}",
                tint = alertKindTint(selected),
                modifier = Modifier.size(18.dp),
            )
            Icon(
                imageVector = Icons.Filled.ArrowDropDown,
                contentDescription = null,
                tint = colors.textMuted,
                modifier = Modifier.size(14.dp),
            )
        }
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            ALERT_KINDS.forEach { option ->
                DropdownMenuItem(
                    text = { Text(option.label) },
                    leadingIcon = {
                        Icon(
                            imageVector = option.icon,
                            contentDescription = null,
                            tint = alertKindTint(option),
                            modifier = Modifier.size(18.dp),
                        )
                    },
                    trailingIcon = {
                        if (option.key == selected.key) {
                            Icon(
                                Icons.Filled.Check,
                                contentDescription = null,
                                modifier = Modifier.size(16.dp),
                            )
                        }
                    },
                    onClick = {
                        expanded = false
                        onPick(option.key)
                    },
                )
            }
        }
    }
}

@Composable
private fun AlertChannelFields(
    channel: AlertChannelDto,
    onChange: ((AlertChannelDto) -> AlertChannelDto) -> Unit,
) {
    OutlinedTextField(
        value = channel.name,
        onValueChange = { value -> onChange { it.copy(name = value) } },
        label = { Text("Channel name") },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
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
