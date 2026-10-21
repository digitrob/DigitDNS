package com.digitrob.digitdns.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
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
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.unit.dp
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import kotlinx.coroutines.launch

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
fun SwipeToDeleteRow(
    onDelete: () -> Unit,
    confirmTitle: String,
    modifier: Modifier = Modifier,
    confirmText: String? = null,
    confirmLabel: String = "Delete",
    content: @Composable () -> Unit,
) {
    val scope = rememberCoroutineScope()
    var confirming by remember { mutableStateOf(false) }
    val state = rememberSwipeToDismissBoxState(
        confirmValueChange = { it == SwipeToDismissBoxValue.EndToStart },
        positionalThreshold = { totalDistance -> totalDistance * SWIPE_THRESHOLD },
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
        backgroundContent = { DeleteBackground() },
    ) {
        content()
    }

    if (confirming) {
        PanelDialog(
            onDismissRequest = {
                confirming = false
                scope.launch { state.reset() }
            },
            title = { Text(confirmTitle) },
            text = { confirmText?.let { Text(it) } },
            confirmButton = {
                TextButton(onClick = {
                    confirming = false
                    onDelete()
                    scope.launch { state.reset() }
                }) {
                    Text(confirmLabel, color = DigitDnsTheme.colors.danger)
                }
            },
            dismissButton = {
                TextButton(onClick = {
                    confirming = false
                    scope.launch { state.reset() }
                }) { Text("Cancel") }
            },
        )
    }
}

/** The red reveal behind a swiped row: a plain tray with the delete label, no glyph. */
@Composable
private fun DeleteBackground() {
    val danger = DigitDnsTheme.colors.danger
    Box(
        modifier = Modifier
            .fillMaxSize()
            .clip(RoundedCornerShape(8.dp))
            .background(danger.copy(alpha = 0.15f))
            .padding(horizontal = 16.dp),
        contentAlignment = Alignment.CenterEnd,
    ) {
        Text(
            text = "Delete",
            style = MaterialTheme.typography.labelLarge,
            color = danger,
        )
    }
}
