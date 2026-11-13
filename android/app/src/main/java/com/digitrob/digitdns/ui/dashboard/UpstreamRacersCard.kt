package com.digitrob.digitdns.ui.dashboard

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Podcasts
import androidx.compose.material.icons.filled.Public
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
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
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.digitrob.digitdns.R
import com.digitrob.digitdns.core.network.ALL_TRANSPORTS
import com.digitrob.digitdns.core.network.ResolutionSettings
import com.digitrob.digitdns.core.network.UpstreamPresetProvider
import com.digitrob.digitdns.core.network.UpstreamStat
import com.digitrob.digitdns.core.network.normalizedAdaptiveIntervalMinutes
import com.digitrob.digitdns.core.network.normalizedResolutionStrategy
import com.digitrob.digitdns.domain.model.relayRelativeAge
import com.digitrob.digitdns.core.network.normalizedTransports
import com.digitrob.digitdns.ui.components.DigitDnsDropdownMenu
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelRowDefaults
import com.digitrob.digitdns.ui.components.SwipeToDeleteRow
import com.digitrob.digitdns.ui.components.UpstreamPresetBrowser
import com.digitrob.digitdns.ui.components.UpstreamPresetBrowserActions
import com.digitrob.digitdns.ui.components.UpstreamPresetBrowserState
import com.digitrob.digitdns.ui.formatCount
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.ExtendedColors
import com.digitrob.digitdns.ui.theme.LocalIsGreyTheme
import com.digitrob.digitdns.ui.theme.PanelMono
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * The Upstream Racers tile: the resolver pool ranked by latency, with the panel's two
 * mutating actions inline — add an upstream, and remove one behind the same
 * confirmation every other swipe-to-delete row in the app uses.
 *
 * Kept in its own file rather than alongside the other dashboard cards because it is the
 * only tile with write actions: it owns transient form state and a dialog, which the
 * read-only stat tiles do not.
 */
@Composable
internal fun UpstreamRacersCard(
    upstreams: List<UpstreamStat>,
    isBenchmarking: Boolean,
    onBenchmark: () -> Unit,
    actions: UpstreamActions,
    /** False while the dashboard grid is scrolling: reading the page must not arm a delete. */
    swipeEnabled: Boolean = true,
    resolutionControls: ResolutionControls = ResolutionControls(),
) {
    val colors = DigitDnsTheme.colors
    var removeError by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            RacerHeader(isBenchmarking = isBenchmarking, onBenchmark = onBenchmark)
            UpstreamAddSection(upstreams = upstreams, actions = actions)

            val policy = poolPolicy(resolutionControls.settings, upstreams)
            val ordered = orderedResolvers(upstreams, policy)
            val adaptiveCurrent = resolutionControls.settings
                ?.takeIf { normalizedResolutionStrategy(it.defaultStrategy) == "adaptive" }
                ?.adaptiveCurrent
                .orEmpty()

            if (upstreams.isEmpty()) {
                Text(
                    text = stringResource(R.string.upstream_none),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            } else {
                // Five rows is the whole viewport. Past that only this box scrolls:
                // the add row, the defaults and the card itself stay put.
                Box(
                    modifier = Modifier
                        .fillMaxWidth()
                        .heightIn(max = PanelRowDefaults.height * 5 + 8.dp * 4),
                ) {
                    Column(
                        modifier = Modifier.verticalScroll(rememberScrollState()),
                        verticalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        ordered.forEach { stat ->
                            UpstreamRow(
                                stat = stat,
                                swipeEnabled = swipeEnabled,
                                dimmed = upstreamProtocolKey(stat.address) !in policy.transports ||
                                    resolverExcludedByTransport(stat.address, policy.transport, policy.encrypted) ||
                                    (policy.pinActive && stat.address != policy.pinned),
                                isAdaptiveCurrent = stat.address == adaptiveCurrent,
                                adaptiveLastRun = resolutionControls.settings?.adaptiveLastRun.orEmpty(),
                                onRemove = {
                                    scope.launch { removeError = actions.onRemove(stat.address) }
                                },
                            )
                        }
                    }
                }
            }

            removeError?.let {
                Text(
                    text = it,
                    style = MaterialTheme.typography.bodySmall,
                    color = colors.danger,
                )
            }

            // Global resolution defaults. Per-domain overrides live in Domains.
            ResolutionDefaults(
                resolution = resolutionControls.settings,
                configured = upstreams.map { it.address },
                onChange = resolutionControls.onChange,
                isAdaptiveRunning = resolutionControls.isAdaptiveRunning,
                onRunAdaptive = resolutionControls.onRunAdaptive,
                lastSingleUpstream = resolutionControls.lastSingleUpstream,
            )
        }
    }
}

/** The add field and the shared provider browser, kept together so their local
 *  form state does not make the racer card itself long and branch-heavy. */
@Suppress("CyclomaticComplexMethod") // Form validation + provider state are one cohesive unit.
@Composable
private fun UpstreamAddSection(
    upstreams: List<UpstreamStat>,
    actions: UpstreamActions,
) {
    val colors = DigitDnsTheme.colors
    var newAddress by remember { mutableStateOf("") }
    var error by remember { mutableStateOf<String?>(null) }
    var presetsOpen by remember { mutableStateOf(false) }
    var presets by remember { mutableStateOf<List<UpstreamPresetProvider>>(emptyList()) }
    var presetsLoading by remember { mutableStateOf(false) }
    var presetsLoaded by remember { mutableStateOf(false) }
    var presetError by remember { mutableStateOf<String?>(null) }
    var expandedProviders by remember { mutableStateOf<Set<String>>(emptySet()) }
    var addingPreset by remember { mutableStateOf<String?>(null) }
    var locallyAdded by remember { mutableStateOf<Set<String>>(emptySet()) }
    val scope = rememberCoroutineScope()

    fun openPresets() {
        presetsOpen = true
        if (presetsLoaded || presetsLoading) return
        presetsLoading = true
        presetError = null
        scope.launch {
            val result = actions.onLoadPresets()
            presetsLoading = false
            presetsLoaded = true
            if (result == null) {
                presetError = "Could not load the provider list. You can still type a resolver address."
            } else {
                presets = result
                if (result.isNotEmpty() && expandedProviders.isEmpty()) {
                    expandedProviders = setOf(result.first().id)
                }
            }
        }
    }

    fun addAddress(address: String) {
        val value = address.trim()
        if (value.isBlank() || addingPreset != null) return
        scope.launch {
            addingPreset = value
            error = null
            val addError = actions.onAdd(value)
            addingPreset = null
            if (addError == null) {
                locallyAdded = locallyAdded + value
                if (value == newAddress.trim()) {
                    newAddress = ""
                    presetsOpen = false
                }
            } else {
                error = addError
            }
        }
    }

    fun removeAddress(address: String) {
        if (addingPreset != null) return
        scope.launch {
            addingPreset = address
            error = null
            val removeError = actions.onRemove(address)
            addingPreset = null
            if (removeError == null) {
                locallyAdded = locallyAdded - address
            } else {
                error = removeError
            }
        }
    }

    UpstreamAddRow(
        value = newAddress,
        onValueChange = { newAddress = it; error = null },
        busy = addingPreset != null,
        presetsOpen = presetsOpen,
        onTogglePresets = { if (presetsOpen) presetsOpen = false else openPresets() },
        onFocus = { openPresets() },
        onSubmit = { addAddress(newAddress) },
    )
    error?.let {
        Text(text = it, style = MaterialTheme.typography.bodySmall, color = colors.danger)
    }
    if (presetsOpen) {
        UpstreamPresetBrowser(
            state = UpstreamPresetBrowserState(
                providers = presets,
                query = newAddress,
                added = upstreams.map { it.address }.toSet() + locallyAdded,
                expanded = expandedProviders,
                loading = presetsLoading,
                loaded = presetsLoaded,
                error = presetError,
                adding = addingPreset,
            ),
            actions = UpstreamPresetBrowserActions(
                onToggleProvider = { id ->
                    expandedProviders = if (expandedProviders.contains(id)) {
                        expandedProviders - id
                    } else {
                        expandedProviders + id
                    }
                },
                onAdd = { addAddress(it) },
                onRemove = { removeAddress(it) },
                onClose = { presetsOpen = false },
            ),
        )
    }
}

/** The three mutating upstream operations the racer card needs. Grouped so the
 *  card and the dashboard grid keep a readable parameter list. */
data class UpstreamActions(
    val onAdd: suspend (String) -> String?,
    val onRemove: suspend (String) -> String?,
    val onLoadPresets: suspend () -> List<UpstreamPresetProvider>?,
)

/** The effective pool policy used for dimming one resolver list. */
private data class PoolPolicy(
    val transport: String,
    val transports: Set<String>,
    val encrypted: Set<String>,
    val pinned: String,
    val pinActive: Boolean,
)

private fun poolPolicy(
    resolution: ResolutionSettings?,
    upstreams: List<UpstreamStat>,
): PoolPolicy {
    val addresses = upstreams.map { it.address }
    val encrypted = resolution?.encryptedUpstreams.orEmpty().toSet()
        .ifEmpty { addresses.filter { resolverLooksEncrypted(it) }.toSet() }
    val pinned = resolution?.defaultUpstream.orEmpty()
    val transport = resolution?.transport ?: "any"
    val transports = normalizedTransports(resolution?.transports ?: ALL_TRANSPORTS).toSet()
    return PoolPolicy(
        transport = transport,
        transports = transports,
        encrypted = encrypted,
        pinned = pinned,
        pinActive = pinned.isNotEmpty() &&
            upstreamProtocolKey(pinned) in transports &&
            !resolverExcludedByTransport(pinned, transport, encrypted),
    )
}

/** Enabled protocols first, then the legacy encrypted/regular filter, preserving latency order. */
private fun orderedResolvers(
    upstreams: List<UpstreamStat>,
    policy: PoolPolicy,
): List<UpstreamStat> {
    val (active, excluded) = upstreams.partition {
        upstreamProtocolKey(it.address) in policy.transports &&
            !resolverExcludedByTransport(it.address, policy.transport, policy.encrypted)
    }
    return active + excluded
}

private fun upstreamProtocolKey(address: String): String {
    val a = address.trim().lowercase()
    return when {
        a.startsWith("tls://") -> "dot"
        a.startsWith("https://") -> "doh"
        a.startsWith("h3://") -> "doh3"
        a.startsWith("quic://") -> "doq"
        a.startsWith("sdns://") -> "dnscrypt"
        else -> "plain"
    }
}

/** The pair the resolver card needs to render and save the global resolution defaults. */
data class ResolutionControls(
    val settings: ResolutionSettings? = null,
    val onChange: (ResolutionSettings) -> Unit = {},
    val isAdaptiveRunning: Boolean = false,
    val onRunAdaptive: () -> Unit = {},
    val lastSingleUpstream: String = "",
)

/**
 * Global resolution defaults: the transport choice, the default upstream and the
 * racing strategy. The encrypted choice is disabled when the resolver list holds
 * no DoT/DoH/DoQ upstream, because selecting it would leave the pool empty.
 */
@Composable
private fun ResolutionDefaults(
    resolution: ResolutionSettings?,
    configured: List<String>,
    onChange: (ResolutionSettings) -> Unit,
    isAdaptiveRunning: Boolean,
    onRunAdaptive: () -> Unit,
    lastSingleUpstream: String,
) {
    val base = resolution ?: ResolutionSettings()
    val strategy = normalizedResolutionStrategy(base.defaultStrategy)
    val single = base.defaultUpstream.isNotEmpty()
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        ChipBlock(label = stringResource(R.string.domains_strategy), dimmed = single) {
            Row(
                modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(2.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                listOf(
                    "adaptive" to stringResource(R.string.strategy_adaptive),
                    "sequential" to stringResource(R.string.strategy_sequential),
                    "hedged" to stringResource(R.string.strategy_hedged),
                    "best" to stringResource(R.string.strategy_best),
                ).forEach { (value, label) ->
                    ResolverChip(
                        label = label,
                        selected = strategy == value,
                        enabled = !single,
                        onClick = { onChange(base.copy(defaultStrategy = value)) },
                    )
                }
            }
        }
        AdaptiveIntervalChips(
            base = base,
            strategy = strategy,
            enabled = !single,
            onChange = onChange,
            isRunning = isAdaptiveRunning,
            onRunAdaptive = onRunAdaptive,
        )
        TransportChips(base = base, enabled = !single, onChange = onChange)
        SingleServerRow(
            base = base,
            configured = configured,
            lastSingleUpstream = lastSingleUpstream,
            onChange = onChange,
        )
    }
}

@Suppress("CyclomaticComplexMethod") // selected/disabled/theme colour matrix is one unit
@Composable
private fun ResolverChip(
    label: String,
    selected: Boolean,
    enabled: Boolean,
    accent: Color? = null,
    onClick: () -> Unit,
) {
    val colors = DigitDnsTheme.colors
    val grey = LocalIsGreyTheme.current
    val primary = MaterialTheme.colorScheme.primary
    val color = accent ?: primary
    val fullGreySelection = selected && grey && accent == null
    val background = when {
        !enabled && selected -> color.copy(alpha = 0.10f)
        !enabled -> colors.bgTile.copy(alpha = 0.55f)
        fullGreySelection -> primary
        selected -> color.copy(alpha = 0.14f)
        else -> colors.bgTile
    }
    val foreground = when {
        !enabled && selected -> color.copy(alpha = 0.62f)
        !enabled -> MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.78f)
        fullGreySelection -> MaterialTheme.colorScheme.onPrimary
        selected -> color
        else -> MaterialTheme.colorScheme.onSurfaceVariant
    }
    val borderColor = when {
        !enabled -> colors.borderColor.copy(alpha = 0.65f)
        !selected -> colors.borderColor
        fullGreySelection -> primary
        else -> color.copy(alpha = 0.5f)
    }
    Box(
        modifier = Modifier
            .clip(RoundedCornerShape(7.dp))
            .background(background)
            .border(1.dp, borderColor, RoundedCornerShape(7.dp))
            .clickable(enabled = enabled, onClick = onClick)
            .alpha(if (enabled) 1f else 0.78f)
            .padding(horizontal = 3.dp, vertical = 2.dp),
    ) {
        Text(
            text = label,
            style = MaterialTheme.typography.labelMedium.copy(
                fontSize = 12.sp,
                lineHeight = 14.sp,
                fontWeight = FontWeight.Bold,
            ),
            color = foreground,
            maxLines = 1,
        )
    }
}

@Composable
private fun ChipBlock(
    label: String,
    dimmed: Boolean = false,
    footer: (@Composable () -> Unit)? = null,
    content: @Composable () -> Unit,
) {
    Column {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(2.dp),
        ) {
            Text(
                text = label,
                style = MaterialTheme.typography.labelSmall.copy(fontSize = 13.sp, lineHeight = 14.sp),
                color = if (dimmed) {
                    MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.68f)
                } else {
                    MaterialTheme.colorScheme.onSurfaceVariant
                },
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.width(76.dp),
            )
            Box(modifier = Modifier.weight(1f)) {
                content()
            }
        }
        footer?.let {
            Box(modifier = Modifier.padding(start = 78.dp, top = 2.dp)) {
                it()
            }
        }
    }
}

@Composable
private fun AdaptiveIntervalChips(
    base: ResolutionSettings,
    strategy: String,
    enabled: Boolean,
    onChange: (ResolutionSettings) -> Unit,
    isRunning: Boolean,
    onRunAdaptive: () -> Unit,
) {
    val colors = DigitDnsTheme.colors
    val adaptive = strategy == "adaptive" && enabled
    val selectedInterval = normalizedAdaptiveIntervalMinutes(base.adaptiveIntervalMinutes)
    var flash by remember { mutableStateOf(false) }
    var wasRunning by remember { mutableStateOf(false) }
    LaunchedEffect(isRunning) {
        if (isRunning) {
            wasRunning = true
        } else if (wasRunning) {
            wasRunning = false
            flash = true
            delay(1500)
            flash = false
        }
    }
    ChipBlock(
        label = stringResource(R.string.adaptive_interval),
        dimmed = !enabled,
        footer = {
            val footerText = when {
                !enabled -> base.defaultUpstream.takeIf { it.isNotBlank() }?.let {
                    stringResource(R.string.upstream_last_used, it)
                }.orEmpty()
                !adaptive -> ""
                base.adaptiveLastRun.isBlank() -> stringResource(R.string.adaptive_last_run_empty)
                else -> stringResource(
                    R.string.adaptive_last_run,
                    formatAdaptiveRun(base.adaptiveLastRun),
                    base.adaptiveCurrent.ifBlank { "—" },
                )
            }
            Text(
                text = footerText,
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = if (enabled) 1f else 0.72f),
                maxLines = 2,
                minLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
        },
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Row(
                modifier = Modifier.weight(1f).horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(4.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                listOf(
                    5 to stringResource(R.string.adaptive_interval_5m),
                    15 to stringResource(R.string.adaptive_interval_15m),
                    60 to stringResource(R.string.adaptive_interval_1h),
                    360 to stringResource(R.string.adaptive_interval_6h),
                    1440 to stringResource(R.string.adaptive_interval_24h),
                ).forEach { (minutes, label) ->
                    ResolverChip(
                        label = label,
                        selected = adaptive && selectedInterval == minutes,
                        enabled = adaptive,
                        onClick = { onChange(base.copy(adaptiveIntervalMinutes = minutes)) },
                    )
                }
            }
            AdaptiveForceButton(
                enabled = adaptive,
                isRunning = isRunning,
                flash = flash,
                onClick = onRunAdaptive,
            )
        }
    }
}

@Composable
private fun AdaptiveForceButton(
    enabled: Boolean,
    isRunning: Boolean,
    flash: Boolean,
    onClick: () -> Unit,
) {
    val colors = DigitDnsTheme.colors
    IconButton(
        onClick = onClick,
        enabled = enabled && !isRunning,
        modifier = Modifier.size(32.dp),
    ) {
        if (isRunning) {
            CircularProgressIndicator(modifier = Modifier.size(14.dp), strokeWidth = 2.dp)
        } else {
            Icon(
                Icons.Filled.Refresh,
                contentDescription = stringResource(R.string.adaptive_check_now),
                modifier = Modifier.size(14.dp),
                tint = if (flash) colors.success else MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
    Spacer(Modifier.width(2.dp))
    Text(
        text = stringResource(R.string.adaptive_check_now),
        style = MaterialTheme.typography.labelMedium.copy(fontSize = 13.sp),
        color = if (enabled) {
            MaterialTheme.colorScheme.onSurfaceVariant
        } else {
            MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.68f)
        },
    )
}

/* One colour per transport, read from the panel theme instead of a file-level map:
   the same seven roles the web renders as .proto-* carry, so a transport means one
   colour on this chip row and on the live-log / history protocol cards alike. A top
   level val cannot read a CompositionLocal, so the lookup lives here in the composable
   that owns the row. */
@Composable
private fun TransportChips(
    base: ResolutionSettings,
    enabled: Boolean,
    onChange: (ResolutionSettings) -> Unit,
) {
    val transports = normalizedTransports(base.transports)
    val colors = DigitDnsTheme.colors
    ChipBlock(label = stringResource(R.string.domains_transports), dimmed = !enabled) {
        Row(
            modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
            horizontalArrangement = Arrangement.spacedBy(4.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            ALL_TRANSPORTS.forEach { protocol ->
                val selected = protocol in transports
                val color = when (protocol.lowercase()) {
                    "plain" -> colors.info
                    "dot" -> colors.success
                    "doh" -> colors.purple
                    "doh3" -> colors.orange
                    "doq" -> colors.warning
                    "dnscrypt" -> colors.fuchsia
                    else -> MaterialTheme.colorScheme.primary
                }
                val label = when (protocol) {
                    "plain" -> stringResource(R.string.transport_plain)
                    "dot" -> stringResource(R.string.transport_dot)
                    "doh" -> stringResource(R.string.transport_doh)
                    "doh3" -> stringResource(R.string.transport_doh3)
                    "doq" -> stringResource(R.string.transport_doq)
                    else -> stringResource(R.string.transport_dnscrypt)
                }
                ResolverChip(
                    label = label,
                    selected = selected,
                    enabled = enabled && !(selected && transports.size == 1),
                    accent = color,
                    onClick = {
                        val next = if (selected) transports - protocol else transports + protocol
                        if (next.isNotEmpty()) onChange(base.copy(transports = normalizedTransports(next)))
                    },
                )
            }
        }
    }
}

@Composable
private fun SingleServerRow(
    base: ResolutionSettings,
    configured: List<String>,
    lastSingleUpstream: String,
    onChange: (ResolutionSettings) -> Unit,
) {
    var expanded by remember { mutableStateOf(false) }
    val pinned = base.defaultUpstream
    val single = pinned.isNotEmpty()
    val choices = if (pinned.isNotEmpty() && pinned !in configured) configured + pinned else configured
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Text(stringResource(R.string.upstream_mode_pool), style = MaterialTheme.typography.labelSmall.copy(fontSize = 13.sp))
        Switch(
            checked = single,
            onCheckedChange = { on ->
                if (on) {
                    val remembered = configured.firstOrNull { it == lastSingleUpstream }
                    val chosen = remembered ?: configured.firstOrNull()
                    if (chosen != null) onChange(base.copy(defaultUpstream = chosen))
                } else {
                    onChange(base.copy(defaultUpstream = ""))
                }
            },
            enabled = configured.isNotEmpty(),
        )
        Text(stringResource(R.string.upstream_mode_single), style = MaterialTheme.typography.labelSmall.copy(fontSize = 13.sp))
        Box(modifier = Modifier.weight(1f)) {
            OutlinedButton(
                onClick = { expanded = true },
                enabled = single,
                modifier = Modifier.fillMaxWidth(),
                contentPadding = PaddingValues(horizontal = 6.dp, vertical = 2.dp),
            ) {
                ProviderMark(pinned, 16.dp)
                Spacer(Modifier.width(4.dp))
                Text(
                    text = pinned.ifBlank { stringResource(R.string.upstream_pool_default) },
                    style = MaterialTheme.typography.labelMedium.copy(fontSize = 13.sp, fontFamily = PanelMono),
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f),
                )
                Icon(Icons.Filled.ArrowDropDown, contentDescription = null, modifier = Modifier.size(16.dp))
            }
            DigitDnsDropdownMenu(
                expanded = expanded,
                onDismissRequest = { expanded = false },
                modifier = Modifier.width(320.dp),
            ) {
                choices.forEach { address ->
                    DropdownMenuItem(
                        text = {
                            Text(
                                address,
                                style = MaterialTheme.typography.bodySmall.copy(fontSize = 13.sp, fontFamily = PanelMono),
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                            )
                        },
                        leadingIcon = { ProviderMark(address, 16.dp) },
                        trailingIcon = {
                            if (address == pinned) {
                                Icon(Icons.Filled.Check, contentDescription = null, modifier = Modifier.size(16.dp))
                            }
                        },
                        onClick = {
                            expanded = false
                            onChange(base.copy(defaultUpstream = address))
                        },
                    )
                }
            }
        }
    }
}

@Composable
private fun ProviderMark(address: String, size: Dp) {
    val drawable = providerDrawable(address)
    if (drawable != null) {
        Image(
            painter = painterResource(drawable),
            contentDescription = null,
            modifier = Modifier.size(size),
        )
    } else {
        Icon(
            Icons.Filled.Public,
            contentDescription = null,
            modifier = Modifier.size(size),
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

private val PROVIDER_MATCHES = listOf(
    listOf("cloudflare", "1.1.1.1", "1.0.0.1", "one.one.one.one") to R.drawable.provider_cloudflare,
    listOf("google", "8.8.8.8", "8.8.4.4") to R.drawable.provider_google,
    listOf("quad9", "9.9.9.9", "149.112.112.112") to R.drawable.provider_quad9,
    listOf("adguard", "94.140.14.14", "94.140.15.15") to R.drawable.provider_adguard,
    listOf("controld", "control d", "freedns", "76.76.2.0", "76.76.10.0") to R.drawable.provider_controld,
    listOf("dns.sb", "dot.sb", "doh.dns.sb", "185.222.222.222", "45.11.45.11") to R.drawable.provider_dnssb,
    listOf("opendns", "208.67.222.222", "208.67.220.220") to R.drawable.provider_opendns,
    listOf("yandex", "77.88.8.8", "77.88.8.1") to R.drawable.provider_yandex,
    listOf("nextdns", "45.90.28.0", "45.90.30.0") to R.drawable.provider_nextdns,
    listOf("cleanbrowsing", "185.228.168.9", "185.228.169.9") to R.drawable.provider_cleanbrowsing,
)

private val PROVIDER_NAMES = listOf(
    listOf("cloudflare", "1.1.1.1", "1.0.0.1", "one.one.one.one") to "Cloudflare",
    listOf("google", "8.8.8.8", "8.8.4.4") to "Google",
    listOf("quad9", "9.9.9.9", "149.112.112.112") to "Quad9",
    listOf("adguard", "94.140.14.14", "94.140.15.15") to "AdGuard",
    listOf("controld", "freedns", "76.76.2.0", "76.76.10.0") to "Control D",
    listOf("dns.sb", "dot.sb", "doh.dns.sb", "185.222.222.222", "45.11.45.11") to "DNS.SB",
    listOf("opendns", "208.67.222.222", "208.67.220.220") to "OpenDNS",
    listOf("yandex", "77.88.8.8", "77.88.8.1") to "Yandex",
    listOf("nextdns", "45.90.28.0", "45.90.30.0") to "NextDNS",
    listOf("cleanbrowsing", "185.228.168.9", "185.228.169.9") to "CleanBrowsing",
)

/** The public provider's display name, or null for a custom resolver. */
internal fun providerName(address: String): String? {
    val a = address.trim().lowercase()
    return PROVIDER_NAMES.firstOrNull { (needles, _) -> needles.any { a.contains(it) } }?.second
}

internal fun providerDrawable(address: String): Int? {
    val a = address.trim().lowercase()
    return PROVIDER_MATCHES.firstOrNull { (needles, _) -> needles.any { a.contains(it) } }?.second
}

/** The server stores RFC3339; the card shows the same localized time as the rest of the app. */
private fun formatAdaptiveRun(raw: String): String {
    val instant = runCatching { Instant.parse(raw) }.getOrNull() ?: return raw
    return DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM, FormatStyle.SHORT)
        .withZone(ZoneId.systemDefault())
        .format(instant)
}

/**
 * Whether the active transport excludes this resolver. The pool applies the same
 * rule to a pinned upstream, so this drives both the picker and the list dimming.
 */
internal fun resolverExcludedByTransport(
    address: String,
    transport: String,
    encrypted: Set<String>,
): Boolean = when (transport.ifEmpty { "any" }) {
    "encrypted" -> address !in encrypted
    "regular" -> address in encrypted
    else -> false
}

/** The three transport choices. Availability is expressed by disabling, not by text. */
@Composable
internal fun transportChoices(): List<Pair<String, String>> = listOf(
    "any" to stringResource(R.string.transport_any),
    "encrypted" to stringResource(R.string.transport_encrypted),
    "regular" to stringResource(R.string.transport_regular),
)

/**
 * Scheme-level fallback classifier. The server's encrypted set is authoritative
 * when present; this exists so a stale snapshot cannot hide a live DoT/DoH/DoQ
 * resolver from the transport picker.
 */
internal fun resolverLooksEncrypted(address: String): Boolean {
    val a = address.trim().lowercase()
    return a.startsWith("tls://") || a.startsWith("https://") || a.startsWith("quic://") ||
        a.startsWith("h3://") || a.startsWith("sdns://")
}

/** The racer card's title row: icon, label and Benchmark action (or its spinner). */
@Composable
private fun RacerHeader(isBenchmarking: Boolean, onBenchmark: () -> Unit) {
    val colors = DigitDnsTheme.colors
    Row(verticalAlignment = Alignment.CenterVertically) {
        DashboardCardTitle(
            text = stringResource(R.string.upstream_resolvers),
            icon = Icons.Filled.Podcasts,
            tint = colors.warning,
            modifier = Modifier.weight(1f),
        )
        // Fixed-height slot so swapping the Benchmark button for its spinner does not
        // change the header row height and make the card jump while a run is in flight.
        Box(
            modifier = Modifier.height(40.dp),
            contentAlignment = Alignment.Center,
        ) {
            if (isBenchmarking) {
                CircularProgressIndicator(
                    modifier = Modifier.size(16.dp),
                    strokeWidth = 2.dp,
                    color = MaterialTheme.colorScheme.primary,
                )
            } else {
                TextButton(onClick = onBenchmark) {
                    Icon(
                        Icons.Filled.PlayArrow,
                        contentDescription = null,
                        modifier = Modifier.size(16.dp),
                    )
                    Spacer(Modifier.width(4.dp))
                    Text(stringResource(R.string.upstream_benchmark))
                }
            }
        }
    }
}

/** The racer card's inline add row: an address field and an Add button. */
@Composable
private fun UpstreamAddRow(
    value: String,
    onValueChange: (String) -> Unit,
    busy: Boolean,
    presetsOpen: Boolean,
    onTogglePresets: () -> Unit,
    onFocus: () -> Unit,
    onSubmit: () -> Unit,
) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        OutlinedTextField(
            value = value,
            onValueChange = onValueChange,
            placeholder = { Text("Search providers or type an address") },
            singleLine = true,
            enabled = !busy,
            modifier = Modifier.weight(1f).onFocusChanged { if (it.isFocused) onFocus() },
            trailingIcon = {
                IconButton(onClick = onTogglePresets) {
                    Icon(
                        Icons.Filled.ArrowDropDown,
                        contentDescription = if (presetsOpen) "Hide DNS providers" else "Browse DNS providers",
                        modifier = Modifier.rotate(if (presetsOpen) 180f else 0f),
                    )
                }
            },
        )
        Spacer(Modifier.width(8.dp))
        IconButton(onClick = onSubmit, enabled = !busy && value.isNotBlank()) {
            Icon(
                Icons.Filled.Add,
                contentDescription = stringResource(R.string.upstream_add),
                tint = MaterialTheme.colorScheme.primary,
            )
        }
    }
}

@Suppress("LongMethod")
@Composable
private fun UpstreamRow(
    stat: UpstreamStat,
    swipeEnabled: Boolean,
    dimmed: Boolean,
    isAdaptiveCurrent: Boolean,
    adaptiveLastRun: String,
    onRemove: () -> Unit,
) {
    val colors = DigitDnsTheme.colors
    val greyTheme = LocalIsGreyTheme.current
    SwipeToDeleteRow(
        onDelete = onRemove,
        confirmTitle = stringResource(R.string.upstream_remove_title),
        confirmText = "${stat.address} will be removed from the resolver pool.",
        confirmLabel = stringResource(R.string.action_remove),
        // Strict mode, and almost the full row: the card sits inside the scrolling
        // dashboard grid, where Material's velocity-based settling could turn a
        // diagonal brush during a scroll into a delete prompt. Only a deliberate,
        // nearly edge-to-edge drag opens the confirmation.
        threshold = 0.92f,
        enabled = swipeEnabled,
        strict = true,
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .heightIn(min = PanelRowDefaults.height)
                .background(
                    when {
                        !isAdaptiveCurrent -> MaterialTheme.colorScheme.surface
                        // Graphite keeps green for health; the current-row wash
                        // uses a neutral highlight so it matches the theme.
                        greyTheme -> MaterialTheme.colorScheme.onSurface.copy(alpha = 0.10f)
                        else -> colors.success.copy(alpha = 0.14f)
                    },
                )
                .alpha(if (dimmed) 0.45f else 1f)
                .padding(vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            // The status dot carries current health, not lifetime history: red while
            // the circuit is open, amber on a failure streak or a high recent error
            // rate, grey until something has been timed. The latency number beside it
            // keeps its own colour, so a slow-but-working resolver stays green.
            Box(
                modifier = Modifier
                    .size(8.dp)
                    .clip(CircleShape)
                    .background(racerDotColor(stat, colors)),
            )
            Spacer(Modifier.width(6.dp))
            ProviderMark(stat.address, 14.dp)
            Spacer(Modifier.width(6.dp))
            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text = stat.address,
                    style = MaterialTheme.typography.bodyMedium.copy(fontFamily = PanelMono),
                    color = MaterialTheme.colorScheme.onSurface,
                    maxLines = 1,
                )
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        text = pluralStringResource(R.plurals.upstream_hit_count, stat.hits.toInt(), stat.hits) +
                            " · " +
                            pluralStringResource(R.plurals.upstream_error_count, stat.errors.toInt(), stat.errors),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    if (isAdaptiveCurrent) {
                        Spacer(Modifier.width(6.dp))
                        Text(
                            text = "FASTEST",
                            style = MaterialTheme.typography.labelSmall.copy(
                                fontSize = 8.sp,
                                fontWeight = FontWeight.Bold,
                            ),
                            color = colors.success,
                            modifier = Modifier
                                .clip(RoundedCornerShape(999.dp))
                                .background(colors.success.copy(alpha = 0.12f))
                                .padding(horizontal = 5.dp, vertical = 1.dp),
                            maxLines = 1,
                        )
                        Spacer(Modifier.width(4.dp))
                        Text(
                            text = relayRelativeAge(adaptiveLastRun).let {
                                if (it == "never") "—" else it
                            },
                            style = MaterialTheme.typography.labelSmall.copy(
                                fontSize = 8.sp,
                                fontFamily = PanelMono,
                            ),
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            maxLines = 1,
                        )
                    }
                }
            }
            Text(
                text = if (stat.isMeasured) "%.1f ms".format(stat.displayLatencyMs) else "—",
                style = MaterialTheme.typography.bodyMedium.copy(fontFamily = PanelMono),
                color = racerLatencyColor(stat, colors),
            )
        }
    }
}

/** The racer status dot: current health, not the latency number beside it. */
private fun racerDotColor(stat: UpstreamStat, colors: ExtendedColors): Color =
    when {
        stat.circuitOpen -> colors.danger
        !stat.isHealthy -> colors.warning
        !stat.isMeasured -> colors.textMuted
        else -> colors.success
    }

/** The latency number's own colour, by the panel's measured thresholds. */
private fun racerLatencyColor(stat: UpstreamStat, colors: ExtendedColors): Color =
    when {
        !stat.isMeasured -> colors.textMuted
        stat.displayLatencyMs > RACER_DANGER_MS -> colors.danger
        stat.displayLatencyMs > RACER_WARNING_MS -> colors.warning
        else -> colors.success
    }

/** Racer latency: at/above this measured latency reads amber, matching the panel. */
private const val RACER_WARNING_MS = 40.0

/** Racer latency: at/above this measured latency reads red, matching the panel. */
private const val RACER_DANGER_MS = 80.0
