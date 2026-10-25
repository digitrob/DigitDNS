package com.digitrob.digitdns.ui.components

import androidx.compose.animation.core.animate
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.gestures.awaitHorizontalTouchSlopOrCancellation
import androidx.compose.foundation.gestures.horizontalDrag
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SwipeToDismissBox
import androidx.compose.material3.SwipeToDismissBoxValue
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberSwipeToDismissBoxState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.input.pointer.positionChange
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import kotlinx.coroutines.launch
import kotlin.math.roundToInt

/**
 * How much of a row a swipe has to cross before it counts as a delete.
 *
 * Material's own threshold is a fixed 56 dp — a fifth of a phone row, so a lazy brush
 * of the screen is enough to start a delete. Almost half the row has to be crossed on
 * purpose, and a shorter drag snaps back without a word. Material settles a fast flick
 * on velocity alone, so the confirmation below is what actually keeps a delete from
 * happening by accident.
 */
private const val SWIPE_THRESHOLD = 0.45f

/**
 * A list row that deletes on a right-to-left swipe, replacing the red trash IconButton
 * the panel-style rows used to carry. The swipe is the only delete affordance, so the row
 * content itself stays free of destructive controls.
 *
 * A swipe never deletes on its own: the row settles, the app's own confirmation window
 * asks, and the delete happens only if that is confirmed. A cancelled confirmation puts
 * the row back where it was. There is no way to ask for a swipe that deletes outright —
 * the confirmation is what makes the gesture safe, so it is not optional.
 */
@Composable
@Suppress("LongParameterList")
fun SwipeToDeleteRow(
    onDelete: () -> Unit,
    confirmTitle: String,
    modifier: Modifier = Modifier,
    confirmText: String? = null,
    confirmLabel: String = "Delete",
    /**
     * How much of the row a swipe must cross before it counts as a delete. The panel
     * default is a deliberate half-row drag; a row living inside a vertical scroller
     * can pass a higher threshold so a diagonal scroll never settles as a swipe.
     */
    threshold: Float = SWIPE_THRESHOLD,
    /**
     * False switches the swipe off entirely without removing the row or its dialog.
     * A host that is itself scrolling (the dashboard grid) sets this while a scroll is
     * in flight, so reading the page cannot arm a delete.
     */
    enabled: Boolean = true,
    /**
     * A stricter gesture for rows inside a scrolling surface (the dashboard grid).
     * It starts only on a deliberate horizontal drag, ignores fling velocity, and
     * needs [threshold] of the row to be crossed before the confirmation opens.
     */
    strict: Boolean = false,
    content: @Composable () -> Unit,
) {
    if (strict) {
        StrictSwipeToDeleteRow(
            onDelete = onDelete,
            confirmTitle = confirmTitle,
            confirmText = confirmText,
            confirmLabel = confirmLabel,
            threshold = threshold,
            enabled = enabled,
            modifier = modifier,
            content = content,
        )
        return
    }

    val scope = rememberCoroutineScope()
    var confirming by remember { mutableStateOf(false) }
    val state = rememberSwipeToDismissBoxState(
        confirmValueChange = { it == SwipeToDismissBoxValue.EndToStart },
        positionalThreshold = { totalDistance -> totalDistance * threshold },
    )
    val dismissed = state.currentValue == SwipeToDismissBoxValue.EndToStart

    LaunchedEffect(dismissed) {
        if (dismissed) {
            confirming = true
        }
    }

    SwipeToDismissBox(
        state = state,
        modifier = modifier.fillMaxWidth(),
        enableDismissFromStartToEnd = false,
        gesturesEnabled = enabled,
        backgroundContent = { DeleteBackground(state = state) },
    ) {
        content()
    }

    if (confirming) {
        DeleteConfirmDialog(
            title = confirmTitle,
            text = confirmText,
            confirmLabel = confirmLabel,
            onDismiss = {
                confirming = false
                scope.launch { state.reset() }
            },
            onConfirm = {
                confirming = false
                onDelete()
                scope.launch { state.reset() }
            },
        )
    }
}

/**
 * The strict variant. A row inside a scrolling grid cannot use Material's dismiss
 * container as-is: it settles a fast flick by velocity, so a short diagonal brush
 * during a scroll could open the delete confirmation. Here the gesture
 *
 *  - starts only once the finger has crossed the horizontal touch slop, leaving
 *    vertical scrolling untouched,
 *  - tracks the finger but never settles on velocity, and
 *  - raises the confirmation only when the finger is released past [threshold] of
 *    the row width. Anything shorter springs back in silence.
 */
@Composable
@Suppress("LongParameterList")
private fun StrictSwipeToDeleteRow(
    onDelete: () -> Unit,
    confirmTitle: String,
    confirmText: String?,
    confirmLabel: String,
    threshold: Float,
    enabled: Boolean,
    modifier: Modifier = Modifier,
    content: @Composable () -> Unit,
) {
    val scope = rememberCoroutineScope()
    var confirming by remember { mutableStateOf(false) }
    var offset by remember { mutableFloatStateOf(0f) }
    var width by remember { mutableFloatStateOf(1f) }
    val danger = DigitDnsTheme.colors.danger
    val limit = width * threshold

    Box(
        modifier = modifier
            .fillMaxWidth()
            .clipToBounds()
            .onSizeChanged { width = it.width.toFloat().coerceAtLeast(1f) },
    ) {
        val progress = if (limit > 0f) (-offset / limit).coerceIn(0f, 1f) else 0f
        Box(
            modifier = Modifier
                .matchParentSize()
                .background(danger.copy(alpha = 0.15f * progress)),
            contentAlignment = Alignment.CenterEnd,
        ) {
            if (progress > 0f) {
                Text(
                    text = confirmLabel,
                    style = MaterialTheme.typography.labelLarge,
                    color = danger.copy(alpha = (0.4f + 0.6f * progress).coerceIn(0f, 1f)),
                    modifier = Modifier.padding(horizontal = 16.dp),
                )
            }
        }
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .offset { IntOffset(offset.roundToInt(), 0) }
                .pointerInput(enabled) {
                    if (!enabled) return@pointerInput
                    awaitEachGesture {
                        val down = awaitFirstDown(requireUnconsumed = false)
                        // Only a horizontal drag starts this. A vertical scroll keeps
                        // the events and moves the grid instead.
                        val slop = awaitHorizontalTouchSlopOrCancellation(down.id) { change, _ ->
                            change.consume()
                        } ?: return@awaitEachGesture
                        var travelled = 0f
                        horizontalDrag(slop.id) { change ->
                            travelled += change.positionChange().x
                            offset = travelled.coerceIn(-width, 0f)
                            change.consume()
                        }
                        if (-travelled >= limit) {
                            offset = 0f
                            confirming = true
                        } else {
                            // No velocity check: the spring home is the only outcome of
                            // a short drag, however fast it was. The animation runs on
                            // the composable's scope; the gesture scope is restricted.
                            val from = offset
                            scope.launch {
                                animate(from, 0f) { value, _ -> offset = value }
                            }
                        }
                    }
                },
        ) {
            content()
        }
    }

    if (confirming) {
        DeleteConfirmDialog(
            title = confirmTitle,
            text = confirmText,
            confirmLabel = confirmLabel,
            onDismiss = { confirming = false },
            onConfirm = {
                confirming = false
                onDelete()
            },
        )
    }
}

/** The one confirmation window both swipe variants use. */
@Composable
private fun DeleteConfirmDialog(
    title: String,
    text: String?,
    confirmLabel: String,
    onDismiss: () -> Unit,
    onConfirm: () -> Unit,
) {
    PanelDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = { text?.let { Text(it) } },
        confirmButton = {
            TextButton(onClick = onConfirm) {
                Text(confirmLabel, color = DigitDnsTheme.colors.danger)
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text("Cancel") }
        },
    )
}

/** The red reveal behind a swiped row: a plain tray with the delete label, no glyph. */
@Composable
private fun DeleteBackground(state: androidx.compose.material3.SwipeToDismissBoxState) {
    val danger = DigitDnsTheme.colors.danger
    val density = LocalDensity.current
    val offset = runCatching { state.requireOffset() }.getOrDefault(0f)
    val swiping = !offset.isNaN() && offset != 0f
    val amount = if (swiping) {
        (kotlin.math.abs(offset) / with(density) { 60.dp.toPx() }).coerceIn(0f, 1f)
    } else 0f
    Box(
        modifier = Modifier
            .fillMaxSize()
            .clip(RoundedCornerShape(8.dp))
            .background(danger.copy(alpha = if (swiping) 0.15f else 0f))
            .padding(horizontal = 16.dp),
        contentAlignment = Alignment.CenterEnd,
    ) {
        if (swiping) {
            Text(
                text = "Delete",
                style = MaterialTheme.typography.labelLarge,
                color = danger.copy(alpha = (0.4f + 0.6f * amount).coerceIn(0f, 1f)),
            )
        }
    }
}
