package com.digitrob.digitdns.ui.auth

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Shield
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
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
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.domain.model.CredentialMode
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.components.PanelDialog

/**
 * The first screen: where the install is, and how the app authenticates to it.
 *
 * The two halves are on one screen deliberately. A connection to a DigitDNS install
 * is meaningless without a credential (every data route is gated), so splitting them
 * into a "connect" step and a "login" step would only add a dead intermediate state
 * where the app holds an address it cannot use.
 */
@Composable
fun LoginScreen(
    onAuthenticated: () -> Unit,
    addNewServer: Boolean = false,
    sessionExpired: Boolean = false,
    viewModel: LoginViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val extended = DigitDnsTheme.colors

    // Prepare the form once: prefill from the active install, unless this is the
    // "Add server" entry point, which wants a blank profile.
    LaunchedEffect(Unit) { viewModel.start(addNewServer = addNewServer) }

    // Navigating out of a composable is a side effect, so it runs in an effect keyed
    // on the flag rather than inline in the composition body.
    LaunchedEffect(state.authenticated) {
        if (state.authenticated) onAuthenticated()
    }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(MaterialTheme.colorScheme.background),
        contentAlignment = Alignment.Center,
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .widthIn(max = 480.dp)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 24.dp, vertical = 32.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                LoginHeader()
            }

            ConnectionFields(state, viewModel)

            // Show exactly what the app will contact, so a typo in the admin path or a
            // missing scheme is visible before a failed attempt rather than after.
            val resolvedUrl = remember(state.host, state.port, state.adminPath, state.isHttps) {
                resolvedBaseUrl(state)
            }
            if (resolvedUrl != null) {
                Text(
                    text = resolvedUrl,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }

            CredentialModeToggle(state, viewModel)

            when (state.credentialMode) {
                CredentialMode.SESSION -> SessionFields(state, viewModel)
                CredentialMode.API_KEY -> ApiKeyFields(state, viewModel)
            }

            // The saved-credentials tick is only offered for a session login: an API key is
            // itself the stored credential, so "save" would be meaningless there.
            if (state.credentialMode == CredentialMode.SESSION) {
                SaveCredentialsToggle(state, viewModel)
            }

            state.error?.let { message ->
                Text(
                    text = message,
                    color = extended.danger,
                    style = MaterialTheme.typography.bodySmall,
                )
            }

            if (sessionExpired && state.error == null) {
                Text(
                    text = "Your session expired. Sign in again to continue.",
                    color = extended.warning,
                    style = MaterialTheme.typography.bodySmall,
                )
            }

            LoginSubmit(state = state, onSubmit = viewModel::submit)
        }
    }

    state.trustPrompt?.let { prompt ->
        TrustCertificateDialog(
            prompt = prompt,
            onTrust = viewModel::onTrustCertificate,
            onDismiss = viewModel::onDismissTrust,
        )
    }
}

/** The branded header: shield mark, wordmark and the one-line purpose line. */
@Composable
private fun LoginHeader() {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Icon(
            imageVector = Icons.Filled.Shield,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.primary,
        )
        Spacer(Modifier.width(10.dp))
        Text(
            text = "DigitDNS",
            style = MaterialTheme.typography.displaySmall,
            color = MaterialTheme.colorScheme.primary,
        )
    }
    Text(
        text = "Connect to your install",
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

/** The primary action, with its in-flight spinner and the two-factor verb. */
@Composable
private fun LoginSubmit(state: LoginUiState, onSubmit: () -> Unit) {
    Button(
        onClick = onSubmit,
        enabled = state.canSubmit,
        modifier = Modifier.fillMaxWidth(),
    ) {
        if (state.isLoading) {
            CircularProgressIndicator(
                modifier = Modifier.height(18.dp).width(18.dp),
                strokeWidth = 2.dp,
                color = MaterialTheme.colorScheme.onPrimary,
            )
            Spacer(Modifier.width(12.dp))
        }
        Text(if (state.needsTwoFactor) "Verify code" else "Sign in")
    }
}

/**
 * The resolved base URL, or null when the host is blank. Mirrors
 * [com.digitrob.digitdns.domain.model.Connection.baseUrl] so the hint matches what the
 * app will actually contact — the admin path is trimmed the same way.
 */
private fun resolvedBaseUrl(state: LoginUiState): String? {
    if (state.host.isBlank()) return null
    val scheme = if (state.isHttps) "https" else "http"
    val path = state.adminPath.trim('/')
    val origin = "$scheme://${state.host}:${state.port}"
    return if (path.isEmpty()) "$origin/" else "$origin/$path/"
}

/**
 * The trust-on-first-use prompt. It shows the exact SHA-256 fingerprint the server
 * presented and asks the operator to confirm it — the same posture SSH takes, because
 * a fresh install's certificate is self-signed and there is no CA to consult. A
 * *changed* fingerprint is called out loudly: the pin rotated, which is either a
 * legitimate re-issue or an interception, and the operator must decide.
 */
@Composable
private fun TrustCertificateDialog(
    prompt: TrustPrompt,
    onTrust: () -> Unit,
    onDismiss: () -> Unit,
) {
    PanelDialog(
        onDismissRequest = onDismiss,
        title = { Text(if (prompt.changedFrom != null) "Certificate changed" else "Trust this install?") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    if (prompt.changedFrom != null) {
                        "The certificate this install presents has changed since you last trusted it. " +
                            "Confirm the fingerprint before continuing — a change you did not make could be an interception."
                    } else {
                        "This install uses a certificate your device does not yet trust. " +
                            "Confirm the fingerprint matches your server before continuing."
                    },
                    style = MaterialTheme.typography.bodyMedium,
                )
                Text(
                    text = prompt.fingerprint,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Text(
                    text = "Tip: connecting by the certificate's hostname (e.g. a domain) lets the " +
                        "system validate it automatically, with no prompt.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        },
        confirmButton = { TextButton(onClick = onTrust) { Text("Trust and continue") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@Composable
private fun ConnectionFields(state: LoginUiState, viewModel: LoginViewModel) {
    OutlinedTextField(
        value = state.host,
        onValueChange = viewModel::onHostChange,
        label = { Text("Host or IP") },
        singleLine = true,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri, imeAction = ImeAction.Next),
        modifier = Modifier.fillMaxWidth(),
    )
    Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        OutlinedTextField(
            value = state.port,
            onValueChange = viewModel::onPortChange,
            label = { Text("Port") },
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number, imeAction = ImeAction.Next),
            modifier = Modifier.width(120.dp),
        )
        OutlinedTextField(
            value = state.adminPath,
            onValueChange = viewModel::onAdminPathChange,
            label = { Text("Admin path") },
            singleLine = true,
            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Next),
            modifier = Modifier.weight(1f),
        )
    }
    Row(verticalAlignment = Alignment.CenterVertically) {
        Switch(checked = state.isHttps, onCheckedChange = viewModel::onHttpsChange)
        Spacer(Modifier.width(12.dp))
        Text("Use HTTPS", style = MaterialTheme.typography.bodyMedium)
    }
}

@Composable
private fun CredentialModeToggle(state: LoginUiState, viewModel: LoginViewModel) {
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        FilterChip(
            selected = state.credentialMode == CredentialMode.SESSION,
            onClick = { viewModel.onModeChange(CredentialMode.SESSION) },
            label = { Text("Account") },
        )
        FilterChip(
            selected = state.credentialMode == CredentialMode.API_KEY,
            onClick = { viewModel.onModeChange(CredentialMode.API_KEY) },
            label = { Text("API key") },
        )
    }
}

@Composable
private fun SessionFields(state: LoginUiState, viewModel: LoginViewModel) {
    OutlinedTextField(
        value = state.username,
        onValueChange = viewModel::onUsernameChange,
        label = { Text("Username") },
        singleLine = true,
        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Next),
        modifier = Modifier.fillMaxWidth(),
    )
    OutlinedTextField(
        value = state.password,
        onValueChange = viewModel::onPasswordChange,
        label = { Text("Password") },
        singleLine = true,
        visualTransformation = PasswordVisualTransformation(),
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Next),
        modifier = Modifier.fillMaxWidth(),
    )
    if (state.needsTwoFactor) {
        OutlinedTextField(
            value = state.code,
            onValueChange = viewModel::onCodeChange,
            label = { Text("Two-factor code") },
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number, imeAction = ImeAction.Done),
            modifier = Modifier.fillMaxWidth(),
        )
        Text(
            text = "Enter the 6-digit code from your authenticator app.",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/**
 * The opt-in that lets the app keep the password. Separate from [SessionFields] because
 * it is a policy choice, not a field: it changes what the app stores, not what it sends.
 */
@Composable
private fun SaveCredentialsToggle(state: LoginUiState, viewModel: LoginViewModel) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Checkbox(
            checked = state.saveCredentials,
            onCheckedChange = viewModel::onSaveCredentialsChange,
        )
        Spacer(Modifier.width(8.dp))
        Column {
            Text("Save credentials", style = MaterialTheme.typography.bodyMedium)
            Text(
                text = "Stays signed in across restarts and re-logs in after the session lapses.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun ApiKeyFields(state: LoginUiState, viewModel: LoginViewModel) {
    var revealed by remember { mutableStateOf(false) }
    OutlinedTextField(
        value = state.apiKey,
        onValueChange = viewModel::onApiKeyChange,
        label = { Text("Master API key") },
        singleLine = true,
        visualTransformation = if (revealed) androidx.compose.ui.text.input.VisualTransformation.None
        else PasswordVisualTransformation(),
        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
        modifier = Modifier.fillMaxWidth(),
    )
    TextButton(onClick = { revealed = !revealed }) {
        Text(if (revealed) "Hide key" else "Show key")
    }
    Text(
        text = "The master key grants full control of the install. Prefer an account where you can.",
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}
