package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.VerifiedUser
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.QrCode
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono

/**
 * Two-factor (TOTP) enrolment. The flow is: identify → scan the QR → confirm a code.
 * The QR is the same component the Connect tab uses, drawn from the `otpauth://` URI
 * the setup call returns — the server never sends a QR image.
 */
@Composable
fun SecurityScreen(viewModel: SecurityViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()

    PanelRefreshBox(
        isRefreshing = state.isLoading,
        onRefresh = viewModel::loadStatus,
        modifier = Modifier.fillMaxSize(),
    ) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.End) {
            val badgeColor = if (state.totpEnabled) DigitDnsTheme.colors.success else DigitDnsTheme.colors.textMuted
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(
                    Icons.Filled.VerifiedUser,
                    contentDescription = null,
                    tint = badgeColor,
                    modifier = Modifier.size(16.dp),
                )
                Spacer(Modifier.width(6.dp))
                Text(
                    text = if (state.totpEnabled) "2FA On" else "2FA Off",
                    style = MaterialTheme.typography.labelSmall,
                    color = badgeColor,
                )
            }
        }

        state.error?.let {
            Text(it, color = DigitDnsTheme.colors.danger, style = MaterialTheme.typography.bodySmall)
        }
        state.message?.let { message ->
            PanelCard {
                Row(modifier = Modifier.fillMaxWidth()) {
                    Text(message, color = MaterialTheme.colorScheme.primary, modifier = Modifier.weight(1f))
                    TextButton(onClick = viewModel::dismissMessage) { Text("OK") }
                }
            }
        }

        if (state.isLoading) {
            CircularProgressIndicator(color = MaterialTheme.colorScheme.primary)
            return@Column
        }

        CredentialsCard(state)

        CredentialFields(state, viewModel)

        TwoFactorAction(state, viewModel)
    }
    }
}

@Composable
private fun TwoFactorAction(state: SecurityUiState, viewModel: SecurityViewModel) {
    when (state.step) {
        EnrolStep.AwaitingConfirmation -> EnrolmentCard(state, viewModel)
        else -> {
            if (state.totpEnabled) {
                OutlinedButton(
                    onClick = viewModel::disable,
                    enabled = canSubmit(state),
                    modifier = Modifier.fillMaxWidth(),
                ) { Text("Disable two-factor") }
            } else {
                Button(
                    onClick = viewModel::beginEnrolment,
                    enabled = canSubmit(state),
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    if (state.working) {
                        CircularProgressIndicator(
                            modifier = Modifier.padding(end = 8.dp),
                            strokeWidth = 2.dp,
                            color = MaterialTheme.colorScheme.onPrimary,
                        )
                    }
                    Text("Start Enrolment")
                }
            }
        }
    }
}

@Composable
private fun CredentialsCard(state: SecurityUiState) {
    val clipboard = LocalClipboardManager.current
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("Credentials", style = MaterialTheme.typography.titleMedium)
            CredentialValue("API key", state.apiKey) { clipboard.setText(AnnotatedString(state.apiKey)) }
            CredentialValue("Admin path", state.adminPath) { clipboard.setText(AnnotatedString(state.adminPath)) }
            Text(
                text = "Regeneration lives on the REST API page.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun CredentialValue(label: String, value: String, onCopy: () -> Unit) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = label,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = value.ifBlank { "—" },
                style = MaterialTheme.typography.bodySmall.copy(fontFamily = PanelMono),
                color = MaterialTheme.colorScheme.onSurface,
            )
        }
        if (value.isNotBlank()) {
            IconButton(onClick = onCopy) {
                Icon(Icons.Filled.ContentCopy, contentDescription = "Copy $label")
            }
        }
    }
}

@Composable
private fun CredentialFields(state: SecurityUiState, viewModel: SecurityViewModel) {
    OutlinedTextField(
        value = state.username,
        onValueChange = viewModel::onUsernameChange,
        label = { Text("Admin username") },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
    OutlinedTextField(
        value = state.currentPassword,
        onValueChange = viewModel::onPasswordChange,
        label = { Text("Current password") },
        singleLine = true,
        visualTransformation = PasswordVisualTransformation(),
        modifier = Modifier.fillMaxWidth(),
    )
    if (state.totpEnabled || state.step == EnrolStep.AwaitingConfirmation) {
        OutlinedTextField(
            value = state.code,
            onValueChange = viewModel::onCodeChange,
            label = { Text("6-digit code") },
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.NumberPassword),
            modifier = Modifier.fillMaxWidth(),
        )
    }
}

@Composable
private fun EnrolmentCard(state: SecurityUiState, viewModel: SecurityViewModel) {
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(
            modifier = Modifier.fillMaxWidth(),
            verticalArrangement = Arrangement.spacedBy(10.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Text("Scan this with your authenticator", style = MaterialTheme.typography.titleMedium)
            if (state.enrolmentUri.isNotBlank()) {
                QrCode(content = state.enrolmentUri, size = 200.dp)
            }
            Text(
                text = "Or enter the code manually:",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = state.enrolmentSecret,
                style = MaterialTheme.typography.bodyMedium.copy(fontFamily = PanelMono),
            )
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = viewModel::confirmEnrolment, enabled = !state.working && state.code.isNotBlank()) {
                    Text(if (state.working) "Verifying…" else "Confirm")
                }
                OutlinedButton(onClick = viewModel::cancelEnrolment) { Text("Cancel") }
            }
        }
    }
}

/** A 2FA write needs a username, the password, and (once enrolling) a code. */
private fun canSubmit(state: SecurityUiState): Boolean =
    !state.working &&
        state.username.isNotBlank() &&
        state.currentPassword.isNotBlank() &&
        (!state.totpEnabled || state.code.isNotBlank())
