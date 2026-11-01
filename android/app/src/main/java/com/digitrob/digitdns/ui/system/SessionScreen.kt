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
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import com.digitrob.digitdns.R
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
        if (state.isAuthenticated) {
            SessionIdentityCard(state)
            SignOutButton(
                isSigningOut = state.isSigningOut,
                onClick = { confirming = true },
            )
            Text(
                text = stringResource(R.string.session_signout_body),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        } else {
            // A server selected in the saved list but never authenticated is not a
            // session — showing its host in the identity box (and offering sign-out
            // for a session that does not exist) claimed otherwise. It stays visible
            // in the Saved servers list, which is where it belongs.
            Text(
                text = stringResource(R.string.session_not_signed_in),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }

    if (confirming) {
        SignOutDialog(
            isSigningOut = state.isSigningOut,
            onConfirm = { viewModel.signOut(onDone = onLoggedOut) },
            onDismiss = { confirming = false },
        )
    }
}

@Composable
private fun SessionIdentityCard(state: SessionUiState) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Text(stringResource(R.string.session_signed_in_to), style = MaterialTheme.typography.labelMedium)
            Text(
                text = state.authority.ifBlank { "—" },
                style = MaterialTheme.typography.titleMedium,
                fontFamily = FontFamily.Monospace,
            )
            if (state.adminPath.isNotBlank()) {
                Text(
                    text = stringResource(R.string.session_admin_path, state.adminPath),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    fontFamily = FontFamily.Monospace,
                )
            }
            Text(
                text = stringResource(R.string.session_credential, state.credentialLabel),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = if (state.credentialsSaved) {
                    stringResource(R.string.session_creds_saved)
                } else {
                    stringResource(R.string.session_creds_not_saved)
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
        Text(stringResource(R.string.session_sign_out))
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
        title = { Text(stringResource(R.string.session_sign_out_title)) },
        text = { Text(stringResource(R.string.session_signout_body)) },
        confirmButton = {
            TextButton(onClick = onConfirm, enabled = !isSigningOut) { Text(stringResource(R.string.session_sign_out)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss, enabled = !isSigningOut) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}
