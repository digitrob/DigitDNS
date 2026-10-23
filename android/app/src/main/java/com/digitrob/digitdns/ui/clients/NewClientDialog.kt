package com.digitrob.digitdns.ui.clients

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.domain.model.ClientLinks
import com.digitrob.digitdns.ui.components.PanelWideDialog
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

/**
 * The new-client form, shown as a dialog over the client list.
 *
 * Adding a subscriber is a short, self-contained job: name it, optionally set a
 * quota, an address list and an expiry. Sending the operator to a separate screen
 * for that pushed the list out of view and turned "add one more" into a
 * navigation, so the form now opens over the list.
 *
 * A successful create does not close the popup. The server response is the only
 * time the app sees the generated UUID, registration secret and subscription
 * link together, so the same dialog switches to a credentials view; the list
 * appears again only when the operator presses Done.
 */
@Composable
fun NewClientDialog(
    origin: String,
    onDismiss: () -> Unit,
    onCreated: () -> Unit,
    viewModel: ClientDetailViewModel,
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val clipboard = LocalClipboardManager.current
    val created = state.created

    // The view model is scoped to the client list and outlives this popup; reset it
    // on every open so a previous client's values, error or "created" view cannot
    // flash on the next one.
    LaunchedEffect(Unit) { viewModel.resetForNewClient() }

    // The success view is itself a dialog, so a screen-level toast would sit
    // behind it; each copy row marks itself instead.
    val copy: (String, String) -> Unit = { value, _ -> clipboard.setText(AnnotatedString(value)) }

    PanelWideDialog(
        onDismissRequest = {
            if (created == null) onDismiss() else onCreated()
        },
        title = {
            Text(
                text = if (created == null) "New client" else "Client created",
                style = MaterialTheme.typography.titleMedium,
                color = MaterialTheme.colorScheme.onSurface,
            )
        },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                state.error?.let {
                    Text(
                        text = it,
                        color = DigitDnsTheme.colors.danger,
                        style = MaterialTheme.typography.bodySmall,
                    )
                }
                if (created == null) {
                    ClientForm(state = state, viewModel = viewModel)
                } else {
                    CreatedCredentials(
                        name = created.name.ifBlank { created.id },
                        link = if (origin.isBlank()) "" else ClientLinks.subscriptionLink(origin, created),
                        secret = created.registerSecret,
                        uuid = created.uuid,
                        onCopy = copy,
                    )
                }
            }
        },
        confirmButton = {
            if (created == null) {
                TextButton(onClick = viewModel::save, enabled = state.canSave) {
                    Text(if (state.isSaving) "Creating…" else "Create")
                }
            } else {
                TextButton(onClick = onCreated) { Text("Done") }
            }
        },
        dismissButton = {
            TextButton(onClick = { if (created == null) onDismiss() else onCreated() }) {
                Text(if (created == null) "Cancel" else "Close")
            }
        },
    )
}

@Composable
private fun CreatedCredentials(
    name: String,
    link: String,
    secret: String,
    uuid: String,
    onCopy: (String, String) -> Unit,
) {
    var copied by remember { mutableStateOf<String?>(null) }
    Text(
        text = name,
        style = MaterialTheme.typography.titleSmall,
        color = MaterialTheme.colorScheme.onSurface,
    )
    Text(
        text = "Copy these now — the secret and UUID are the credentials the " +
            "subscriber needs. They stay available in the client's details.",
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    val copyAndMark: (String, String) -> Unit = { value, label ->
        onCopy(value, label)
        copied = label
    }
    CreatedRow("Subscription", link.ifBlank { "Not configured" }, link, copied, copyAndMark)
    CreatedRow("Secret", secret.ifBlank { "No secret on file" }, secret, copied, copyAndMark)
    CreatedRow("UUID", uuid.ifBlank { "N/A" }, uuid, copied, copyAndMark)
}

@Composable
private fun CreatedRow(
    label: String,
    value: String,
    copyValue: String,
    copiedLabel: String?,
    onCopy: (String, String) -> Unit,
) {
    Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Text(
            text = if (copiedLabel == label) "$label · copied" else label,
            style = MaterialTheme.typography.labelMedium,
            color = if (copiedLabel == label) {
                DigitDnsTheme.colors.success
            } else {
                MaterialTheme.colorScheme.onSurfaceVariant
            },
            modifier = Modifier.padding(end = 8.dp),
        )
        Text(
            text = value,
            style = MaterialTheme.typography.bodySmall.copy(fontFamily = FontFamily.Monospace),
            color = MaterialTheme.colorScheme.onSurface,
            maxLines = 3,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f),
        )
        if (copyValue.isNotBlank()) {
            IconButton(onClick = { onCopy(copyValue, label) }) {
                Icon(Icons.Filled.ContentCopy, contentDescription = "Copy $label")
            }
        }
    }
}
