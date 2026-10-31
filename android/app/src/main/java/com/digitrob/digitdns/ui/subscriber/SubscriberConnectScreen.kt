package com.digitrob.digitdns.ui.subscriber

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.Lan
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.digitrob.digitdns.core.network.SubscriberStatusDto
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono

private enum class ConnectSection(val label: String) {
    ADDRESSES("Addresses"),
    ENDPOINTS("Endpoints"),
    GUIDE("Guide"),
}

/** Registered addresses, the copy-card endpoints, and the full device guide. */
@Composable
internal fun ConnectTab(
    state: SubscriberUiState,
    onCopy: (String) -> Unit,
) {
    var section by rememberSaveable { mutableStateOf(ConnectSection.ADDRESSES) }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(bottom = 8.dp),
    ) {
        Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            ConnectSection.entries.forEach { entry ->
                FilterChip(
                    selected = section == entry,
                    onClick = { section = entry },
                    label = { Text(entry.label) },
                )
            }
        }
        Spacer(Modifier.height(8.dp))

        val status = state.status
        if (status == null) {
            Text(
                "Waiting for the subscription details…",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            return@Column
        }

        // Only the Guide is taller than the space it is given, and it carries its
        // own scroll. The two list tabs stay fixed-height so the page never moves.
        when (section) {
            ConnectSection.ADDRESSES -> AddressesTab(status, Modifier.weight(1f))
            ConnectSection.ENDPOINTS -> EndpointsCard(status, onCopy)
            ConnectSection.GUIDE -> GuideTab(status, onCopy)
        }
    }
}

@Composable
private fun AddressesTab(
    status: SubscriberStatusDto,
    modifier: Modifier,
) {
    val client = status.client
    val exactCount = client.allowedIps.count { !it.contains("/") }
    SubscriberCard(
        verticalPadding = 10.dp,
        title = "Registered Addresses",
        icon = Icons.Filled.Dns,
        modifier = modifier.fillMaxWidth(),
        fillHeight = true,
    ) {
        Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Text(
                text = status.detectedIp.ifBlank { "Unknown" },
                style = MaterialTheme.typography.labelSmall.copy(fontFamily = PanelMono),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.weight(1f),
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            if (client.maxDevices > 0) {
                Text(
                    "$exactCount of ${client.maxDevices} used",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        // The card is fixed to the tab; only this text scrolls. A long address list
        // therefore moves inside the box instead of turning the page into a scroll.
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .weight(1f)
                .verticalScroll(rememberScrollState()),
        ) {
            if (client.allowedIps.isEmpty()) {
                Text(
                    "None yet. Register this device or a specific address on the Status tab.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            } else {
                // One wrapped block of text, not chips: addresses and subnets are
                // listed exactly as the server stores them, comma-separated.
                Text(
                    text = client.allowedIps.joinToString(", "),
                    style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
                )
            }
        }
        Text(
            "Subnets are shown as CIDR. Scroll the list if it is long.",
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun EndpointsCard(status: SubscriberStatusDto, onCopy: (String) -> Unit) {
    val doh = "https://${status.serverHost}:${status.dohPort}/dns-query"
    val dot = "${status.serverHost}:${status.dotPort}"
    SubscriberCard(verticalPadding = 10.dp, title = "Endpoints", icon = Icons.Filled.Lan) {
        Text(
            "Tap a value to copy",
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        CopyRow("Primary DNS", status.serverDns, onCopy)
        CopyRow("Secondary DNS", "1.1.1.1", onCopy)
        if (status.hasDomain) CopyRow("DoH", doh, onCopy)
        CopyRow("DoT", dot, onCopy)
    }
}

@Composable
private fun CopyRow(label: String, value: String, onCopy: (String) -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable { onCopy(value) }
            .padding(vertical = 2.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(label, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Text(
                value,
                style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
                // The DoH URL is longer than a phone row; let it use a second line
                // instead of hiding /dns-query behind an ellipsis.
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
        }
        IconButton(onClick = { onCopy(value) }, modifier = Modifier.size(32.dp)) {
            Icon(
                Icons.Filled.ContentCopy,
                contentDescription = "Copy $label",
                modifier = Modifier.size(16.dp),
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}
