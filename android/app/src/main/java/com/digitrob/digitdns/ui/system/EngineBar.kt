package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.ui.components.PanelToggle
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono


/**
 * The global engine switch. It is deliberately outside the tab content: turning
 * the policy engine off must be reachable from every screen, not buried in
 * Settings. Off means queries resolve directly, with no rules or proxying.
 */
@Composable
fun EngineBar(
    modifier: Modifier = Modifier,
    compact: Boolean = false,
    viewModel: EngineViewModel = hiltViewModel(),
) {
    val enabled by viewModel.enabled.collectAsStateWithLifecycle()
    val known = enabled != null
    val on = enabled != false
    val tone = when {
        !known -> MaterialTheme.colorScheme.onSurfaceVariant
        on -> DigitDnsTheme.colors.success
        else -> DigitDnsTheme.colors.danger
    }
    if (compact) {
        Column(
            modifier = modifier
                .padding(vertical = 8.dp)
                .clip(RoundedCornerShape(10.dp))
                .clickable(role = Role.Switch, enabled = known) { viewModel.setEnabled(!on) }
                .padding(horizontal = 8.dp, vertical = 6.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(3.dp),
        ) {
            Box(Modifier.size(9.dp).clip(CircleShape).background(tone))
            Text(
                text = when {
                    !known -> "…"
                    on -> "ON"
                    else -> "OFF"
                },
                style = MaterialTheme.typography.labelSmall.copy(fontFamily = PanelMono, fontWeight = FontWeight.Bold),
                color = tone,
            )
        }
        return
    }
    Row(
        modifier = modifier
            .fillMaxWidth()
            .background(MaterialTheme.colorScheme.surface)
            .padding(horizontal = 14.dp, vertical = 5.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(Modifier.size(9.dp).clip(CircleShape).background(tone))
        Spacer(Modifier.width(8.dp))
        Text(
            text = when {
                !known -> "Engine…"
                on -> "Engine ON"
                else -> "Engine OFF"
            },
            style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.Bold),
            color = tone,
        )
        if (known && !on) {
            Spacer(Modifier.width(8.dp))
            Text(
                text = "Direct only",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        Spacer(Modifier.weight(1f))
        PanelToggle(
            checked = on,
            enabled = known,
            onCheckedChange = viewModel::setEnabled,
            colors = SwitchDefaults.colors(
                checkedThumbColor = DigitDnsTheme.colors.knob,
                checkedTrackColor = DigitDnsTheme.colors.success,
                checkedBorderColor = DigitDnsTheme.colors.success,
                uncheckedThumbColor = DigitDnsTheme.colors.knob,
                uncheckedTrackColor = DigitDnsTheme.colors.danger,
                uncheckedBorderColor = DigitDnsTheme.colors.danger,
            ),
        )
    }
}
