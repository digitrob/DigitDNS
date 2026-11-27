package com.digitrob.digitdns.ui.system

import androidx.activity.compose.BackHandler
import androidx.annotation.StringRes
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.BarChart
import androidx.compose.material.icons.filled.Block
import androidx.compose.material.icons.filled.Code
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.FileDownload
import androidx.compose.material.icons.filled.History
import androidx.compose.material.icons.filled.Info
import androidx.compose.material.icons.filled.Key
import androidx.compose.material.icons.filled.Layers
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.MonitorHeart
import androidx.compose.material.icons.filled.Notifications
import androidx.compose.material.icons.filled.Podcasts
import androidx.compose.material.icons.filled.Policy
import androidx.compose.material.icons.filled.Public
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.Share
import androidx.compose.material.icons.filled.Shield
import androidx.compose.material.icons.filled.Speed
import androidx.compose.material.icons.filled.Sync
import androidx.compose.material.icons.filled.Tune
import androidx.compose.material.icons.filled.VerifiedUser
import androidx.compose.material.icons.filled.Widgets
import androidx.compose.material.icons.filled.Wifi
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.digitrob.digitdns.R
import com.digitrob.digitdns.ui.connect.ConnectScreen
import com.digitrob.digitdns.ui.rules.AccessScreen
import com.digitrob.digitdns.ui.rules.AdBlockingScreen
import com.digitrob.digitdns.ui.rules.AdGuardUpstreamImportScreen
import com.digitrob.digitdns.ui.rules.GroupsScreen
import com.digitrob.digitdns.ui.rules.MigrateScreen
import com.digitrob.digitdns.ui.rules.PoliciesScreen
import com.digitrob.digitdns.ui.rules.UpstreamsScreen

/**
 * Settings groups. The panel's Settings tab is a column of cards, and its
 * Resolvers tab is the resolver half of the same idea; on a phone the hub is a
 * grouped list instead, because twenty-three chips in one horizontal scroller
 * buried the one section an operator came for and offered no search.
 *
 * The group order mirrors the web sidebar: resolver plumbing first, then the
 * server and its access controls, then DNS policy, monitoring, and the app
 * itself.
 */
internal enum class SettingsGroup(@StringRes val labelRes: Int) {
    Resolvers(R.string.settings_group_resolvers),
    Server(R.string.settings_group_server),
    Policy(R.string.settings_group_policy),
    Monitoring(R.string.settings_group_monitoring),
    App(R.string.settings_group_app),
}

/**
 * One row in the hub. [keywords] is the English search vocabulary for a
 * section; the app ships one language, so this stays a plain string rather than
 * another resource that would never be translated. The enum name is searched
 * too, which covers "Dnssec", "Ipv6" and the other internal spellings.
 */
internal enum class SettingsSection(
    @StringRes val labelRes: Int,
    val icon: ImageVector,
    val group: SettingsGroup,
    val keywords: String = "",
) {
    Upstreams(R.string.settings_upstreams, Icons.Filled.Podcasts, SettingsGroup.Resolvers, "upstream dns resolver"),
    Transports(R.string.settings_transports, Icons.Filled.Wifi, SettingsGroup.Resolvers, "transports doh dot doq dnscrypt mtls"),
    Dnssec(R.string.settings_dnssec, Icons.Filled.Shield, SettingsGroup.Resolvers, "dnssec proxy leak validation"),
    Ipv6(R.string.settings_ipv6, Icons.Filled.Public, SettingsGroup.Resolvers, "ipv6 aaaa"),
    Access(R.string.settings_access, Icons.Filled.Key, SettingsGroup.Resolvers, "access doh tokens allowlist cidr"),

    Servers(R.string.settings_servers, Icons.Filled.Dns, SettingsGroup.Server, "servers switch"),
    Network(R.string.settings_network, Icons.Filled.Settings, SettingsGroup.Server, "network server port"),
    Tls(R.string.settings_tls, Icons.Filled.Lock, SettingsGroup.Server, "tls ssl certificate acme"),
    Subscription(R.string.settings_subscription, Icons.Filled.Share, SettingsGroup.Server, "subscription portal"),
    DnsProfiles(
        R.string.settings_dns_profiles,
        Icons.Filled.FileDownload,
        SettingsGroup.Server,
        "dns profiles mobileconfig macos doh dot download",
    ),
    Security(R.string.settings_security, Icons.Filled.VerifiedUser, SettingsGroup.Server, "security 2fa password admin path credentials"),
    RelayAgents(R.string.settings_relay_agents, Icons.Filled.Speed, SettingsGroup.Server, "egress relay agents socks wireguard"),

    AdBlocking(R.string.settings_ad_blocking, Icons.Filled.Block, SettingsGroup.Policy, "ad blocking filters blocklist"),
    Import(R.string.settings_import, Icons.Filled.FileDownload, SettingsGroup.Policy, "import migrate pihole adguard"),
    AdGuardUpstreamImport(
        R.string.settings_adguard_upstream_import,
        Icons.Filled.Sync,
        SettingsGroup.Policy,
        "adguard upstream list import sync domains coverage",
    ),
    Groups(R.string.settings_groups, Icons.Filled.Layers, SettingsGroup.Policy, "groups policy bundles"),
    Policies(R.string.settings_policies, Icons.Filled.Policy, SettingsGroup.Policy, "policies rules"),

    History(R.string.settings_history, Icons.Filled.BarChart, SettingsGroup.Monitoring, "history traffic stats"),
    QueryLog(R.string.settings_query_log, Icons.Filled.History, SettingsGroup.Monitoring, "query log history"),
    Alerts(R.string.settings_alerts, Icons.Filled.Notifications, SettingsGroup.Monitoring, "alerts notifications"),
    SystemLog(R.string.settings_system_log, Icons.Filled.Code, SettingsGroup.Monitoring, "system log diagnostics"),
    Diagnostics(R.string.settings_diagnostics, Icons.Filled.MonitorHeart, SettingsGroup.Monitoring, "diagnostics health"),

    App(R.string.settings_app, Icons.Filled.Tune, SettingsGroup.App, "app theme biometrics layout"),
    Widgets(
        R.string.settings_widgets,
        Icons.Filled.Widgets,
        SettingsGroup.App,
        "widget widgets home screen resolver theme opacity transparency text size",
    ),
    RestApi(R.string.settings_rest_api, Icons.Filled.Code, SettingsGroup.App, "rest api key"),
    ConnectGuide(R.string.settings_connect_guide, Icons.Filled.Wifi, SettingsGroup.App, "connect guide setup"),
    About(R.string.settings_about, Icons.Filled.Info, SettingsGroup.App, "about version licence"),
}

/**
 * Search filter for the hub. Pure so it can be unit-tested without Compose:
 * an empty query returns every section in declaration order, and a query
 * matches the enum name or its English keywords.
 */
internal fun filterSettingsSections(query: String): List<SettingsSection> {
    val needle = query.trim().lowercase()
    if (needle.isEmpty()) return SettingsSection.entries
    return SettingsSection.entries.filter { section ->
        section.name.lowercase().contains(needle) || section.keywords.contains(needle)
    }
}

/**
 * The Settings tab. [onLoggedOut] is invoked once the operator has signed out, so
 * the host can return to the login screen — the tab itself owns no navigation.
 *
 * The hub is a list, not a chip row. Selecting a row opens the existing section
 * screen in place with a back arrow; the system back gesture returns to the
 * list. The selection is `rememberSaveable`, so leaving the tab and coming back
 * resumes the section, matching the per-tab state the bottom bar restores.
 */
@Composable
fun SettingsHubScreen(
    onLoggedOut: () -> Unit,
    onAddServer: () -> Unit,
    onOpenSubscriber: () -> Unit = {},
) {
    var query by rememberSaveable { mutableStateOf("") }
    var section by rememberSaveable { mutableStateOf(SettingsSection.App) }
    var detailOpen by rememberSaveable { mutableStateOf(false) }

    // Only a detail swallows back. On the hub list the gesture keeps its normal
    // meaning (leave the tab), so this handler must be disabled there.
    BackHandler(enabled = detailOpen) { detailOpen = false }

    if (detailOpen) {
        Column(modifier = Modifier.fillMaxSize()) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 4.dp, vertical = 2.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                IconButton(onClick = { detailOpen = false }) {
                    Icon(
                        imageVector = Icons.AutoMirrored.Filled.ArrowBack,
                        contentDescription = stringResource(R.string.settings_back),
                    )
                }
                Text(
                    text = stringResource(section.labelRes),
                    style = MaterialTheme.typography.titleMedium,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            SettingsSectionContent(section, onLoggedOut, onAddServer, onOpenSubscriber)
        }
        return
    }

    val matches = filterSettingsSections(query)

    Column(modifier = Modifier.fillMaxSize()) {
        OutlinedTextField(
            value = query,
            onValueChange = { query = it },
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 12.dp, vertical = 8.dp),
            singleLine = true,
            leadingIcon = { Icon(Icons.Filled.Search, contentDescription = null) },
            placeholder = { Text(stringResource(R.string.settings_search_hint)) },
        )

        if (matches.isEmpty()) {
            Text(
                text = stringResource(R.string.settings_search_empty),
                modifier = Modifier.padding(16.dp),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            return@Column
        }

        LazyColumn(modifier = Modifier.fillMaxSize()) {
            SettingsGroup.entries.forEach { group ->
                val grouped = matches.filter { it.group == group }
                if (grouped.isEmpty()) return@forEach
                item(key = "group-${group.name}") {
                    Text(
                        text = stringResource(group.labelRes),
                        modifier = Modifier.padding(start = 16.dp, end = 16.dp, top = 14.dp, bottom = 4.dp),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.primary,
                    )
                }
                items(grouped, key = { it.name }) { candidate ->
                    SettingsRow(candidate) {
                        section = candidate
                        detailOpen = true
                    }
                }
            }
        }
    }
}

@Composable
private fun SettingsRow(section: SettingsSection, onClick: () -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Icon(
            imageVector = section.icon,
            contentDescription = null,
            modifier = Modifier.size(20.dp),
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Text(
            text = stringResource(section.labelRes),
            style = MaterialTheme.typography.bodyLarge,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
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
    onOpenSubscriber: () -> Unit,
) {
    when (section) {
        SettingsSection.App -> AppSettingsScreen()
        SettingsSection.Widgets -> WidgetSettingsScreen()
        SettingsSection.Dnssec -> DnssecProxySettingsScreen()
        SettingsSection.Transports -> ClientTransportsScreen()
        SettingsSection.Ipv6 -> Ipv6SettingsScreen()
        SettingsSection.History -> HistorySettingsScreen()
        SettingsSection.AdBlocking -> AdBlockingScreen()
        SettingsSection.Import -> MigrateScreen()
        SettingsSection.AdGuardUpstreamImport -> AdGuardUpstreamImportScreen()
        SettingsSection.Servers -> ServersScreen(
            onLoggedOut = onLoggedOut,
            onAddServer = onAddServer,
            onOpenSubscriber = onOpenSubscriber,
        )
        SettingsSection.RelayAgents -> RelayAgentsScreen()
        SettingsSection.SystemLog -> SystemLogScreen()
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
        SettingsSection.DnsProfiles -> DnsProfilesScreen()
        SettingsSection.Alerts -> AlertsScreen()
        SettingsSection.QueryLog -> QueryLogScreen()
        SettingsSection.About -> AboutScreen()
    }
}
