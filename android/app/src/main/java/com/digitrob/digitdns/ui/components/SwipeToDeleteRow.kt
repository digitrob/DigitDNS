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
import androidx.compose.material3.rememberSwipeToDismissBoxState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.unit.dp
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

/**
 * A list row that deletes on a right-to-left swipe, replacing the red trash IconButton
 * the panel-style rows used to carry. The swipe is the only delete affordance, so the row
 * content itself stays free of destructive controls.
 *
 * The delete is fired only once the row has settled fully dismissed; callers that need a
 * confirmation (upstreams, servers) open their dialog here rather than deleting outright.
 */
@Composable
fun SwipeToDeleteRow(
    onDelete: () -> Unit,
    modifier: Modifier = Modifier,
    content: @Composable () -> Unit,
) {
    val state = rememberSwipeToDismissBoxState(
        confirmValueChange = { it == SwipeToDismissBoxValue.EndToStart },
    )
    val dismissed = state.currentValue == SwipeToDismissBoxValue.EndToStart

    LaunchedEffect(dismissed) {
        if (dismissed) {
            onDelete()
            state.reset()
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
