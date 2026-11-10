package com.digitrob.digitdns.ui.components

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AddCircle
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.digitrob.digitdns.R
import com.digitrob.digitdns.core.network.UpstreamPresetProvider
import com.digitrob.digitdns.core.network.UpstreamPresetServer
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono
import com.digitrob.digitdns.ui.theme.protocolColor
import com.digitrob.digitdns.ui.theme.protocolLabel
import java.util.Locale

/**
 * Everything the shared provider browser needs to draw. Kept as data rather than
 * a dozen parameters so both the Rules upstream screen and the dashboard racer
 * card can use the same, larger-type dropdown.
 */
data class UpstreamPresetBrowserState(
    val providers: List<UpstreamPresetProvider>,
    val query: String,
    val added: Set<String>,
    val expanded: Set<String>,
    val loading: Boolean,
    val loaded: Boolean,
    val error: String?,
    val adding: String?,
)

data class UpstreamPresetBrowserActions(
    val onToggleProvider: (String) -> Unit,
    val onAdd: (String) -> Unit,
    val onRemove: (String) -> Unit,
    val onClose: () -> Unit,
)

@Composable
fun UpstreamPresetBrowser(
    state: UpstreamPresetBrowserState,
    actions: UpstreamPresetBrowserActions,
    modifier: Modifier = Modifier,
) {
    val query = state.query.trim()
    val providers = remember(state.providers, query) { filterPresetProviders(state.providers, query) }
    // Solid, not the translucent glass card: a nested translucent panel let the
    // racer/list underneath show through behind this heading and the rows.
    PanelCard(modifier = modifier.fillMaxWidth(), contentPadding = 12.dp, solid = true) {
        // PanelCard is a Box. Without this Column every child below would be
        // stacked on top of the others, which is why the heading appeared over
        // the first provider rows.
        Column(modifier = Modifier.fillMaxWidth()) {
            Row(
            modifier = Modifier
                .fillMaxWidth()
                .clip(RoundedCornerShape(8.dp))
                .background(MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.45f))
                .padding(start = 10.dp, end = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                text = "Public DNS providers",
                style = MaterialTheme.typography.titleSmall.copy(fontSize = 15.sp),
                fontWeight = FontWeight.Bold,
                color = MaterialTheme.colorScheme.onSurface,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f).padding(end = 8.dp),
            )
            IconButton(onClick = actions.onClose) {
                Icon(
                    Icons.Filled.Close,
                    contentDescription = "Close DNS provider list",
                    tint = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        Text(
            text = "Tap a server to add it. Tap a checked server to remove it.",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(start = 10.dp, end = 10.dp, top = 6.dp, bottom = 6.dp),
        )
        HorizontalDivider(
            color = MaterialTheme.colorScheme.outline.copy(alpha = 0.45f),
            modifier = Modifier.padding(horizontal = 2.dp),
        )
        Spacer(Modifier.height(6.dp))
        when {
            state.loading && state.providers.isEmpty() -> Text(
                text = "Loading providers…",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            state.error != null && state.providers.isEmpty() -> Text(
                text = state.error,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            else -> PresetBrowserList(
                state = state,
                providers = providers,
                query = query,
                actions = actions,
            )
            }
        }
    }
}

@Composable
private fun PresetBrowserList(
    state: UpstreamPresetBrowserState,
    providers: List<UpstreamPresetProvider>,
    query: String,
    actions: UpstreamPresetBrowserActions,
) {
    val exact = state.providers.any { provider ->
        provider.servers.any { it.address.equals(query, ignoreCase = true) }
    }
    LazyColumn(
        modifier = Modifier.fillMaxWidth().heightIn(max = 420.dp),
        verticalArrangement = Arrangement.spacedBy(2.dp),
    ) {
        if (query.isNotBlank() && !exact) {
            item(key = "custom") {
                CustomPresetRow(query = query, onAdd = actions.onAdd)
            }
        }
        providers.forEach { provider ->
            val expanded = query.isNotBlank() || state.expanded.contains(provider.id)
            item(key = "provider:${provider.id}") {
                ProviderHeader(
                    provider = provider,
                    expanded = expanded,
                    onToggle = { actions.onToggleProvider(provider.id) },
                )
            }
            if (expanded) {
                if (provider.note.isNotBlank()) {
                    item(key = "note:${provider.id}") {
                        Text(
                            text = provider.note,
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier = Modifier.padding(start = 38.dp, end = 8.dp, bottom = 4.dp),
                        )
                    }
                }
                items(provider.servers, key = { "server:${it.address}" }) { server ->
                    PresetServerRow(
                        server = server,
                        added = state.added.contains(server.address),
                        adding = state.adding == server.address,
                        onAdd = { actions.onAdd(server.address) },
                        onRemove = { actions.onRemove(server.address) },
                    )
                }
            }
        }
        if (providers.isEmpty() && query.isBlank()) {
            item(key = "empty") {
                Text(
                    text = "No providers available.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(vertical = 8.dp),
                )
            }
        }
    }
}

@Composable
private fun ProviderHeader(
    provider: UpstreamPresetProvider,
    expanded: Boolean,
    onToggle: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(8.dp))
            .clickable(onClick = onToggle)
            .padding(horizontal = 4.dp, vertical = 7.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        ProviderLogo(provider.id)
        Spacer(Modifier.width(10.dp))
        Text(
            text = provider.name,
            style = MaterialTheme.typography.bodyLarge.copy(fontSize = 17.sp),
            fontWeight = FontWeight.SemiBold,
            modifier = Modifier.weight(1f),
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        Text(
            text = provider.servers.size.toString(),
            style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Icon(
            Icons.Filled.ArrowDropDown,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.rotate(if (expanded) 180f else 0f),
        )
    }
}

@Composable
private fun ProviderLogo(id: String) {
    val res = providerLogoRes(id)
    if (res != null) {
        Image(
            painter = painterResource(res),
            contentDescription = null,
            modifier = Modifier.size(26.dp).clip(RoundedCornerShape(5.dp)),
        )
    } else {
        Box(
            modifier = Modifier
                .size(24.dp)
                .clip(RoundedCornerShape(5.dp))
                .background(MaterialTheme.colorScheme.surfaceVariant),
            contentAlignment = Alignment.Center,
        ) {
            Text(
                text = id.take(1).uppercase(Locale.ROOT),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun PresetServerRow(
    server: UpstreamPresetServer,
    added: Boolean,
    adding: Boolean,
    onAdd: () -> Unit,
    onRemove: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(8.dp))
            .clickable(enabled = !adding, onClick = if (added) onRemove else onAdd)
            .padding(start = 38.dp, end = 6.dp, top = 6.dp, bottom = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = server.label.ifBlank { server.address },
            style = MaterialTheme.typography.bodyMedium.copy(fontFamily = PanelMono, fontSize = 15.sp),
            modifier = Modifier.weight(1f),
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        Text(
            text = protocolLabel(server.protocol),
            style = MaterialTheme.typography.labelMedium.copy(
                fontFamily = PanelMono,
                fontWeight = FontWeight.Bold,
                fontSize = 13.sp,
            ),
            color = protocolColor(server.protocol),
            maxLines = 1,
        )
        Spacer(Modifier.width(10.dp))
        Box(modifier = Modifier.width(22.dp), contentAlignment = Alignment.Center) {
            when {
                adding -> CircularProgressIndicator(
                    modifier = Modifier.size(16.dp),
                    strokeWidth = 2.dp,
                    color = DigitDnsTheme.colors.info,
                )
                added -> Icon(
                    Icons.Filled.CheckCircle,
                    contentDescription = "Remove ${server.address} from the upstream list",
                    tint = DigitDnsTheme.colors.success,
                    modifier = Modifier.size(21.dp),
                )
                else -> Icon(
                    Icons.Filled.AddCircle,
                    contentDescription = "Add ${server.address} to the upstream list",
                    tint = DigitDnsTheme.colors.info,
                    modifier = Modifier.size(21.dp),
                )
            }
        }
    }
}

@Composable
private fun CustomPresetRow(query: String, onAdd: (String) -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(8.dp))
            .clickable { onAdd(query) }
            .padding(horizontal = 8.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            text = "Use custom: $query",
            style = MaterialTheme.typography.bodyMedium.copy(fontFamily = PanelMono, fontSize = 15.sp),
            color = DigitDnsTheme.colors.info,
            modifier = Modifier.weight(1f),
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        Icon(
            Icons.Filled.AddCircle,
            contentDescription = "Add custom address",
            tint = DigitDnsTheme.colors.info,
            modifier = Modifier.size(21.dp),
        )
    }
}

internal fun filterPresetProviders(
    providers: List<UpstreamPresetProvider>,
    query: String,
): List<UpstreamPresetProvider> {
    val q = query.trim().lowercase(Locale.ROOT)
    if (q.isEmpty()) return providers
    return providers.mapNotNull { provider ->
        val nameMatch = provider.name.lowercase(Locale.ROOT).contains(q) ||
            provider.id.lowercase(Locale.ROOT).contains(q)
        val servers = if (nameMatch) {
            provider.servers
        } else {
            provider.servers.filter { server ->
                server.address.lowercase(Locale.ROOT).contains(q) ||
                    server.label.lowercase(Locale.ROOT).contains(q) ||
                    server.protocol.lowercase(Locale.ROOT).contains(q)
            }
        }
        if (servers.isEmpty()) null else provider.copy(servers = servers)
    }
}

private fun providerLogoRes(id: String): Int? = when (id) {
    "cloudflare" -> R.drawable.provider_cloudflare
    "google" -> R.drawable.provider_google
    "quad9" -> R.drawable.provider_quad9
    "opendns" -> R.drawable.provider_opendns
    "adguard" -> R.drawable.provider_adguard
    "controld" -> R.drawable.provider_controld
    "dnssb" -> R.drawable.provider_dnssb
    "yandex" -> R.drawable.provider_yandex
    "nextdns" -> R.drawable.provider_nextdns
    "cleanbrowsing" -> R.drawable.provider_cleanbrowsing
    else -> null
}
