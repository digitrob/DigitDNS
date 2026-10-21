package com.digitrob.digitdns.ui.clients

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.domain.model.Client
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.PanelScreenHeader
import com.digitrob.digitdns.ui.components.QuotaBar
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.components.clickableText
import com.digitrob.digitdns.ui.formatBytes
import com.digitrob.digitdns.ui.formatDayDelta
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

/**
 * The client list: every subscriber, searchable, each showing the two things an
 * operator scans for — is it active, and how much quota is left.
 */
@Composable
fun ClientsScreen(
    onOpenClient: (String) -> Unit,
    onCreate: () -> Unit,
    viewModel: ClientsViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()

    // Refetch whenever the list comes back into view — returning from a client detail
    // screen (where a client may have been created, renamed or deleted) must not leave
    // a stale list on screen.
    LifecycleResumeEffect(Unit) {
        viewModel.refresh()
        onPauseOrDispose { }
    }

    Column(modifier = Modifier.fillMaxSize()) {
        PanelScreenHeader {
            IconButton(onClick = onCreate) {
                Icon(Icons.Filled.Add, contentDescription = "New client")
            }
        }

        OutlinedTextField(
            value = state.query,
            onValueChange = viewModel::onQueryChange,
            label = { Text("Search name, id or IP") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp),
        )

        PanelRefreshBox(
            isRefreshing = state.isLoading,
            onRefresh = viewModel::refresh,
            modifier = Modifier.fillMaxSize(),
        ) {
            Column(modifier = Modifier.fillMaxSize()) {
                // A failed refresh is shown above the rows, not instead of them: the
                // stale list is still useful and the error explains why it is stale.
                state.error?.takeIf { state.clients.isNotEmpty() }?.let { message ->
                    Text(
                        text = message,
                        color = DigitDnsTheme.colors.danger,
                        style = MaterialTheme.typography.bodySmall,
                        modifier = Modifier.padding(horizontal = 12.dp, vertical = 4.dp),
                    )
                }
                when {
                    state.isLoading && state.clients.isEmpty() -> CenterBox {
                        CircularProgressIndicator(color = MaterialTheme.colorScheme.primary)
                    }
                    state.error != null && state.clients.isEmpty() -> CenterBox {
                        Text(state.error!!, color = DigitDnsTheme.colors.danger)
                    }
                    state.filtered.isEmpty() -> CenterBox {
                        Text(
                            text = if (state.query.isBlank()) "No clients yet." else "Nothing matches \"${state.query}\".",
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    else -> LazyColumn(
                        modifier = Modifier.fillMaxSize(),
                        contentPadding = androidx.compose.foundation.layout.PaddingValues(12.dp),
                        verticalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        items(state.filtered, key = { it.id }) { client ->
                            ClientRow(client = client, onClick = { onOpenClient(client.id) })
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun ClientRow(client: Client, onClick: () -> Unit) {
    val colors = DigitDnsTheme.colors
    PanelCard(modifier = Modifier.fillMaxWidth().clickableText(onClick)) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    text = client.name.ifBlank { client.id },
                    style = MaterialTheme.typography.titleMedium,
                    color = MaterialTheme.colorScheme.onSurface,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f),
                )
                Spacer(Modifier.width(8.dp))
                StatusBadge(
                    text = clientStatusLabel(client),
                    color = clientStatusColor(client, colors),
                )
            }

            val detail = buildString {
                val used = formatBytes(client.trafficUsedBytes)
                if (client.hasQuota) {
                    val limitBytes = (client.trafficLimitGb * Client.BYTES_PER_GB).toLong()
                    append("$used / ${formatBytes(limitBytes)}")
                } else {
                    append("$used used")
                }
                client.daysUntilExpiry?.let { append("  ·  expires ${formatDayDelta(it)}") }
            }
            Text(
                text = detail,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )

            if (client.hasQuota) {
                QuotaBar(fraction = client.quotaFraction)
            }
        }
    }
}

/** Days-to-expiry at or under which an account is shown as "Expiring" (amber). */
private const val EXPIRING_THRESHOLD_DAYS = 3L

private fun Client.isExpiringSoon(): Boolean =
    daysUntilExpiry?.let { it <= EXPIRING_THRESHOLD_DAYS } == true

/** The label a status badge shows, in priority order: the most blocking wins. */
private fun clientStatusLabel(client: Client): String = when {
    client.quotaExceeded -> "Quota"
    client.isExpired -> "Expired"
    !client.enabled -> "Disabled"
    client.isExpiringSoon() -> "Expiring"
    else -> "Active"
}

private fun clientStatusColor(client: Client, colors: com.digitrob.digitdns.ui.theme.ExtendedColors) = when {
    client.quotaExceeded || client.isExpired || !client.enabled -> colors.danger
    client.isExpiringSoon() -> colors.warning
    else -> colors.success
}

@Composable
private fun CenterBox(content: @Composable () -> Unit) {
    Column(
        modifier = Modifier.fillMaxSize(),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) { content() }
}
