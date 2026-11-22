package com.digitrob.digitdns.ui.system

import android.content.Intent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.core.content.FileProvider
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono
import java.io.File
import java.util.UUID

/**
 * The SettingsHub detail screen for exporting a macOS .mobileconfig profile.
 *
 * It reuses ClientTransportsViewModel so the endpoint shown here is the one the
 * server is actually configured with (DoH/DoT host and ports), rather than a
 * hard-coded address. The generated profile is written to the app cache and
 * shared through a FileProvider so it arrives as a real attachment.
 */
@Composable
fun DnsProfilesScreen(viewModel: ClientTransportsViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val context = LocalContext.current
    var transport by remember { mutableStateOf("doh") }
    var message by remember { mutableStateOf("") }

    val host = state.dotHost.ifBlank { state.serverHost }
    val port = if (transport == "doh") {
        state.dohPort.ifBlank { "8443" }
    } else {
        state.dotPort.ifBlank { "853" }
    }
    val endpoint = if (host.isBlank()) {
        "Configure a domain and certificate first"
    } else if (transport == "doh") {
        val authority = if (port == "443") host else "$host:$port"
        "https://$authority/dns-query"
    } else {
        "tls://$host:$port"
    }

    PanelRefreshBox(
        isRefreshing = state.isRefreshing,
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
            state.error?.let {
                Text(it, color = DigitDnsTheme.colors.danger, style = MaterialTheme.typography.bodySmall)
            }

            PanelCard(modifier = Modifier.fillMaxWidth()) {
                Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    Text(
                        text = "Generate a macOS configuration profile for a client device.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        FilterChip(
                            selected = transport == "doh",
                            onClick = { transport = "doh"; message = "" },
                            label = { Text("DoH (DNS over HTTPS)") },
                        )
                        FilterChip(
                            selected = transport == "dot",
                            onClick = { transport = "dot"; message = "" },
                            label = { Text("DoT (DNS over TLS)") },
                        )
                    }
                }
            }

            PanelCard(modifier = Modifier.fillMaxWidth()) {
                Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Text(
                        text = "Server endpoint",
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Text(
                        text = endpoint,
                        style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
                    )
                }
            }

            PanelCard(modifier = Modifier.fillMaxWidth()) {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Button(
                        onClick = {
                            if (host.isBlank()) {
                                message = "Configure a domain and certificate first"
                                return@Button
                            }
                            val fileName = "digitdns-profile-$transport.mobileconfig"
                            val xml = buildMobileconfig(transport, host, port, endpoint)
                            val file = File(context.cacheDir, fileName)
                            file.writeText(xml)
                            val uri = FileProvider.getUriForFile(
                                context,
                                "${context.packageName}.fileprovider",
                                file,
                            )
                            val sendIntent = Intent(Intent.ACTION_SEND).apply {
                                type = "application/x-plist"
                                putExtra(Intent.EXTRA_STREAM, uri)
                                putExtra(Intent.EXTRA_SUBJECT, fileName)
                                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                            }
                            runCatching {
                                context.startActivity(Intent.createChooser(sendIntent, "Share $fileName"))
                                message = "Saved $fileName"
                            }.onFailure { e ->
                                message = e.message ?: "Could not share the profile"
                            }
                        },
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text("Generate .mobileconfig")
                    }
                    if (message.isNotBlank()) {
                        Text(message, style = MaterialTheme.typography.bodySmall)
                    }
                    Text(
                        text = "Save the shared file, open it on macOS, and approve it in " +
                            "System Settings → VPN & Device Management. Remove it there when it is no longer needed.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
    }
}

/** Apple com.apple.dns.managed payload; DNSProtocol selects HTTPS (DoH) or TLS (DoT). */
private fun buildMobileconfig(transport: String, host: String, port: String, endpoint: String): String {
    val protocol = if (transport == "doh") "HTTPS" else "TLS"
    val label = if (transport == "doh") "DoH" else "DoT"
    val idPrefix = "com.digitdns.profile.$transport"
    val serverKeys = if (transport == "doh") {
        "            <key>ServerURL</key>\n" +
            "            <string>$endpoint</string>\n" +
            "            <key>ServerNames</key>\n" +
            "            <array>\n" +
            "                <string>$host</string>\n" +
            "            </array>"
    } else {
        "            <key>ServerAddresses</key>\n" +
            "            <array>\n" +
            "                <string>$host</string>\n" +
            "            </array>\n" +
            "            <key>ServerNames</key>\n" +
            "            <array>\n" +
            "                <string>$host</string>\n" +
            "            </array>"
    }
    return """<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>PayloadContent</key>
    <array>
        <dict>
            <key>DNSSettings</key>
            <dict>
                <key>DNSProtocol</key>
                <string>$protocol</string>
$serverKeys
            </dict>
            <key>PayloadDisplayName</key>
            <string>DigitDNS $label</string>
            <key>PayloadIdentifier</key>
            <string>$idPrefix.dns</string>
            <key>PayloadType</key>
            <string>com.apple.dns.managed</string>
            <key>PayloadUUID</key>
            <string>${UUID.randomUUID()}</string>
            <key>PayloadVersion</key>
            <integer>1</integer>
        </dict>
    </array>
    <key>PayloadDescription</key>
    <string>Configure DigitDNS encrypted DNS ($label)</string>
    <key>PayloadDisplayName</key>
    <string>DigitDNS DNS $label</string>
    <key>PayloadIdentifier</key>
    <string>$idPrefix</string>
    <key>PayloadOrganization</key>
    <string>DigitDNS</string>
    <key>PayloadRemovalDisallowed</key>
    <false/>
    <key>PayloadType</key>
    <string>Configuration</string>
    <key>PayloadUUID</key>
    <string>${UUID.randomUUID()}</string>
    <key>PayloadVersion</key>
    <integer>1</integer>
</dict>
</plist>"""
}
