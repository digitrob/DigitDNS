package com.digitrob.digitdns.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.border
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
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SnackbarData
import androidx.compose.material3.SnackbarDuration
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/**
 * The app's own dialog and transient-message surfaces, shaped like the panel rather
 * than left as stock Material 3.
 *
 * The panel draws every modal on the same glass card and every toast as a small
 * tinted pill; a stock M3 dialog with its own corner radius and background made the
 * app read as a different product the moment one opened. These wrappers carry the
 * glass treatment so the two surfaces match the rest of the app.
 */

/** How long a transient message lingers. M3's own `Short` is 4 s; the panel uses ~3 s. */
private const val TOAST_DURATION_MS = 3_000L

/** How long an action-bearing message (Undo) lingers — the M3 `Short` 4 s. */
private const val ACTION_DURATION_MS = 4_000L

/**
 * A themed replacement for [AlertDialog]. Same parameters as M3's, so a call site only
 * swaps the import; the difference is the glass surface, the panel's corner radius and
 * the panel text colours.
 *
 * Confirmation and form dialogs never auto-dismiss: a form the operator is still
 * filling cannot have a lifetime, so the only way out is the buttons (or a backdrop /
 * back press, which [onDismissRequest] handles).
 */
@Composable
fun PanelDialog(
    onDismissRequest: () -> Unit,
    confirmButton: @Composable () -> Unit,
    modifier: Modifier = Modifier,
    dismissButton: @Composable (() -> Unit)? = null,
    title: @Composable (() -> Unit)? = null,
    text: @Composable (() -> Unit)? = null,
    solid: Boolean = false,
) {
    val colors = DigitDnsTheme.colors
    val container = if (solid) {
        if (isSystemInDarkTheme()) Color(0xFF0F172A) else Color(0xFFFFFFFF)
    } else {
        colors.bgCard
    }
    AlertDialog(
        onDismissRequest = onDismissRequest,
        confirmButton = confirmButton,
        modifier = modifier,
        dismissButton = dismissButton,
        title = title,
        text = text,
        shape = RoundedCornerShape(14.dp),
        containerColor = container,
        titleContentColor = MaterialTheme.colorScheme.onSurface,
        textContentColor = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

/**
 * The themed host for transient messages. It replaces [SnackbarHost] so a toast drawn
 * on any screen is the same small glass pill the panel uses, toned by the message type
 * rather than the screen it happens to appear on.
 */
@Composable
fun PanelSnackbarHost(hostState: SnackbarHostState) {
    SnackbarHost(hostState = hostState) { data -> PanelToast(data) }
}

@Composable
private fun PanelToast(data: SnackbarData) {
    val colors = DigitDnsTheme.colors
    val tone = toastTone(data.visuals.message)
    Row(
        modifier = Modifier
            .padding(horizontal = 12.dp, vertical = 8.dp)
            .clip(RoundedCornerShape(12.dp))
            .background(colors.bgCard)
            .border(1.dp, tone.copy(alpha = 0.5f), RoundedCornerShape(12.dp))
            .padding(horizontal = 14.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(
            modifier = Modifier
                .size(8.dp)
                .clip(CircleShape)
                .background(tone),
        )
        Spacer(Modifier.width(10.dp))
        Column(modifier = Modifier.weight(1f, fill = false), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(
                text = data.visuals.message,
                style = MaterialTheme.typography.bodySmall.copy(fontWeight = FontWeight.SemiBold),
                color = MaterialTheme.colorScheme.onSurface,
            )
            data.visuals.actionLabel?.let { label ->
                Text(
                    text = label.uppercase(),
                    style = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.Bold),
                    color = MaterialTheme.colorScheme.primary,
                    modifier = Modifier.padding(top = 2.dp),
                )
            }
        }
    }
}

/**
 * Show a transient message with the app's own lifetime. A plain message lingers 3 s;
 * one that carries an action (Undo) gets 4 s, because an undo the operator cannot
 * reach in time is the same as no undo.
 *
 * M3 has no 3 s duration constant, so the snackbar is shown indefinitely and a sibling
 * timer dismisses it. The real [androidx.compose.material3.SnackbarResult] is returned,
 * so a call site that checks for an action keeps working.
 */
suspend fun SnackbarHostState.showPanelToast(
    message: String,
    actionLabel: String? = null,
    withDismissAction: Boolean = false,
): androidx.compose.material3.SnackbarResult {
    val duration = if (actionLabel != null) ACTION_DURATION_MS else TOAST_DURATION_MS
    return coroutineScope {
        val snackbar = async {
            showSnackbar(
                message = message,
                actionLabel = actionLabel,
                withDismissAction = withDismissAction,
                duration = SnackbarDuration.Indefinite,
            )
        }
        val timer = launch {
            delay(duration)
            currentSnackbarData?.dismiss()
        }
        val result = snackbar.await()
        timer.cancel()
        result
    }
}

/** The tint for a message, guessed from its wording — the panel tones its toasts the same way. */
@Composable
private fun toastTone(message: String): Color {
    val colors = DigitDnsTheme.colors
    val lower = message.lowercase()
    return when {
        lower.contains("fail") || lower.contains("error") || lower.contains("could not") ||
            lower.contains("unable") || lower.contains("refused") -> colors.danger
        lower.contains("removed") || lower.contains("saved") || lower.contains("copied") ||
            lower.contains("success") || lower.contains("done") -> colors.success
        else -> colors.info
    }
}
