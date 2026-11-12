package com.digitrob.digitdns.ui.subscriber

import androidx.annotation.ArrayRes
import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.MenuBook
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringArrayResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.digitrob.digitdns.R
import com.digitrob.digitdns.core.network.SubscriberStatusDto
import com.digitrob.digitdns.ui.components.PanelCopyIconButton
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono

/** One platform's guide, stored as resources so the steps can be translated. */
private data class GuidePlatform(
    @StringRes val chipRes: Int,
    @StringRes val titleRes: Int,
    @ArrayRes val stepsRes: Int,
    @StringRes val noteRes: Int? = null,
    val privateDns: Boolean = false,
)

private val playStationGuide = GuidePlatform(
    chipRes = R.string.guide_chip_playstation,
    titleRes = R.string.guide_title_playstation,
    stepsRes = R.array.guide_playstation_steps,
)

private val xboxGuide = GuidePlatform(
    chipRes = R.string.guide_chip_xbox,
    titleRes = R.string.guide_title_xbox,
    stepsRes = R.array.guide_xbox_steps,
)

private val switchGuide = GuidePlatform(
    chipRes = R.string.guide_chip_switch,
    titleRes = R.string.guide_title_switch,
    stepsRes = R.array.guide_switch_steps,
)

private val windowsGuide = GuidePlatform(
    chipRes = R.string.guide_chip_windows,
    titleRes = R.string.guide_title_windows,
    stepsRes = R.array.guide_windows_steps,
)

private val appleGuide = GuidePlatform(
    chipRes = R.string.guide_chip_apple,
    titleRes = R.string.guide_title_apple,
    stepsRes = R.array.guide_apple_steps,
)

private val routerGuide = GuidePlatform(
    chipRes = R.string.guide_chip_router,
    titleRes = R.string.guide_title_router,
    stepsRes = R.array.guide_router_steps,
)

/** Android differs by install: Private DNS needs a hostname, static Wi-Fi does not. */
private fun androidGuide(status: SubscriberStatusDto) = GuidePlatform(
    chipRes = R.string.guide_chip_android,
    titleRes = R.string.guide_title_android,
    stepsRes = if (status.hasDomain) R.array.guide_android_steps else R.array.guide_android_nodomain_steps,
    noteRes = if (status.hasDomain) null else R.string.guide_note_android_no_domain,
    privateDns = true,
)

private fun platforms(status: SubscriberStatusDto): List<GuidePlatform> = listOf(
    playStationGuide,
    xboxGuide,
    switchGuide,
    windowsGuide,
    appleGuide,
    androidGuide(status),
    routerGuide,
)

/**
 * The full setup guide, native. The portal used to be the only place with the
 * per-platform steps, which meant leaving the app for a browser page; every panel
 * and every step now lives here, with the live values from this subscription.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
internal fun GuideTab(status: SubscriberStatusDto, onCopy: (String) -> Unit) {
    val entries = platforms(status)
    var selected by rememberSaveable { mutableIntStateOf(0) }
    val platform = entries[selected.coerceIn(entries.indices)]

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(bottom = 12.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        FlowRow(
            horizontalArrangement = Arrangement.spacedBy(6.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            entries.forEachIndexed { index, entry ->
                FilterChip(
                    selected = index == selected,
                    onClick = { selected = index },
                    label = {
                        Text(stringResource(entry.chipRes), style = MaterialTheme.typography.labelSmall)
                    },
                )
            }
        }

        SubscriberCard(verticalPadding = 10.dp, title = stringResource(R.string.guide_card_guide), icon = Icons.Filled.MenuBook) {
            Text(
                stringResource(platform.titleRes),
                style = MaterialTheme.typography.titleSmall.copy(fontWeight = FontWeight.SemiBold),
            )
            val steps = stringArrayResource(platform.stepsRes)
            steps.forEachIndexed { index, step ->
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    Text(
                        "${index + 1}.",
                        style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Text(
                        step,
                        style = MaterialTheme.typography.bodySmall,
                        modifier = Modifier.weight(1f),
                    )
                }
            }
            platform.noteRes?.let { noteRes ->
                Text(
                    stringResource(noteRes),
                    style = MaterialTheme.typography.bodySmall,
                    color = DigitDnsTheme.colors.warning,
                )
            }
        }

        SubscriberCard(verticalPadding = 10.dp, title = stringResource(R.string.guide_card_values), icon = Icons.Filled.Dns) {
            GuideValueRow(stringResource(R.string.subscriber_primary_dns), status.serverDns, onCopy)
            GuideValueRow(stringResource(R.string.subscriber_secondary_dns), "1.1.1.1", onCopy)
            if (platform.privateDns && status.hasDomain) {
                GuideValueRow(stringResource(R.string.guide_private_dns_hostname), status.serverHost, onCopy)
            }
            if (status.hasDomain) {
                GuideValueRow(stringResource(R.string.subscriber_doh), "https://${status.serverHost}:${status.dohPort}/dns-query", onCopy)
            }
            if (status.doh3Enabled && status.hasDomain) {
                GuideValueRow(stringResource(R.string.subscriber_doh3), "https://${status.serverHost}:${status.dohPort}/dns-query", onCopy)
            }
            if (status.doqEnabled && status.doqUrl.isNotBlank()) {
                GuideValueRow(stringResource(R.string.subscriber_doq), status.doqUrl, onCopy)
            }
            GuideValueRow(stringResource(R.string.subscriber_dot), "${status.serverHost}:${status.dotPort}", onCopy)
        }
    }
}

@Composable
private fun GuideValueRow(label: String, value: String, onCopy: (String) -> Unit) {
    Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Column(modifier = Modifier.weight(1f)) {
            Text(
                label,
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                value,
                style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
        }
        PanelCopyIconButton(
            contentDescription = stringResource(R.string.subscriber_copy_value, label),
            onCopy = { onCopy(value) },
            modifier = Modifier.size(32.dp),
            iconSize = 16.dp,
        )
    }
}
