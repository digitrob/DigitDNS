package com.digitrob.digitdns.ui.clients

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.ui.components.PanelDialog
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

/**
 * The new-client form, shown as a dialog over the client list.
 *
 * Adding a subscriber is a short, self-contained job: name it, optionally set a
 * quota, an address list and a validity window. Sending the operator to a
 * separate screen for that pushed the list out of view and turned "add one more"
 * into a navigation, so the form now opens over the list and closes as soon as
 * the client exists.
 *
 * The form itself is the one the detail screen uses, so the two paths cannot
 * drift apart; only the framing differs.
 */
@Composable
fun NewClientDialog(
    onDismiss: () -> Unit,
    onCreated: () -> Unit,
    viewModel: ClientDetailViewModel,
) {
    val state by viewModel.state.collectAsStateWithLifecycle()

    // Saving flips the flag; that is the signal to close the dialog and let the
    // list show the client that was just created.
    LaunchedEffect(state.saved) {
        if (state.saved) onCreated()
    }

    PanelDialog(
        onDismissRequest = onDismiss,
        title = { Text("New client") },
        text = {
            // The form is taller than a dialog slot: a capped, scrollable column
            // keeps every field reachable on a short screen instead of clipping
            // the last ones off.
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(max = 400.dp)
                    .verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                state.error?.let {
                    Text(
                        text = it,
                        color = DigitDnsTheme.colors.danger,
                        style = MaterialTheme.typography.bodySmall,
                    )
                }
                ClientForm(state = state, viewModel = viewModel)
            }
        },
        confirmButton = {
            TextButton(onClick = viewModel::save, enabled = state.canSave) {
                Text(if (state.isSaving) "Creating…" else "Create")
            }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}
