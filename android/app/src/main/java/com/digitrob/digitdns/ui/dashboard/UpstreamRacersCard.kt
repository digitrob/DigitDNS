package com.digitrob.digitdns.ui.dashboard

import androidx.compose.foundation.background
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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Podcasts
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import com.digitrob.digitdns.core.network.ResolutionSettings
import com.digitrob.digitdns.core.network.UpstreamStat
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelRowDefaults
import com.digitrob.digitdns.ui.components.SwipeToDeleteRow
import com.digitrob.digitdns.ui.formatCount
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.ExtendedColors
import com.digitrob.digitdns.ui.theme.PanelMono
import kotlinx.coroutines.launch

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
    onAdd: suspend (String) -> String?,
    onRemove: suspend (String) -> String?,
    /** False while the dashboard grid is scrolling: reading the page must not arm a delete. */
    swipeEnabled: Boolean = true,
    resolutionControls: ResolutionControls = ResolutionControls(),
) {
    val colors = DigitDnsTheme.colors
    var newAddress by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            RacerHeader(isBenchmarking = isBenchmarking, onBenchmark = onBenchmark)

            // Inline add, mirroring the panel's racer input.
            UpstreamAddRow(
                value = newAddress,
                onValueChange = { newAddress = it; error = null },
                busy = busy,
                onSubmit = {
                    val addr = newAddress.trim()
                    if (addr.isNotBlank()) {
                        scope.launch {
                            busy = true
                            error = onAdd(addr)
                            busy = false
                            if (error == null) newAddress = ""
                        }
                    }
                },
            )
            error?.let {
                Text(
                    text = it,
                    style = MaterialTheme.typography.bodySmall,
                    color = colors.danger,
                )
            }

            val policy = poolPolicy(resolutionControls.settings, upstreams)

            if (upstreams.isEmpty()) {
                Text(
                    text = "No upstreams reported yet.",
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
                        upstreams.forEach { stat ->
                            UpstreamRow(
                                stat = stat,
                                swipeEnabled = swipeEnabled,
                                dimmed = resolverExcludedByTransport(stat.address, policy.transport, policy.encrypted) ||
                                    (policy.pinActive && stat.address != policy.pinned),
                                onRemove = {
                                    scope.launch {
                                        busy = true
                                        error = onRemove(stat.address)
                                        busy = false
                                    }
                                },
                            )
                        }
                    }
                }
            }

            // Global resolution defaults. Per-domain overrides live in Domains.
            ResolutionDefaults(
                resolution = resolutionControls.settings,
                configured = upstreams.map { it.address },
                onChange = resolutionControls.onChange,
            )
        }
    }
}

/** The effective pool policy used for dimming one resolver list. */
private data class PoolPolicy(
    val transport: String,
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
    return PoolPolicy(
        transport = transport,
        encrypted = encrypted,
        pinned = pinned,
        pinActive = pinned.isNotEmpty() &&
            !resolverExcludedByTransport(pinned, transport, encrypted),
    )
}

/** The pair the resolver card needs to render and save the global resolution defaults. */
data class ResolutionControls(
    val settings: ResolutionSettings? = null,
    val onChange: (ResolutionSettings) -> Unit = {},
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
) {
    val base = resolution ?: ResolutionSettings()
    val encrypted = base.encryptedUpstreams.toSet()
        .ifEmpty { configured.filter { resolverLooksEncrypted(it) }.toSet() }
    val hasEncrypted = base.encryptedAvailable || encrypted.isNotEmpty()
    val hasRegular = configured.any { !resolverLooksEncrypted(it) }
    val excluded = configured.filter {
        resolverExcludedByTransport(it, base.transport, encrypted)
    }.toSet()
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        ResolutionPicker(
            label = "Transports",
            value = base.transport.ifEmpty { "any" },
            choices = transportChoices(),
            unavailable = buildSet {
                if (!hasEncrypted) add("encrypted")
                if (configured.isNotEmpty() && !hasRegular) add("regular")
            },
            onSelect = { onChange(base.copy(transport = it)) },
        )
        ResolutionPicker(
            label = "Default upstream",
            value = base.defaultUpstream,
            // A pin is a preference, not a policy override: the pool ignores one
            // the transport excludes, so the UI must still let it be chosen.
            choices = listOf("" to "Pool Default") + configured.map { it to it },
            onSelect = { onChange(base.copy(defaultUpstream = it)) },
        )
        if (base.defaultUpstream.isNotEmpty() && base.defaultUpstream in excluded) {
            Text(
                text = "Default upstream ${base.defaultUpstream} is excluded by the active " +
                    "transport and is ignored.",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.tertiary,
            )
        }
        ResolutionPicker(
            label = "Strategy",
            value = base.defaultStrategy,
            choices = listOf(
                "" to "Pool default",
                "fastest" to "Fastest Wins",
                "sequential" to "In Order",
            ),
            onSelect = { onChange(base.copy(defaultStrategy = it)) },
        )
    }
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
internal fun transportChoices(): List<Pair<String, String>> = listOf(
    "any" to "Any Transport",
    "encrypted" to "Encrypted Only (DoT · DoH · DoQ)",
    "regular" to "Regular DNS (UDP · TCP)",
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

@Composable
private fun ResolutionPicker(
    label: String,
    value: String,
    choices: List<Pair<String, String>>,
    onSelect: (String) -> Unit,
    unavailable: Set<String> = emptySet(),
) {
    var expanded by remember { mutableStateOf(false) }
    val selected = choices.firstOrNull { it.first == value }?.second ?: choices.first().second
    Row(verticalAlignment = Alignment.CenterVertically) {
        Text(
            text = label,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.weight(1f),
        )
        OutlinedButton(onClick = { expanded = true }, contentPadding = PaddingValues(horizontal = 8.dp, vertical = 2.dp)) {
            Text(selected, style = MaterialTheme.typography.labelMedium, maxLines = 1)
            Icon(Icons.Filled.ArrowDropDown, contentDescription = null, modifier = Modifier.size(16.dp))
        }
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            choices.forEach { (key, text) ->
                DropdownMenuItem(
                    text = { Text(text, style = MaterialTheme.typography.bodySmall) },
                    enabled = key !in unavailable,
                    trailingIcon = {
                        if (key == value) Icon(Icons.Filled.Check, contentDescription = null, modifier = Modifier.size(16.dp))
                    },
                    onClick = { expanded = false; onSelect(key) },
                )
            }
        }
    }
}

/** The racer card's title row: icon, label and Benchmark action (or its spinner). */
@Composable
private fun RacerHeader(isBenchmarking: Boolean, onBenchmark: () -> Unit) {
    val colors = DigitDnsTheme.colors
    Row(verticalAlignment = Alignment.CenterVertically) {
        Icon(
            Icons.Filled.Podcasts,
            contentDescription = null,
            tint = colors.warning,
            modifier = Modifier.size(18.dp),
        )
        Spacer(Modifier.width(6.dp))
        Label("Upstream Resolvers", modifier = Modifier.weight(1f))
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
                    Text("Benchmark")
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
    onSubmit: () -> Unit,
) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        OutlinedTextField(
            value = value,
            onValueChange = onValueChange,
            placeholder = { Text("8.8.8.8, tls://dns.google") },
            singleLine = true,
            enabled = !busy,
            modifier = Modifier.weight(1f),
        )
        Spacer(Modifier.width(8.dp))
        IconButton(onClick = onSubmit, enabled = !busy && value.isNotBlank()) {
            Icon(
                Icons.Filled.Add,
                contentDescription = "Add upstream",
                tint = MaterialTheme.colorScheme.primary,
            )
        }
    }
}

@Composable
private fun UpstreamRow(
    stat: UpstreamStat,
    swipeEnabled: Boolean,
    dimmed: Boolean,
    onRemove: () -> Unit,
) {
    val colors = DigitDnsTheme.colors
    SwipeToDeleteRow(
        onDelete = onRemove,
        confirmTitle = "Remove upstream?",
        confirmText = "${stat.address} will be removed from the resolver pool.",
        confirmLabel = "Remove",
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
                .background(MaterialTheme.colorScheme.surface)
                .alpha(if (dimmed) 0.45f else 1f)
                .padding(vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            // The status dot mirrors the panel: green when measured and fast, amber above
            // 40 ms, red above 80 ms, grey when nothing has timed it yet.
            Box(
                modifier = Modifier
                    .size(8.dp)
                    .clip(CircleShape)
                    .background(racerDotColor(stat, colors)),
            )
            Spacer(Modifier.width(8.dp))
            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text = stat.address,
                    style = MaterialTheme.typography.bodyMedium.copy(fontFamily = PanelMono),
                    color = MaterialTheme.colorScheme.onSurface,
                    maxLines = 1,
                )
                Text(
                    text = "${formatCount(stat.hits)} hits · ${formatCount(stat.errors)} errors",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Text(
                text = if (stat.isMeasured) "%.1f ms".format(stat.displayLatencyMs) else "—",
                style = MaterialTheme.typography.bodyMedium.copy(fontFamily = PanelMono),
                color = racerDotColor(stat, colors),
            )
        }
    }
}

/** The racer status dot, by the panel's measured-latency thresholds. */
private fun racerDotColor(stat: UpstreamStat, colors: ExtendedColors): Color =
    when {
        !stat.isMeasured -> colors.textMuted
        stat.displayLatencyMs > RACER_DANGER_MS -> colors.danger
        stat.displayLatencyMs > RACER_WARNING_MS -> colors.warning
        else -> colors.success
    }

/** Racer dot: at/above this measured latency reads amber, matching the panel. */
private const val RACER_WARNING_MS = 40.0

/** Racer dot: at/above this measured latency reads red, matching the panel. */
private const val RACER_DANGER_MS = 80.0
