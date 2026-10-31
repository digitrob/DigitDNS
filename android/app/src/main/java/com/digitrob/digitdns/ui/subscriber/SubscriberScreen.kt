package com.digitrob.digitdns.ui.subscriber

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.fillMaxHeight
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
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Dashboard
import androidx.compose.material.icons.filled.DataUsage
import androidx.compose.material.icons.filled.Key
import androidx.compose.material.icons.filled.Person
import androidx.compose.material.icons.filled.Public
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.Shield
import androidx.compose.material.icons.filled.WifiTethering
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import kotlinx.coroutines.launch
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import android.content.ClipData
import android.content.ClipboardManager
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.core.network.SubscriberStatusDto
import com.digitrob.digitdns.ui.auth.TrustPrompt
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelDialog
import com.digitrob.digitdns.ui.components.PanelSectionTitle
import com.digitrob.digitdns.ui.components.PanelSnackbarHost
import com.digitrob.digitdns.ui.components.showPanelToast
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono

/** The subscriber bottom-bar destinations. */
private enum class SubscriberTab(val label: String) {
    STATUS("Status"),
    CONNECT("Connect"),
    SETTINGS("Settings"),
}

/** The register card's editable fields, grouped so the composable keeps a short signature. */
internal data class RegisterForm(
    val secret: String,
    val saveSecret: Boolean,
    val customIp: String,
)

/**
 * The customer portal as a native surface: Status, Connect, Settings.
 *
 * It mirrors the `/sub/<token>` page with the app's own cards and typography. Content
 * is kept compact so each tab fits a phone without scrolling where the text scale
 * allows it; a scroll is still there as the fallback for large fonts.
 */
@Composable
fun SubscriberScreen(
    onSignedOut: () -> Unit,
    onForgotten: () -> Unit,
    onSwitchToAdmin: () -> Unit,
    viewModel: SubscriberViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()
    val context = LocalContext.current
    var tab by rememberSaveable { mutableStateOf(SubscriberTab.STATUS) }
    val onCopy: (String) -> Unit = { value ->
        val clipboard = context.getSystemService(ClipboardManager::class.java)
        clipboard?.setPrimaryClip(ClipData.newPlainText("DigitDNS", value))
        scope.launch { snackbar.showPanelToast("Copied") }
    }
    // The registration form lives here, not inside the Status tab, so switching to
    // Connect and back does not throw away a half-typed secret. It is shown on
    // Status but survives a tab change.
    var registerSecret by rememberSaveable { mutableStateOf("") }
    var registerAddress by rememberSaveable { mutableStateOf("") }
    var saveRegisterSecret by rememberSaveable { mutableStateOf(state.secretSaved) }
    LaunchedEffect(state.secretSaved) { saveRegisterSecret = state.secretSaved }

    LaunchedEffect(state.message) {
        val message = state.message ?: return@LaunchedEffect
        snackbar.showPanelToast(message)
        viewModel.consumeMessage()
    }
    LaunchedEffect(state.exit) {
        when (state.exit) {
            SubscriberExit.SIGNED_OUT -> onSignedOut()
            SubscriberExit.FORGOTTEN -> onForgotten()
            SubscriberExit.SWITCHED_TO_ADMIN -> onSwitchToAdmin()
            null -> Unit
        }
    }

    Scaffold(
        snackbarHost = { PanelSnackbarHost(snackbar) },
        bottomBar = { SubscriberBottomBar(tab = tab, onSelect = { tab = it }) },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(horizontal = 12.dp),
        ) {
            SubscriberHeader(state = state, onRefresh = { viewModel.refresh() })
            // The tab content gets the remaining height: without the weight it could
            // measure against the whole column and draw under the navigation bar.
            Box(modifier = Modifier.weight(1f).fillMaxWidth()) {
                when (tab) {
                    SubscriberTab.STATUS -> StatusTab(
                        state = state,
                        onCopy = onCopy,
                        register = RegisterForm(registerSecret, saveRegisterSecret, registerAddress),
                        onRegisterChange = { form ->
                            registerSecret = form.secret
                            saveRegisterSecret = form.saveSecret
                            registerAddress = form.customIp
                        },
                        onRegisterDevice = { viewModel.register(registerSecret, null, saveRegisterSecret) },
                        onRegisterCustom = {
                            viewModel.register(registerSecret, registerAddress, saveRegisterSecret)
                        },
                    )
                    SubscriberTab.CONNECT -> ConnectTab(
                        state = state,
                        onCopy = onCopy,
                    )
                    SubscriberTab.SETTINGS -> SettingsTab(
                        state = state,
                        viewModel = viewModel,
                        onCopy = onCopy,
                        onSwitchToAdmin = viewModel::switchToAdmin,
                    )
                }
            }
        }
    }

    state.trustPrompt?.let { prompt ->
        SubscriberTrustDialog(
            prompt = prompt,
            onTrust = viewModel::trustCurrentHost,
            onDismiss = viewModel::dismissTrust,
        )
    }
}

@Composable
private fun SubscriberBottomBar(tab: SubscriberTab, onSelect: (SubscriberTab) -> Unit) {
    NavigationBar {
        SubscriberTab.entries.forEach { entry ->
            NavigationBarItem(
                selected = tab == entry,
                onClick = { onSelect(entry) },
                icon = {
                    Icon(
                        when (entry) {
                            SubscriberTab.STATUS -> Icons.Filled.Dashboard
                            SubscriberTab.CONNECT -> Icons.Filled.WifiTethering
                            SubscriberTab.SETTINGS -> Icons.Filled.Settings
                        },
                        contentDescription = entry.label,
                    )
                },
                label = { Text(entry.label) },
            )
        }
    }
}

/**
 * A subscriber card. [PanelCard] is a Box, so a card with several lines must put them
 * in a Column or they draw on top of each other. This wrapper is that Column, with the
 * app's compact 4 dp rhythm.
 */
@Composable
internal fun SubscriberCard(
    verticalPadding: androidx.compose.ui.unit.Dp = 10.dp,
    title: String? = null,
    icon: ImageVector? = null,
    modifier: Modifier = Modifier,
    fillHeight: Boolean = false,
    content: @Composable androidx.compose.foundation.layout.ColumnScope.() -> Unit,
) {
    PanelCard(verticalPadding = verticalPadding, modifier = modifier) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .then(if (fillHeight) Modifier.fillMaxHeight() else Modifier),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            // The same uppercase heading and 14 dp glyph the admin cards carry.
            title?.let { PanelSectionTitle(text = it, icon = icon) }
            content()
        }
    }
}

@Composable
private fun SubscriberHeader(state: SubscriberUiState, onRefresh: () -> Unit) {
    // The top-right pill is connectivity, not the account. Account states
    // (Active, Suspended, Expired, Quota spent) belong in the Account card; this
    // corner answers only "can the app reach the server", like the server screen.
    val online = state.error == null
    val color = if (online) DigitDnsTheme.colors.success else DigitDnsTheme.colors.textMuted
    val label = if (online) "Online" else "Offline"
    Row(
        modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(Icons.Filled.Shield, contentDescription = null, tint = MaterialTheme.colorScheme.primary)
        Spacer(Modifier.width(8.dp))
        Text(
            text = "DigitDNS",
            style = MaterialTheme.typography.titleMedium.copy(fontWeight = FontWeight.Bold),
            color = MaterialTheme.colorScheme.primary,
        )
        Spacer(Modifier.weight(1f))
        // Refresh first, status last: the badge belongs in the right corner, the
        // same place the server screen puts its Online pill.
        IconButton(onClick = onRefresh, enabled = !state.refreshing && !state.loading) {
            if (state.refreshing || state.loading) {
                CircularProgressIndicator(modifier = Modifier.size(18.dp), strokeWidth = 2.dp)
            } else {
                Icon(Icons.Filled.Refresh, contentDescription = "Refresh")
            }
        }
        Spacer(Modifier.width(4.dp))
        StatusBadge(text = label, color = color)
    }
}

@Composable
private fun StatusTab(
    state: SubscriberUiState,
    onCopy: (String) -> Unit,
    register: RegisterForm,
    onRegisterChange: (RegisterForm) -> Unit,
    onRegisterDevice: () -> Unit,
    onRegisterCustom: () -> Unit,
) {
    val status = state.status
    if (status == null && state.loading) {
        Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            CircularProgressIndicator()
        }
        return
    }
    // No vertical scroll on this tab. The cards are compact and the registration
    // form is pinned to the bottom by the weight above it; the address list lives
    // on Connect, where it can wrap without pushing this page taller.
    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(bottom = 8.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        if (state.error != null) {
            SubscriberCard(verticalPadding = 8.dp) {
                Text(state.error, color = DigitDnsTheme.colors.danger, style = MaterialTheme.typography.bodySmall)
            }
        }
        if (status == null) return@Column

        IdentityCard(status, onCopy)
        DeviceIpCard(status, onCopy)
        UsageCard(status, state.now)
        RegisterCard(
            state = state,
            form = register,
            onChange = onRegisterChange,
            onRegisterDevice = onRegisterDevice,
            onRegisterCustom = onRegisterCustom,
            modifier = Modifier.weight(1f),
        )
    }
}

@Composable
private fun IdentityCard(status: SubscriberStatusDto, onCopy: (String) -> Unit) {
    val client = status.client
    SubscriberCard(verticalPadding = 8.dp, title = "Account", icon = Icons.Filled.Person) {
        Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Text(
                text = client.name.ifBlank { "Subscriber" },
                style = MaterialTheme.typography.titleSmall.copy(fontWeight = FontWeight.SemiBold),
                modifier = Modifier.weight(1f),
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            StatusBadge(
                text = SubscriberFormat.statusLabel(status.status),
                color = statusColor(status.status),
            )
            // The Device IP row ends with a 28 dp copy button; the same slot here
            // keeps the two status pills on one vertical line.
            Spacer(Modifier.size(28.dp))
        }
        CopyValueRow("ID", client.id, onCopy)
        CopyValueRow("UUID", client.uuid, onCopy)
    }
}

/** The address this device reaches the internet from, with a copy action. */
@Composable
private fun DeviceIpCard(status: SubscriberStatusDto, onCopy: (String) -> Unit) {
    SubscriberCard(verticalPadding = 8.dp, title = "Device IP", icon = Icons.Filled.Public) {
        Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Text(
                text = status.detectedIp.ifBlank { "Unknown" },
                style = MaterialTheme.typography.titleLarge.copy(
                    fontFamily = PanelMono,
                    fontWeight = FontWeight.Bold,
                ),
                color = DigitDnsTheme.colors.success,
                modifier = Modifier.weight(1f),
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            StatusBadge(
                text = if (status.detectedIpRegistered) "Registered" else "Not registered",
                color = if (status.detectedIpRegistered) DigitDnsTheme.colors.success else DigitDnsTheme.colors.warning,
            )
            IconButton(
                onClick = { if (status.detectedIp.isNotBlank()) onCopy(status.detectedIp) },
                enabled = status.detectedIp.isNotBlank(),
                modifier = Modifier.size(28.dp),
            ) {
                Icon(
                    Icons.Filled.ContentCopy,
                    contentDescription = "Copy device IP",
                    modifier = Modifier.size(14.dp),
                    tint = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

/** One label/value line with a copy pictogram; the whole line stays one row tall. */
@Composable
private fun CopyValueRow(label: String, value: String, onCopy: (String) -> Unit) {
    Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Text(
            text = label,
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.width(42.dp),
        )
        Text(
            text = value,
            style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
            modifier = Modifier.weight(1f),
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        IconButton(
            onClick = { onCopy(value) },
            enabled = value.isNotBlank(),
            modifier = Modifier.size(28.dp),
        ) {
            Icon(
                Icons.Filled.ContentCopy,
                contentDescription = "Copy $label",
                modifier = Modifier.size(14.dp),
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun UsageCard(status: SubscriberStatusDto, now: Long) {
    val client = status.client
    val fraction = SubscriberFormat.trafficFraction(client.trafficUsedBytes, client.trafficLimitGb)
    val barColor = when {
        status.quotaExceeded || fraction >= 0.95f -> DigitDnsTheme.colors.danger
        fraction >= 0.8f -> DigitDnsTheme.colors.warning
        else -> MaterialTheme.colorScheme.primary
    }
    SubscriberCard(verticalPadding = 8.dp, title = "Traffic", icon = Icons.Filled.DataUsage) {
        Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Text(
                text = SubscriberFormat.traffic(client.trafficUsedBytes, client.trafficLimitGb),
                style = MaterialTheme.typography.titleSmall.copy(fontFamily = PanelMono),
                modifier = Modifier.weight(1f),
            )
            Text(
                text = SubscriberFormat.percentLabel(client.trafficUsedBytes, client.trafficLimitGb),
                style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono, fontWeight = FontWeight.Bold),
            )
        }
        // A plain track + fill rather than LinearProgressIndicator: the Material
        // component adds a stop dot at the end, which reads as "some usage" at 0%.
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .height(6.dp)
                .clip(RoundedCornerShape(999.dp))
                .background(DigitDnsTheme.colors.borderColor),
        ) {
            Box(
                modifier = Modifier
                    .fillMaxWidth(fraction)
                    .fillMaxHeight()
                    .background(barColor),
            )
        }
        UsageFooter(status, now)
    }
}

/** The expiry, reset and policy line under the traffic bar. */
@Composable
private fun UsageFooter(status: SubscriberStatusDto, now: Long) {
    val countdown = SubscriberFormat.countdown(status.client.expiresAt, now)
    val left = if (countdown.lifetime) {
        "Lifetime"
    } else {
        "${countdown.days}d ${countdown.hours}h ${countdown.minutes}m left"
    }
    val policies = status.client.customPolicies
    Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Text(
            text = SubscriberFormat.date(status.client.expiresAt),
            style = MaterialTheme.typography.labelSmall.copy(fontFamily = PanelMono),
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.weight(1f))
        SubscriberFormat.reset(status.nextTrafficReset)?.let { reset ->
            Text(
                text = "Resets $reset",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
    Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Text(
            text = left,
            style = MaterialTheme.typography.labelSmall.copy(fontFamily = PanelMono),
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.weight(1f),
        )
        Text(
            text = "Policies · ${if (policies.isEmpty()) "All" else policies.joinToString(", ")}",
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f),
        )
    }
}

@Composable
private fun SubscriberTrustDialog(
    prompt: TrustPrompt,
    onTrust: () -> Unit,
    onDismiss: () -> Unit,
) {
    PanelDialog(
        onDismissRequest = onDismiss,
        solid = true,
        title = { Text(if (prompt.changedFrom != null) "Certificate changed" else "Trust this server?") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    if (prompt.changedFrom != null) {
                        "The certificate has changed since you last trusted it. Confirm the fingerprint before continuing."
                    } else {
                        "This subscription's server uses a certificate your device does not yet trust."
                    },
                    style = MaterialTheme.typography.bodyMedium,
                )
                Text(
                    prompt.fingerprint,
                    style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        },
        confirmButton = { TextButton(onClick = onTrust) { Text("Trust and continue") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@Composable
internal fun statusColor(status: String): Color = when (status) {
    "suspended", "expired" -> DigitDnsTheme.colors.danger
    "quota" -> DigitDnsTheme.colors.warning
    else -> DigitDnsTheme.colors.success
}
