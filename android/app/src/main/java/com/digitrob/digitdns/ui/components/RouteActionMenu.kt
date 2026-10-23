package com.digitrob.digitdns.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.positionInRoot
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.DpOffset
import com.digitrob.digitdns.ui.rules.MatchScope
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

/**
 * The routing quick-action menu, shared by the Domains list and the Live Query Log so
 * both offer the panel's identical menu: a match-scope selector (exact / *.host /
 * *.parent) followed by the four pictogram actions Proxy / Block / Direct / Remove.
 *
 * [onAction] receives the resolved scope string the caller should write, so the scope
 * is decided here and the caller only has to route the result.
 */
@Composable
@Suppress("LongMethod", "CyclomaticComplexMethod")
fun RouteActionMenu(
    domain: String,
    expanded: Boolean,
    onDismiss: () -> Unit,
    onAction: (RouteAction, String) -> Unit,
    anchorSize: IntSize = IntSize.Zero,
    anchorY: Float = 0f,
) {
    var scope by remember(domain) { mutableStateOf(MatchScope.Exact) }
    val colors = DigitDnsTheme.colors
    val density = LocalDensity.current
    val config = LocalConfiguration.current
    val screenHeightPx = with(density) { config.screenHeightDp.dp.toPx() }
    val menuWidthPx = with(density) { 280.dp.toPx() }
    val offsetX = if (anchorSize.width > 0) {
        ((anchorSize.width - menuWidthPx) / 2f).toInt()
    } else 0
    var menuHeightPx by remember { mutableStateOf(with(density) { 200.dp.toPx() }) }
    val anchorCenterY = anchorY + anchorSize.height / 2f
    val overlapPx = with(density) { 12.dp.toPx() }.toInt()
    val offsetY = if (anchorSize.height > 0) {
        if (anchorCenterY < screenHeightPx / 2f) {
            // upper half: menu overlaps the bottom ~12dp of the box, extends down
            -overlapPx
        } else {
            // lower half: menu overlaps the top ~12dp of the box, extends up
            -anchorSize.height - menuHeightPx.toInt() + overlapPx
        }
    } else {
        with(density) { 4.dp.toPx() }.toInt()
    }
    DropdownMenu(
        expanded = expanded,
        onDismissRequest = onDismiss,
        offset = DpOffset(with(density) { offsetX.toDp() }, with(density) { offsetY.toDp() }),
        modifier = Modifier
            .background(colors.bgCard)
            .border(1.dp, colors.borderColor, RoundedCornerShape(12.dp))
            .width(280.dp)
            .onSizeChanged { menuHeightPx = it.height.toFloat() },
    ) {
        Text(
            text = domain,
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.padding(horizontal = 12.dp, vertical = 4.dp),
        )
        ScopeSelector(domain, scope, onSelect = { scope = it })
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 2.dp),
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            val moveActions = listOf(RouteAction.Proxy, RouteAction.Block, RouteAction.Direct)
            moveActions.forEach { action ->
                val target = scope.resolve(domain)
                val enabled = target != null
                OutlinedButton(
                    onClick = {
                        val resolved = target ?: return@OutlinedButton
                        onDismiss()
                        onAction(action, resolved)
                    },
                    enabled = enabled,
                    modifier = Modifier.weight(1f),
                    contentPadding = PaddingValues(horizontal = 2.dp, vertical = 0.dp),
                ) {
                    Icon(
                        action.icon,
                        contentDescription = null,
                        tint = if (enabled) action.color() else MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.size(14.dp),
                    )
                    Spacer(Modifier.width(2.dp))
                    Text(action.label, maxLines = 1, overflow = TextOverflow.Ellipsis, style = MaterialTheme.typography.labelSmall)
                }
            }
        }
        val removeAction = RouteAction.Remove
        val removeTarget = scope.resolve(domain)
        val removeEnabled = removeTarget != null
        Button(
            onClick = {
                val resolved = removeTarget ?: return@Button
                onDismiss()
                onAction(removeAction, resolved)
            },
            enabled = removeEnabled,
            modifier = Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 2.dp),
            colors = ButtonDefaults.buttonColors(
                containerColor = if (removeEnabled) colors.danger else MaterialTheme.colorScheme.surfaceVariant,
                contentColor = MaterialTheme.colorScheme.onPrimary,
            ),
        ) {
            Icon(removeAction.icon, contentDescription = null, modifier = Modifier.size(16.dp))
            Spacer(Modifier.width(6.dp))
            Text(removeAction.label)
        }
    }
}

/** The scope radiogroup row inside the menu: exact / *.host / *.parent. */
@Composable
private fun ScopeSelector(domain: String, selected: MatchScope, onSelect: (MatchScope) -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 8.dp, vertical = 4.dp),
        horizontalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        MatchScope.entries.forEach { scope ->
            val valid = scope.resolve(domain) != null
            val active = scope == selected && valid
            Column(
                modifier = Modifier
                    .weight(1f)
                    .clip(RoundedCornerShape(6.dp))
                    .background(
                        if (active) {
                            MaterialTheme.colorScheme.primary.copy(alpha = 0.15f)
                        } else {
                            Color.Transparent
                        },
                    )
                    .clickable(enabled = valid) { if (valid) onSelect(scope) }
                    .padding(horizontal = 6.dp, vertical = 8.dp),
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                Text(
                    text = scope.label,
                    style = MaterialTheme.typography.labelMedium,
                    textAlign = TextAlign.Center,
                    color = if (!valid) {
                        MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.4f)
                    } else if (active) {
                        MaterialTheme.colorScheme.primary
                    } else {
                        MaterialTheme.colorScheme.onSurfaceVariant
                    },
                )
                Text(
                    text = scope.hint,
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = if (valid) 1f else 0.4f),
                    maxLines = 2,
                    textAlign = TextAlign.Center,
                )
            }
        }
    }
}
