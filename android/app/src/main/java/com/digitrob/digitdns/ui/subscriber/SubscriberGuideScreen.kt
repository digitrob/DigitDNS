package com.digitrob.digitdns.ui.subscriber

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.MenuBook
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.digitrob.digitdns.core.network.SubscriberStatusDto
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono

/**
 * One platform's setup guide. [privateDns] marks the Android page, whose first
 * option is the encrypted Private DNS hostname rather than a plain resolver IP.
 */
private data class GuidePlatform(
    val chip: String,
    val title: String,
    val steps: List<String>,
    val note: String? = null,
    val privateDns: Boolean = false,
)

private val playStationGuide = GuidePlatform(
    chip = "PlayStation",
    title = "Setting the DNS on a PlayStation",
    steps = listOf(
        "Open Settings on the console and go to Network.",
        "Choose Set Up Internet Connection and pick how you connect (Wi-Fi or LAN).",
        "Choose Custom so the manual settings appear.",
        "Leave IP Address on Automatic and DHCP Host Name on Do Not Specify.",
        "Under DNS Settings choose Manual and enter the two values below.",
        "Leave MTU on Automatic and Proxy Server on Do Not Use, then run Test Internet Connection.",
    ),
)

private val xboxGuide = GuidePlatform(
    chip = "Xbox",
    title = "Setting the DNS on an Xbox",
    steps = listOf(
        "Press the Xbox button and open Settings.",
        "Go to General ➔ Network settings ➔ Advanced settings.",
        "Open DNS settings and switch it to Manual.",
        "Enter the two values below as the primary and secondary IPv4 DNS, then save.",
    ),
)

private val switchGuide = GuidePlatform(
    chip = "Switch",
    title = "Setting the DNS on a Nintendo Switch",
    steps = listOf(
        "Go to System Settings ➔ Internet ➔ Internet Settings.",
        "Select the Wi-Fi network you are on and choose Change Settings.",
        "Set DNS Settings to Manual and enter the two values below.",
    ),
)

private val windowsGuide = GuidePlatform(
    chip = "Windows",
    title = "Setting the DNS on Windows 10 / 11",
    steps = listOf(
        "Open Settings ➔ Network & Internet.",
        "Click the connection you are using (Wi-Fi or Ethernet) and press Edit next to DNS server assignment.",
        "Change Automatic to Manual and turn the IPv4 switch on.",
        "Enter the two values below as Preferred DNS and Alternate DNS, then press Save.",
    ),
)

private val appleGuide = GuidePlatform(
    chip = "iOS / macOS",
    title = "Setting the DNS on iOS and macOS",
    steps = listOf(
        "iPhone and iPad: Settings ➔ Wi-Fi ➔ the (i) icon next to your network ➔ " +
            "Configure DNS ➔ Manual, delete the listed addresses, then use Add Server " +
            "to add the address below.",
        "Mac: System Settings ➔ Network ➔ Details ➔ DNS, add the address below with " +
            "the (+) button, and press OK.",
    ),
)

private val routerGuide = GuidePlatform(
    chip = "Router",
    title = "Setting the DNS on a modem or router",
    steps = listOf(
        "Open your modem's admin page in a browser (usually 192.168.1.1 or 192.168.0.1).",
        "Find the WAN, Internet or DHCP section and change DNS from Automatic to Manual/Static.",
        "Enter the two values below, save, and restart the modem once. Every device in " +
            "the house then uses this DNS with no setup of its own.",
    ),
)

/** Android differs by install: Private DNS needs a hostname, static Wi-Fi does not. */
private fun androidGuide(status: SubscriberStatusDto) = GuidePlatform(
    chip = "Android",
    title = "Setting the DNS on Android",
    steps = if (status.hasDomain) {
        listOf(
            "Option 1 — Private DNS: open Settings ➔ Network & internet ➔ Private DNS, " +
                "choose “Private DNS provider hostname” and enter the hostname shown below.",
            "Option 2 — static Wi-Fi: open Settings ➔ Wi-Fi ➔ long-press your network ➔ " +
                "Modify network ➔ Advanced, set IP settings to Static, and enter the two DNS " +
                "values shown below as DNS 1 and DNS 2.",
        )
    } else {
        listOf(
            "This server has no domain and TLS certificate yet, so Android's Private DNS " +
                "cannot be used — that field will not accept an IP address.",
            "Static Wi-Fi: open Settings ➔ Wi-Fi ➔ long-press your network ➔ Modify network ➔ " +
                "Advanced, set IP settings to Static, and enter the two DNS values shown below " +
                "as DNS 1 and DNS 2. Or ask your provider to add a domain.",
        )
    },
    note = if (status.hasDomain) null else
        "Private DNS needs a hostname. Until this server has a domain, use the static " +
            "Wi-Fi option below.",
    privateDns = true,
)

private fun platforms(status: SubscriberStatusDto): List<GuidePlatform> = listOf(
    playStationGuide,
    xboxGuide,
    switchGuide,
    windowsGuide,
    appleGuide,
    androidGuide(status),
    routerGuide,
)

/**
 * The full setup guide, native. The portal used to be the only place with the
 * per-platform steps, which meant leaving the app for a browser page; every panel
 * and every step now lives here, with the live values from this subscription.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
internal fun GuideTab(status: SubscriberStatusDto, onCopy: (String) -> Unit) {
    val entries = platforms(status)
    var selected by rememberSaveable { mutableIntStateOf(0) }
    val platform = entries[selected.coerceIn(entries.indices)]

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(bottom = 12.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        FlowRow(
            horizontalArrangement = Arrangement.spacedBy(6.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            entries.forEachIndexed { index, entry ->
                FilterChip(
                    selected = index == selected,
                    onClick = { selected = index },
                    label = {
                        Text(entry.chip, style = MaterialTheme.typography.labelSmall)
                    },
                )
            }
        }

        SubscriberCard(verticalPadding = 10.dp, title = "Guide", icon = Icons.Filled.MenuBook) {
            Text(
                platform.title,
                style = MaterialTheme.typography.titleSmall.copy(fontWeight = FontWeight.SemiBold),
            )
            platform.steps.forEachIndexed { index, step ->
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    Text(
                        "${index + 1}.",
                        style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Text(
                        step,
                        style = MaterialTheme.typography.bodySmall,
                        modifier = Modifier.weight(1f),
                    )
                }
            }
            platform.note?.let { note ->
                Text(
                    note,
                    style = MaterialTheme.typography.bodySmall,
                    color = DigitDnsTheme.colors.warning,
                )
            }
        }

        SubscriberCard(verticalPadding = 10.dp, title = "Values", icon = Icons.Filled.Dns) {
            GuideValueRow("Primary DNS", status.serverDns, onCopy)
            GuideValueRow("Secondary DNS", "1.1.1.1", onCopy)
            if (platform.privateDns && status.hasDomain) {
                GuideValueRow("Private DNS hostname", status.serverHost, onCopy)
            }
            if (status.hasDomain) {
                GuideValueRow("DoH", "https://${status.serverHost}:${status.dohPort}/dns-query", onCopy)
            }
            GuideValueRow("DoT", "${status.serverHost}:${status.dotPort}", onCopy)
        }
    }
}

@Composable
private fun GuideValueRow(label: String, value: String, onCopy: (String) -> Unit) {
    Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Column(modifier = Modifier.weight(1f)) {
            Text(
                label,
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                value,
                style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
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
