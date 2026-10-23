package com.digitrob.digitdns.ui.clients

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.AllInclusive
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Link
import androidx.compose.material.icons.filled.Public
import androidx.compose.material.icons.filled.Timer
import androidx.compose.material3.AssistChip
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Switch
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.domain.model.Client
import com.digitrob.digitdns.domain.model.ClientLinks
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelDialog
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.PanelScreenHeader
import com.digitrob.digitdns.ui.components.PanelSnackbarHost
import com.digitrob.digitdns.ui.components.SwipeToDeleteRow
import com.digitrob.digitdns.ui.components.QuotaBar
import com.digitrob.digitdns.ui.components.StatusBadge
import com.digitrob.digitdns.ui.components.clickableText
import com.digitrob.digitdns.ui.components.showPanelToast
import com.digitrob.digitdns.ui.formatBytes
import com.digitrob.digitdns.ui.formatDayDelta
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import kotlinx.coroutines.launch

/**
 * The client list: every subscriber, searchable, each showing the two things an
 * operator scans for — is it active, and how much quota is left.
 *
 * The quick actions (link, bot card, +30 days) live on the row, matching the web
 * card; the panel's three buttons are the reason an operator opens this screen at
 * all, and a popup for each one would put two taps where the panel has one.
 */
@Composable
fun ClientsScreen(
    viewModel: ClientsViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    var showCreate by remember { mutableStateOf(false) }
    // The client being edited. The list stays on screen behind the popup, so the
    // operator closes it and carries on with the row they were reading.
    var editingClientId by remember { mutableStateOf<String?>(null) }
    // Hoisted so the client can be loaded *before* the popup mounts; otherwise the
    // first frame is a small spinner (or the previous client) that the real form then
    // replaces, which reads as the window jumping.
    val detailViewModel: ClientDetailViewModel =
        hiltViewModel(key = ClientDetailViewModel.DETAIL_VIEWMODEL_KEY)

    val clipboard = LocalClipboardManager.current
    val snackbar = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()

    val copy: (String, String) -> Unit = { value, label ->
        clipboard.setText(AnnotatedString(value))
        scope.launch { snackbar.showPanelToast("$label copied.") }
    }

    // Refetch whenever the list comes back into view.
    LifecycleResumeEffect(Unit) {
        viewModel.refresh()
        onPauseOrDispose { }
    }

    LaunchedEffect(state.message) {
        val message = state.message ?: return@LaunchedEffect
        snackbar.showPanelToast(message)
        viewModel.dismissMessage()
    }

    Box(modifier = Modifier.fillMaxSize()) {
        Column(modifier = Modifier.fillMaxSize()) {
            ClientsTopBar(
                query = state.query,
                allowAll = state.allowAll,
                onQueryChange = viewModel::onQueryChange,
                onNewClient = { showCreate = true },
                onAccessModeChange = viewModel::onAccessModeChange,
            )

            PanelRefreshBox(
                isRefreshing = state.isRefreshing,
                onRefresh = { viewModel.refresh(userInitiated = true) },
                modifier = Modifier.fillMaxSize(),
            ) {
                ClientList(
                    state = state,
                    onOpen = {
                        detailViewModel.open(it)
                        editingClientId = it
                    },
                    onRenew = viewModel::renew,
                    onMakeLifetime = viewModel::makeLifetime,
                    onDelete = viewModel::delete,
                    onCopy = copy,
                )
            }
        }

        Box(modifier = Modifier.align(Alignment.BottomCenter)) {
            PanelSnackbarHost(hostState = snackbar)
        }
    }

    ClientDialogs(
        showCreate = showCreate,
        editingClientId = editingClientId,
        origin = state.origin,
        onCloseCreate = { showCreate = false },
        onCloseDetail = { editingClientId = null },
        onRefreshList = { viewModel.refresh() },
        detailViewModel = detailViewModel,
    )
}

/** The add action, search box and the server-wide "allow all" notice. */
@Composable
private fun ClientsTopBar(
    query: String,
    allowAll: Boolean,
    onQueryChange: (String) -> Unit,
    onNewClient: () -> Unit,
    onAccessModeChange: (Boolean) -> Unit,
) {
    PanelScreenHeader {
        IconButton(onClick = onNewClient) {
            Icon(Icons.Filled.Add, contentDescription = "New client")
        }
    }

    OutlinedTextField(
        value = query,
        onValueChange = onQueryChange,
        label = { Text("Search name, id or IP") },
        singleLine = true,
        modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp),
    )

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 12.dp, vertical = 4.dp),
        horizontalArrangement = Arrangement.Center,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            Icons.Filled.Lock,
            contentDescription = null,
            tint = DigitDnsTheme.colors.success,
            modifier = Modifier.size(18.dp),
        )
        Spacer(Modifier.width(4.dp))
        Text("Public Mode", style = MaterialTheme.typography.bodyMedium)
        Spacer(Modifier.width(8.dp))
        Switch(
            checked = !allowAll,
            onCheckedChange = { onAccessModeChange(!it) },
        )
        Spacer(Modifier.width(8.dp))
        Text("Whitelist Mode", style = MaterialTheme.typography.bodyMedium)
        Spacer(Modifier.width(4.dp))
        Icon(
            Icons.Filled.Lock,
            contentDescription = null,
            tint = DigitDnsTheme.colors.info,
            modifier = Modifier.size(18.dp),
        )
    }
}

/** The row callbacks grouped so the card's parameter list stays readable. */
private data class ClientRowActions(
    val onOpen: () -> Unit,
    val onRenew: () -> Unit,
    val onMakeLifetime: () -> Unit,
    val onCopy: (String, String) -> Unit,
)

@Composable
private fun ClientRow(
    client: Client,
    origin: String,
    dnsAddress: String,
    busy: Boolean,
    actions: ClientRowActions,
) {
    val colors = DigitDnsTheme.colors
    var showBotCard by remember { mutableStateOf(false) }

    PanelCard(modifier = Modifier.fillMaxWidth().clickableText(actions.onOpen)) {
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

            QuickActions(
                client = client,
                origin = origin,
                busy = busy,
                onShowBotCard = { showBotCard = true },
                onRenew = actions.onRenew,
                onMakeLifetime = actions.onMakeLifetime,
                onCopy = actions.onCopy,
            )
        }
    }

    if (showBotCard) {
        BotCardDialog(
            client = client,
            origin = origin,
            dnsAddress = dnsAddress,
            onDismiss = { showBotCard = false },
            onCopy = actions.onCopy,
        )
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun QuickActions(
    client: Client,
    origin: String,
    busy: Boolean,
    onShowBotCard: () -> Unit,
    onRenew: () -> Unit,
    onMakeLifetime: () -> Unit,
    onCopy: (String, String) -> Unit,
) {
    val subscription = if (origin.isBlank()) "" else ClientLinks.subscriptionLink(origin, client)

    Row(
        horizontalArrangement = Arrangement.spacedBy(4.dp),
        modifier = Modifier
            .fillMaxWidth()
            .horizontalScroll(rememberScrollState()),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        LinkChip(subscription = subscription, onCopy = onCopy)

        AssistChip(
            onClick = onShowBotCard,
            enabled = origin.isNotBlank(),
            leadingIcon = { Icon(Icons.AutoMirrored.Filled.Send, contentDescription = null, modifier = Modifier.size(16.dp)) },
            label = { Text("Bot", style = MaterialTheme.typography.labelSmall) },
        )

        AssistChip(
            onClick = onRenew,
            enabled = !busy,
            leadingIcon = {
                if (busy) {
                    CircularProgressIndicator(strokeWidth = 2.dp, modifier = Modifier.size(16.dp))
                } else {
                    Icon(Icons.Filled.Timer, contentDescription = null, modifier = Modifier.size(16.dp))
                }
            },
            label = { Text("+30d", style = MaterialTheme.typography.labelSmall) },
        )

        AssistChip(
            onClick = onMakeLifetime,
            enabled = !busy && client.expiresAt != null,
            leadingIcon = {
                Icon(
                    Icons.Filled.AllInclusive,
                    contentDescription = null,
                    modifier = Modifier.size(16.dp),
                )
            },
            label = { Text("Lifetime", style = MaterialTheme.typography.labelSmall) },
        )

        // Fills the end of the slideable row so the last chip never touches the border.
        Spacer(Modifier.width(8.dp))
    }
}

/** The link chip and its popup; the popup owns no state outside this row. */
@Composable
private fun LinkChip(
    subscription: String,
    onCopy: (String, String) -> Unit,
) {
    var showLink by remember { mutableStateOf(false) }
    Box {
        AssistChip(
            onClick = { showLink = true },
            leadingIcon = { Icon(Icons.Filled.Link, contentDescription = null, modifier = Modifier.size(16.dp)) },
            label = { Text("Link", style = MaterialTheme.typography.labelSmall) },
        )
        val colors = DigitDnsTheme.colors
        DropdownMenu(
            expanded = showLink,
            onDismissRequest = { showLink = false },
            modifier = Modifier
                .background(colors.bgCard)
                .border(1.dp, colors.borderColor, RoundedCornerShape(12.dp)),
        ) {
            Text(
                text = subscription.ifBlank { "Subscription origin is not configured." },
                style = MaterialTheme.typography.bodySmall,
                modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp).widthIn(max = 280.dp),
            )
            if (subscription.isNotBlank()) {
                DropdownMenuItem(
                    text = { Text("Copy link") },
                    leadingIcon = { Icon(Icons.Filled.ContentCopy, contentDescription = null) },
                    onClick = {
                        showLink = false
                        onCopy(subscription, "Subscription link")
                    },
                )
            }
        }
    }
}

/** The bot provisioning text, shown whole: it is meant to be copied, not read in the app. */
@Composable
private fun BotCardDialog(
    client: Client,
    origin: String,
    dnsAddress: String,
    onDismiss: () -> Unit,
    onCopy: (String, String) -> Unit,
) {
    val message = ClientLinks.telegramBotCard(
        client = client,
        origin = origin,
        dnsAddress = dnsAddress,
        expiresLabel = ClientLinks.expiryLabel(client.expiresAt),
    )
    PanelDialog(
        onDismissRequest = onDismiss,
        title = { Text("Telegram bot card") },
        solid = true,
        text = {
            Text(
                text = message,
                style = MaterialTheme.typography.bodySmall.copy(fontFamily = FontFamily.Monospace),
                modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()),
            )
        },
        confirmButton = {
            TextButton(onClick = {
                onCopy(message, "Bot card")
                onDismiss()
            }) { Text("Copy") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Close") } },
    )
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

@Composable
private fun ClientList(
    state: ClientsUiState,
    onOpen: (String) -> Unit,
    onRenew: (Client, Int) -> Unit,
    onMakeLifetime: (Client) -> Unit,
    onDelete: (Client) -> Unit,
    onCopy: (String, String) -> Unit,
) {
    Column(modifier = Modifier.fillMaxSize()) {
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
                    SwipeToDeleteRow(
                        onDelete = { onDelete(client) },
                        confirmTitle = "Delete client?",
                        confirmText = "Their subscription link and whitelisted address stop working immediately. This cannot be undone.",
                        confirmLabel = "Delete",
                    ) {
                        ClientRow(
                            client = client,
                            origin = state.origin,
                            dnsAddress = state.dnsAddress,
                            busy = state.busyId == client.id,
                            actions = ClientRowActions(
                                onOpen = { onOpen(client.id) },
                                onRenew = { onRenew(client, 30) },
                                onMakeLifetime = { onMakeLifetime(client) },
                                onCopy = onCopy,
                            ),
                        )
                    }
                }
            }
        }
    }
}

/**
 * Both client popups, kept out of the list body so the screen reads as list first.
 *
 * Each popup's view model is looked up here (under its own key) so both can live in
 * this tab's store: the create form resets itself when it mounts, and the detail form
 * loads whichever client it was opened for.
 */
@Composable
private fun ClientDialogs(
    showCreate: Boolean,
    editingClientId: String?,
    origin: String,
    onCloseCreate: () -> Unit,
    onCloseDetail: () -> Unit,
    onRefreshList: () -> Unit,
    detailViewModel: ClientDetailViewModel,
) {
    if (showCreate) {
        NewClientDialog(
            origin = origin,
            onDismiss = onCloseCreate,
            onCreated = {
                onCloseCreate()
                // The list is behind the popup, so it never saw the new client.
                onRefreshList()
            },
            viewModel = hiltViewModel(key = ClientDetailViewModel.CREATE_VIEWMODEL_KEY),
        )
    }

    if (editingClientId != null) {
        ClientDetailDialog(
            clientId = editingClientId,
            origin = origin,
            onDismiss = onCloseDetail,
            onChanged = {
                onCloseDetail()
                // Saved or deleted behind the popup: the list never saw it.
                onRefreshList()
            },
            viewModel = detailViewModel,
        )
    }
}
