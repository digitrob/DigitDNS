package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Code
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.Info
import androidx.compose.material.icons.filled.Key
import androidx.compose.material.icons.filled.Layers
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.MonitorHeart
import androidx.compose.material.icons.filled.Notifications
import androidx.compose.material.icons.filled.Podcasts
import androidx.compose.material.icons.filled.Policy
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.Share
import androidx.compose.material.icons.filled.Tune
import androidx.compose.material.icons.filled.VerifiedUser
import androidx.compose.material.icons.filled.Wifi
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.dp
import com.digitrob.digitdns.ui.connect.ConnectScreen
import com.digitrob.digitdns.ui.rules.AccessScreen
import com.digitrob.digitdns.ui.rules.GroupsScreen
import com.digitrob.digitdns.ui.rules.PoliciesScreen
import com.digitrob.digitdns.ui.rules.UpstreamsScreen

/**
 * The Settings & SSL tab — the panel's top-level `Settings & SSL` section, plus the
 * panel's `REST API Gateway` and `Connect Guide`, which do not get their own phone tab.
 *
 * The panel presents these as a column of cards; on a phone they become a scrollable
 * chip row with one view at a time, because stacking twenty cards on a phone screen
 * buries the one you came for. The selection is `rememberSaveable`, so returning to the
 * tab lands where it was left, matching the per-tab state the bottom bar restores.
 *
 * Each section carries the panel's own Feather icon, matched to the nearest Material
 * glyph, so the row reads the same way it does on the web.
 */
private enum class SettingsSection(val label: String, val icon: ImageVector) {
    App("App", Icons.Filled.Tune),
    Servers("Servers", Icons.Filled.Dns),
    ConnectGuide("Connect Guide", Icons.Filled.Wifi),
    Diagnostics("Diagnostics", Icons.Filled.MonitorHeart),
    Tls("SSL / TLS", Icons.Filled.Lock),
    Network("Network & Server", Icons.Filled.Settings),
    RestApi("REST API", Icons.Filled.Code),
    Security("Security", Icons.Filled.VerifiedUser),
    Groups("Groups", Icons.Filled.Layers),
    Policies("Policies", Icons.Filled.Policy),
    Access("Access", Icons.Filled.Key),
    Upstreams("Upstreams", Icons.Filled.Podcasts),
    Subscription("Subscription", Icons.Filled.Share),
    Alerts("Alerts", Icons.Filled.Notifications),
    About("About", Icons.Filled.Info),
}

/**
 * The Settings & SSL tab. [onLoggedOut] is invoked once the operator has signed out, so
 * the host can return to the login screen — the tab itself owns no navigation.
 */
@Composable
fun SettingsHubScreen(onLoggedOut: () -> Unit, onAddServer: () -> Unit) {
    var section by rememberSaveable { mutableStateOf(SettingsSection.App) }

    Column(modifier = Modifier.fillMaxSize()) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .horizontalScroll(rememberScrollState())
                .padding(horizontal = 12.dp, vertical = 8.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            SettingsSection.entries.forEach { candidate ->
                FilterChip(
                    selected = section == candidate,
                    onClick = { section = candidate },
                    label = {
                        Row(verticalAlignment = androidx.compose.ui.Alignment.CenterVertically) {
                            Icon(
                                imageVector = candidate.icon,
                                contentDescription = null,
                                modifier = Modifier.size(16.dp),
                            )
                            Spacer(Modifier.width(6.dp))
                            Text(candidate.label)
                        }
                    },
                )
            }
        }

        SettingsSectionContent(section, onLoggedOut, onAddServer)
    }
}

// A pure dispatch table: one branch per section, no nesting or conditions to untangle.
// The complexity metric counts the branches, but there is nothing here to simplify —
// splitting it would only scatter the section-to-screen mapping across the file.
@Suppress("CyclomaticComplexMethod")
@Composable
private fun SettingsSectionContent(
    section: SettingsSection,
    onLoggedOut: () -> Unit,
    onAddServer: () -> Unit,
) {
    when (section) {
        SettingsSection.App -> AppSettingsScreen()
        SettingsSection.Servers -> ServersScreen(onLoggedOut = onLoggedOut, onAddServer = onAddServer)
        SettingsSection.ConnectGuide -> ConnectScreen()
        SettingsSection.Diagnostics -> DiagnosticsScreen()
        SettingsSection.Tls -> TlsScreen()
        SettingsSection.Network -> SettingsScreen()
        SettingsSection.RestApi -> RestApiScreen()
        SettingsSection.Security -> SecurityScreen()
        SettingsSection.Groups -> GroupsScreen()
        SettingsSection.Policies -> PoliciesScreen()
        SettingsSection.Access -> AccessScreen()
        SettingsSection.Upstreams -> UpstreamsScreen()
        SettingsSection.Subscription -> SubscriptionScreen()
        SettingsSection.Alerts -> AlertsScreen()
        SettingsSection.About -> AboutScreen()
    }
}
