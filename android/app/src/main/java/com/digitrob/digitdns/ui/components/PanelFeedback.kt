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
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SnackbarData
import androidx.compose.material3.SnackbarDuration
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
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

/** How much of the screen the wide dialog claims. */
private const val WIDE_DIALOG_WIDTH = 0.96f

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
 * A roomy relative of [PanelDialog], for forms that need the whole screen.
 *
 * M3's dialog lives in a platform window that pins it to a narrow column and pads its
 * content by 24dp, so a form with a row of chips either loses the chips off the side
 * or has to be squeezed into a scrolling box. This one owns its window instead: it
 * takes 96% of the width, pads by 16dp and grows to whatever height the content
 * needs. The whole card scrolls as one piece, so the buttons travel with the form
 * when the keyboard is up and nothing is left behind — pass [scrollable] = false for
 * a host that arranges its own fixed-height body with its own scrolling region, so
 * the card scroll and the body scroll cannot compete (nested vertical scrolls are a
 * runtime constraint the panel learned the hard way).
 */
@Composable
@Suppress("LongParameterList")
fun PanelWideDialog(
    onDismissRequest: () -> Unit,
    confirmButton: @Composable () -> Unit,
    modifier: Modifier = Modifier,
    dismissButton: @Composable (() -> Unit)? = null,
    title: @Composable (() -> Unit)? = null,
    text: @Composable (() -> Unit)? = null,
    solid: Boolean = true,
    scrollable: Boolean = true,
) {
    val colors = DigitDnsTheme.colors
    val container = if (solid) {
        if (isSystemInDarkTheme()) Color(0xFF0F172A) else Color(0xFFFFFFFF)
    } else {
        colors.bgCard
    }
    Dialog(
        onDismissRequest = onDismissRequest,
        properties = DialogProperties(usePlatformDefaultWidth = false),
    ) {
        Surface(
            modifier = modifier.fillMaxWidth(WIDE_DIALOG_WIDTH),
            shape = RoundedCornerShape(14.dp),
            color = container,
            contentColor = MaterialTheme.colorScheme.onSurfaceVariant,
        ) {
            Column(
                modifier = Modifier
                    .padding(12.dp)
                    .then(if (scrollable) Modifier.verticalScroll(rememberScrollState()) else Modifier),
            ) {
                title?.let {
                    Box(modifier = Modifier.padding(bottom = 12.dp)) { it() }
                }
                if (scrollable) {
                    text?.invoke()
                } else {
                    // A fixed-height host owns its scroll region inside `text`, so the
                    // body takes whatever height the card has left over and the footer
                    // stays on the visible bottom edge instead of flowing off it.
                    Box(modifier = Modifier.weight(1f)) { text?.invoke() }
                }
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 12.dp),
                    horizontalArrangement = Arrangement.End,
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    dismissButton?.let {
                        it()
                        Spacer(Modifier.width(8.dp))
                    }
                    confirmButton()
                }
            }
        }
    }
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
        }
    }
}

/**
 * Show a transient message with the app's own lifetime: it lingers 3 s and then goes.
 *
 * M3 has no 3 s duration constant, so the snackbar is shown indefinitely and a sibling
 * timer dismisses it. The message carries no action: a destructive change asks before
 * it happens, so there is nothing left to reverse once it has.
 */
suspend fun SnackbarHostState.showPanelToast(
    message: String,
    withDismissAction: Boolean = false,
) {
    coroutineScope {
        val snackbar = async {
            showSnackbar(
                message = message,
                withDismissAction = withDismissAction,
                duration = SnackbarDuration.Indefinite,
            )
        }
        val timer = launch {
            delay(TOAST_DURATION_MS)
            currentSnackbarData?.dismiss()
        }
        snackbar.await()
        timer.cancel()
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
