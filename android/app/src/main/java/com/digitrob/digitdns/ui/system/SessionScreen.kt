package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.ui.components.PanelDialog

/**
 * The Session view: what the app is signed into, and the way out.
 *
 * Signing out asks for confirmation first — it drops the session *and* any saved
 * credentials, so it is not a tap to fire off by accident.
 */
@Composable
fun SessionScreen(
    onLoggedOut: () -> Unit,
    viewModel: SessionViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    var confirming by remember { mutableStateOf(false) }

    // No scroll/fill of its own: this is embedded under Servers' scrolling column, and
    // a second vertical scroll container inside one is a runtime constraint crash.
    Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
        SessionIdentityCard(state)
        SignOutButton(
            isSigningOut = state.isSigningOut,
            onClick = { confirming = true },
        )
        Text(
            text = CONSEQUENCE,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }

    if (confirming) {
        SignOutDialog(
            isSigningOut = state.isSigningOut,
            onConfirm = { viewModel.signOut(onDone = onLoggedOut) },
            onDismiss = { confirming = false },
        )
    }
}

private const val CONSEQUENCE =
    "Signing out revokes the session on the server and removes any saved credentials " +
        "from this device."

@Composable
private fun SessionIdentityCard(state: SessionUiState) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Text("Signed in to", style = MaterialTheme.typography.labelMedium)
            Text(
                text = state.authority.ifBlank { "—" },
                style = MaterialTheme.typography.titleMedium,
                fontFamily = FontFamily.Monospace,
            )
            if (state.adminPath.isNotBlank()) {
                Text(
                    text = "Admin path: ${state.adminPath}",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    fontFamily = FontFamily.Monospace,
                )
            }
            Text(
                text = "Credential: ${state.credentialLabel}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = if (state.credentialsSaved) {
                    "Saved credentials: yes — the app re-logs in automatically."
                } else {
                    "Saved credentials: no."
                },
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun SignOutButton(isSigningOut: Boolean, onClick: () -> Unit) {
    Button(
        onClick = onClick,
        enabled = !isSigningOut,
        colors = ButtonDefaults.buttonColors(
            containerColor = MaterialTheme.colorScheme.errorContainer,
            contentColor = MaterialTheme.colorScheme.onErrorContainer,
        ),
        modifier = Modifier.fillMaxWidth(),
    ) {
        if (isSigningOut) {
            CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp))
        }
        Text("Sign out")
    }
}

@Composable
private fun SignOutDialog(
    isSigningOut: Boolean,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    PanelDialog(
        onDismissRequest = { if (!isSigningOut) onDismiss() },
        title = { Text("Sign out?") },
        text = { Text(CONSEQUENCE) },
        confirmButton = {
            TextButton(onClick = onConfirm, enabled = !isSigningOut) { Text("Sign out") }
        },
        dismissButton = {
            TextButton(onClick = onDismiss, enabled = !isSigningOut) { Text("Cancel") }
        },
    )
}
