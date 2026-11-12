@file:Suppress("MatchingDeclarationName")

package com.digitrob.digitdns.ui.rules

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.outlined.RadioButtonUnchecked
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
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
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.digitrob.digitdns.R
import com.digitrob.digitdns.ui.components.CountryFlag
import com.digitrob.digitdns.ui.components.DigitDnsDropdownMenu
import com.digitrob.digitdns.ui.components.PanelWideDialog
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono

/** One row in a relay chooser. [id] is what gets written to the rules map. */
internal data class RelayChoice(
    val id: String,
    val label: String,
    val country: String,
    val detail: String = "",
    /** Set on the first row of a group; the dialog draws the heading above it. */
    val group: String? = null,
)

/**
 * The domain-level chooser: inherit the service default, force This Server, or pick
 * an agent. This is a dialog opened from the row's second line, not a column, so the
 * gear / status / switch group keeps its exact position.
 */
@Composable
internal fun RelayPickerDialog(
    domain: String,
    state: RulesUiState,
    onPick: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    val serviceAgent = state.serviceRelayAgent(domain)
    val serviceName = state.lists.serviceOf(domain)
        .takeIf { it.isNotEmpty() }
        ?.let(state::serviceName)
    val inheritDetail = if (serviceAgent != null) {
        listOfNotNull(serviceName, serviceAgent.name.ifBlank { serviceAgent.id })
            .joinToString(" · ")
    } else {
        stringResource(R.string.relay_this_server)
    }
    val options = buildList {
        add(
            RelayChoice(
                id = "__inherit__",
                label = stringResource(R.string.relay_inherit_service),
                country = serviceAgent?.country ?: state.serverCountry,
                detail = inheritDetail,
            ),
        )
        add(
            RelayChoice(
                id = "__local__",
                label = stringResource(R.string.relay_this_server),
                country = state.serverCountry,
            ),
        )
        state.relayAgents().forEachIndexed { index, agent ->
            add(
                RelayChoice(
                    id = agent.id,
                    label = agent.name.ifBlank { agent.id },
                    country = agent.country,
                    detail = agent.country,
                    group = if (index == 0) stringResource(R.string.egress_group_agents) else null,
                ),
            )
        }
        var lastGroup = ""
        state.relayEgresses().forEach { (id, egress) ->
            val group = if (egress.kind == "wireguard") {
                stringResource(R.string.egress_group_wireguard)
            } else {
                stringResource(R.string.egress_group_socks5)
            }
            add(
                RelayChoice(
                    id = id,
                    label = egress.name.ifBlank { id },
                    country = egress.country,
                    detail = egress.country,
                    group = group.takeIf { it != lastGroup },
                ),
            )
            lastGroup = group
        }
    }
    val stored = state.storedRelayId(domain)
    RelayChoiceDialog(
        title = stringResource(R.string.relay_picker_title, domain),
        body = stringResource(R.string.relay_picker_body),
        options = options,
        // No stored override means "inherit", which is the row that must carry the
        // check; an empty string would match none of the options.
        selected = stored.ifEmpty { "__inherit__" },
        onPick = onPick,
        onDismiss = onDismiss,
    )
}

/** The same choice set for a bulk selection; [count] goes in the title only. */
@Composable
internal fun BulkRelayPickerDialog(
    count: Int,
    state: RulesUiState,
    onPick: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    val options = buildList {
        add(
            RelayChoice(
                id = "__inherit__",
                label = stringResource(R.string.relay_inherit_service),
                country = state.serverCountry,
                detail = stringResource(R.string.relay_this_server),
            ),
        )
        add(
            RelayChoice(
                id = "__local__",
                label = stringResource(R.string.relay_this_server),
                country = state.serverCountry,
            ),
        )
        state.relayAgents().forEachIndexed { index, agent ->
            add(
                RelayChoice(
                    id = agent.id,
                    label = agent.name.ifBlank { agent.id },
                    country = agent.country,
                    detail = agent.country,
                    group = if (index == 0) stringResource(R.string.egress_group_agents) else null,
                ),
            )
        }
        var lastGroup = ""
        state.relayEgresses().forEach { (id, egress) ->
            val group = if (egress.kind == "wireguard") {
                stringResource(R.string.egress_group_wireguard)
            } else {
                stringResource(R.string.egress_group_socks5)
            }
            add(
                RelayChoice(
                    id = id,
                    label = egress.name.ifBlank { id },
                    country = egress.country,
                    detail = egress.country,
                    group = group.takeIf { it != lastGroup },
                ),
            )
            lastGroup = group
        }
    }
    RelayChoiceDialog(
        title = stringResource(R.string.relay_bulk_title, count),
        body = stringResource(R.string.relay_bulk_body),
        options = options,
        selected = "",
        onPick = onPick,
        onDismiss = onDismiss,
    )
}

@Composable
internal fun RelayChoiceDialog(
    title: String,
    body: String,
    options: List<RelayChoice>,
    selected: String,
    onPick: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    PanelWideDialog(
        onDismissRequest = onDismiss,
        solid = true,
        title = { Text(title, style = MaterialTheme.typography.titleLarge) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(
                    body,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(bottom = 6.dp),
                )
                options.forEach { option ->
                    option.group?.let { heading ->
                        Text(
                            heading.uppercase(),
                            style = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.Bold),
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier = Modifier.padding(start = 10.dp, top = 6.dp, bottom = 2.dp),
                        )
                    }
                    RelayChoiceRow(
                        option = option,
                        selected = option.id == selected,
                        onClick = { onPick(option.id) },
                    )
                }
            }
        },
        confirmButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}

@Composable
private fun RelayChoiceRow(option: RelayChoice, selected: Boolean, onClick: () -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(10.dp))
            .background(
                if (selected) DigitDnsTheme.colors.success.copy(alpha = 0.10f) else Color.Transparent,
            )
            .clickable(role = Role.Button, onClick = onClick)
            .padding(horizontal = 10.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            if (selected) Icons.Filled.CheckCircle else Icons.Outlined.RadioButtonUnchecked,
            contentDescription = null,
            tint = if (selected) {
                DigitDnsTheme.colors.success
            } else {
                MaterialTheme.colorScheme.onSurfaceVariant
            },
            modifier = Modifier.size(24.dp),
        )
        Spacer(Modifier.width(10.dp))
        CountryFlag(country = option.country, width = 26.dp, height = 19.dp)
        Spacer(Modifier.width(12.dp))
        Column(modifier = Modifier.weight(1f)) {
            Text(
                option.label,
                style = MaterialTheme.typography.bodyLarge.copy(fontSize = 16.sp),
                color = MaterialTheme.colorScheme.onSurface,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            if (option.detail.isNotBlank() && option.detail != option.label) {
                Text(
                    option.detail,
                    style = MaterialTheme.typography.bodySmall.copy(
                        fontFamily = PanelMono,
                        fontSize = 12.sp,
                    ),
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
    }
}

/**
 * The service editor's default-relay field. "This Server" writes no service_agents
 * entry, which is exactly what the panel stores for the default.
 */
@Composable
internal fun ServiceDefaultRelayField(
    state: RulesUiState,
    selected: String,
    onSelect: (String) -> Unit,
) {
    var expanded by remember { mutableStateOf(false) }
    val options = buildList {
        add(RelayChoice("", stringResource(R.string.relay_this_server), state.serverCountry))
        state.relayAgents().forEach { agent ->
            add(RelayChoice(agent.id, agent.name.ifBlank { agent.id }, agent.country, agent.country))
        }
    }
    val current = options.firstOrNull { it.id == selected } ?: options.first()

    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(
            stringResource(R.string.relay_service_default),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Box {
            OutlinedButton(
                onClick = { expanded = true },
                modifier = Modifier.fillMaxWidth(),
            ) {
                CountryFlag(country = current.country, width = 24.dp, height = 18.dp)
                Spacer(Modifier.width(8.dp))
                Text(
                    current.label,
                    modifier = Modifier.weight(1f),
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                Icon(Icons.Filled.ArrowDropDown, contentDescription = null)
            }
            DigitDnsDropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
                Column(
                    modifier = Modifier
                        .heightIn(max = 280.dp)
                        .widthIn(min = 280.dp, max = 360.dp)
                        .verticalScroll(rememberScrollState()),
                ) {
                    options.forEach { option ->
                        DropdownMenuItem(
                            text = {
                                Row(verticalAlignment = Alignment.CenterVertically) {
                                    CountryFlag(country = option.country, width = 24.dp, height = 18.dp)
                                    Spacer(Modifier.width(8.dp))
                                    Text(
                                option.label,
                                style = MaterialTheme.typography.bodyLarge.copy(fontSize = 15.sp),
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                            )
                                }
                            },
                            trailingIcon = {
                                if (option.id == current.id) {
                                    Icon(
                                        Icons.Filled.CheckCircle,
                                        contentDescription = null,
                                        tint = DigitDnsTheme.colors.success,
                                        modifier = Modifier.size(20.dp),
                                    )
                                }
                            },
                            onClick = {
                                expanded = false
                                onSelect(option.id)
                            },
                        )
                    }
                }
            }
        }
        Text(
            stringResource(R.string.relay_service_default_body),
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/** The relay chooser on the domain row's second line: the flag alone is the control. */
@Composable
internal fun RelayRowIcon(
    label: String,
    country: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val relayDescription = stringResource(R.string.relay_row_content, label)
    Box(
        modifier = modifier
            .size(30.dp)
            .clip(RoundedCornerShape(5.dp))
            .clickable(role = Role.Button, onClick = onClick)
            .semantics {
                contentDescription = relayDescription
            },
        contentAlignment = Alignment.Center,
    ) {
        CountryFlag(
            country = country,
            width = 20.dp,
            height = 15.dp,
            // The touchable Box owns the full description; the flag itself is
            // decorative, so a screen reader does not hear the country twice.
            modifier = Modifier.clearAndSetSemantics {},
        )
    }
}

/** Opens the bulk Egress chooser while a selection exists. */
@Composable
internal fun BulkRelayHost(
    open: Boolean,
    domains: Set<String>,
    state: RulesUiState,
    onApply: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    if (!open || domains.isEmpty()) return
    BulkRelayPickerDialog(
        count = domains.size,
        state = state,
        onPick = onApply,
        onDismiss = onDismiss,
    )
}
