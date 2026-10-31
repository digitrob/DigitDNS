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

/** The last scope picked per name, so reopening the menu keeps it even before a write. */
private object RouteScopeMemory {
    private val scopes = mutableMapOf<String, MatchScope>()

    fun get(domain: String): MatchScope? = scopes[domain]

    fun put(domain: String, scope: MatchScope) {
        scopes[domain] = scope
    }
}

/**
 * The routing quick-action menu, shared by the Domains list and the Live Query Log so
 * both offer the panel's identical menu: a match-scope selector (exact / *.host /
 * *.parent) followed by the routing actions Proxy / Block / Direct. [allowRemove]
 * is true only on the Domains list; deleting is a managed-list decision, so the
 * live log does not carry a Remove button.
 *
 * [onAction] receives the resolved scope string the caller should write, so the scope
 * is decided here and the caller only has to route the result.
 */
@Composable
@Suppress("LongMethod", "CyclomaticComplexMethod", "LongParameterList")
fun RouteActionMenu(
    domain: String,
    expanded: Boolean,
    onDismiss: () -> Unit,
    onAction: (RouteAction, List<String>) -> Unit,
    onScopeChange: (MatchScope, List<String>) -> Unit = { _, _ -> },
    allowRemove: Boolean = true,
    patterns: Set<String> = emptySet(),
    anchorSize: IntSize = IntSize.Zero,
    anchorY: Float = 0f,
) {
    // Keep the last scope picked for this name, then fall back to the one its
    // rules imply. Reopening after choosing *.host must not snap back to exact.
    var scope by remember(domain, patterns) {
        mutableStateOf(RouteScopeMemory.get(domain) ?: MatchScope.initialScope(domain, patterns))
    }
    // What a scope click wrote in this menu session. The next click removes it, so
    // *.parent -> exact restores the origin even though the origin left the lists.
    var appliedTargets by remember(domain, expanded) { mutableStateOf(emptyList<String>()) }
    val colors = DigitDnsTheme.colors
    val density = LocalDensity.current
    val config = LocalConfiguration.current
    val screenHeightPx = with(density) { config.screenHeightDp.dp.toPx() }
    // The menu carries four scopes plus two lines of text now, so it takes the
    // window width minus 16 dp margins, capped at 340 dp. The previous fixed 280 dp
    // left each scope ~63 dp and wrapped the hints into a cramped column.
    val menuWidth = (config.screenWidthDp - 32).coerceIn(280, 340).dp
    val menuWidthPx = with(density) { menuWidth.toPx() }
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
            .width(menuWidth)
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
        ScopeSelector(domain, scope, onSelect = { chosen ->
            onScopeChange(chosen, appliedTargets)
            appliedTargets = chosen.targets(domain)
            scope = chosen
            RouteScopeMemory.put(domain, chosen)
        })
        // The scope labels are short on purpose: without the resolved pattern a
        // *.host action on a host looks like it covers that host, while it writes
        // *.host and covers its subdomains only.
        val targets = scope.targets(domain)
        Text(
            text = if (targets.isEmpty()) {
                "No pattern for this scope"
            } else {
                "applies to ${targets.joinToString(" + ")}"
            },
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.padding(horizontal = 12.dp, vertical = 2.dp),
        )
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 2.dp),
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            val moveActions = listOf(RouteAction.Proxy, RouteAction.Block, RouteAction.Direct)
            moveActions.forEach { action ->
                val enabled = targets.isNotEmpty()
                OutlinedButton(
                    onClick = {
                        if (targets.isEmpty()) return@OutlinedButton
                        onDismiss()
                        onAction(action, targets)
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
        if (allowRemove) {
            val removeAction = RouteAction.Remove
            val removeEnabled = targets.isNotEmpty()
            Button(
                onClick = {
                    if (targets.isEmpty()) return@Button
                    onDismiss()
                    onAction(removeAction, targets)
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
}

/** The scope selector: a 2 x 2 grid, so every label and hint fits on one line. */
@Composable
private fun ScopeSelector(domain: String, selected: MatchScope, onSelect: (MatchScope) -> Unit) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 8.dp, vertical = 4.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        MatchScope.entries.chunked(SCOPE_COLUMNS).forEach { row ->
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(4.dp),
            ) {
                row.forEach { scope ->
                    ScopeCell(
                        scope = scope,
                        domain = domain,
                        selected = scope == selected,
                        onSelect = onSelect,
                        modifier = Modifier.weight(1f),
                    )
                }
                repeat(SCOPE_COLUMNS - row.size) { Spacer(Modifier.weight(1f)) }
            }
        }
    }
}

/** One scope in the selector: the pattern label plus a one-line hint. */
@Composable
private fun ScopeCell(
    scope: MatchScope,
    domain: String,
    selected: Boolean,
    onSelect: (MatchScope) -> Unit,
    modifier: Modifier = Modifier,
) {
    val valid = scope.targets(domain).isNotEmpty()
    val active = selected && valid
    Column(
        modifier = modifier
            .clip(RoundedCornerShape(6.dp))
            .background(
                if (active) MaterialTheme.colorScheme.primary.copy(alpha = 0.15f) else Color.Transparent,
            )
            .clickable(enabled = valid) { if (valid) onSelect(scope) }
            .padding(horizontal = 6.dp, vertical = 8.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(
            text = scope.label,
            style = MaterialTheme.typography.labelMedium,
            textAlign = TextAlign.Center,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            color = when {
                !valid -> MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.4f)
                active -> MaterialTheme.colorScheme.primary
                else -> MaterialTheme.colorScheme.onSurfaceVariant
            },
        )
        Text(
            text = scope.hint,
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = if (valid) 1f else 0.4f),
            textAlign = TextAlign.Center,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
    }
}

private const val SCOPE_COLUMNS = 3
