package com.digitrob.digitdns.ui.subscriber

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Link
import androidx.compose.material.icons.filled.Palette
import androidx.compose.material.icons.filled.Person
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.FilterChip
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.digitrob.digitdns.data.ThemeMode
import com.digitrob.digitdns.domain.model.SubscriberLink
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelDialog
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono

/** The subscriber surface's account, appearance and exit controls. */
@Composable
internal fun SettingsTab(
    state: SubscriberUiState,
    viewModel: SubscriberViewModel,
    onCopy: (String) -> Unit,
    onSwitchToAdmin: () -> Unit,
) {
    val context = LocalContext.current
    var confirmForget by remember { mutableStateOf(false) }
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .verticalScroll(rememberScrollState())
            .padding(bottom = 12.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        SubscriptionCard(
            state = state,
            link = viewModel.currentLink(),
            onCopy = onCopy,
            onOpenPortal = { url ->
                context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
            },
            onRefresh = viewModel::refresh,
        )
        AppearanceCard(state, viewModel)
        AccountCard(
            canSwitchToAdmin = state.adminAvailable,
            onSwitchToAdmin = onSwitchToAdmin,
            onSignOut = viewModel::signOut,
            onForget = { confirmForget = true },
        )
    }

    if (confirmForget) {
        ForgetDialog(
            onCancel = { confirmForget = false },
            onConfirm = {
                confirmForget = false
                viewModel.forget()
            },
        )
    }
}

@Composable
private fun SubscriptionCard(
    state: SubscriberUiState,
    link: SubscriberLink?,
    onCopy: (String) -> Unit,
    onOpenPortal: (String) -> Unit,
    onRefresh: () -> Unit,
) {
    SubscriberCard(verticalPadding = 10.dp, title = "Subscription", icon = Icons.Filled.Link) {
        Text(
            text = link?.authority ?: "—",
            style = MaterialTheme.typography.titleSmall.copy(
                fontFamily = PanelMono,
                fontWeight = FontWeight.SemiBold,
            ),
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        Spacer(Modifier.height(4.dp))
        Text(
            text = link?.maskedPortalUrl ?: "No link",
            style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            StatusBadge(
                text = if (state.secretSaved) "Secret saved" else "Secret not saved",
                color = if (state.secretSaved) DigitDnsTheme.colors.success else DigitDnsTheme.colors.warning,
            )
            Spacer(Modifier.weight(1f))
            IconButton(
                onClick = { link?.portalUrl?.let(onCopy) },
                enabled = link != null,
                modifier = Modifier.size(32.dp),
            ) {
                Icon(
                    Icons.Filled.ContentCopy,
                    contentDescription = "Copy subscription link",
                    modifier = Modifier.size(16.dp),
                    tint = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton(onClick = { link?.portalUrl?.let(onOpenPortal) }) { Text("Open portal") }
            OutlinedButton(onClick = onRefresh) { Text("Refresh") }
        }
    }
}

@Composable
private fun AppearanceCard(state: SubscriberUiState, viewModel: SubscriberViewModel) {
    SubscriberCard(verticalPadding = 10.dp, title = "Appearance", icon = Icons.Filled.Palette) {
        Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            ThemeMode.entries.forEach { mode ->
                FilterChip(
                    selected = state.themeMode == mode,
                    onClick = { viewModel.setTheme(mode) },
                    label = {
                        Text(
                            when (mode) {
                                ThemeMode.SYSTEM -> "System"
                                ThemeMode.DARK -> "Dark"
                                ThemeMode.LIGHT -> "Light"
                            },
                        )
                    },
                )
            }
        }
    }
}

@Composable
private fun AccountCard(
    canSwitchToAdmin: Boolean,
    onSwitchToAdmin: () -> Unit,
    onSignOut: () -> Unit,
    onForget: () -> Unit,
) {
    SubscriberCard(verticalPadding = 10.dp, title = "Account", icon = Icons.Filled.Person) {
        Text(
            "Signing out keeps the link saved on this device. Forgetting removes the " +
                "link, the secret and this server's certificate pin.",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.height(6.dp))
        // The admin session is still signed in underneath; this only moves the app
        // back to it, so there is nothing to re-authenticate. With no admin login on
        // this device the control stays visible but disabled, which explains itself
        // better than a button that silently is not there.
        OutlinedButton(
            onClick = onSwitchToAdmin,
            enabled = canSwitchToAdmin,
            modifier = Modifier.fillMaxWidth(),
        ) { Text("Switch to Admin") }
        if (!canSwitchToAdmin) {
            Text(
                "No admin account is signed in on this device.",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        Spacer(Modifier.height(6.dp))
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(onClick = onSignOut) { Text("Sign out") }
            TextButton(onClick = onForget) {
                Text("Forget subscription", color = DigitDnsTheme.colors.danger)
            }
        }
    }
}

@Composable
private fun ForgetDialog(onCancel: () -> Unit, onConfirm: () -> Unit) {
    PanelDialog(
        onDismissRequest = onCancel,
        solid = true,
        title = { Text("Forget this subscription?") },
        text = {
            Text(
                "The saved link and registration secret are removed from this device. " +
                    "You can sign in again with the link your provider gave you.",
            )
        },
        confirmButton = {
            TextButton(onClick = onConfirm) {
                Text("Forget", color = DigitDnsTheme.colors.danger)
            }
        },
        dismissButton = { TextButton(onClick = onCancel) { Text("Cancel") } },
    )
}
