package com.digitrob.digitdns.ui.security

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Fingerprint
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.LifecycleResumeEffect
import com.digitrob.digitdns.R
import com.digitrob.digitdns.ui.components.PanelCard

/**
 * The local unlock gate. It is shown instead of the signed-in shell while the
 * biometric lock is engaged, so no panel content is composed before the prompt
 * succeeds.
 *
 * Device PIN/pattern is part of the same prompt, so the only fallback that has to
 * be offered here is a full sign-out — for a device whose biometrics were removed
 * or permanently locked out.
 */
@Composable
fun AppLockScreen(
    message: String?,
    authenticating: Boolean,
    onUnlock: () -> Unit,
    onSignOut: () -> Unit,
    onShown: () -> Unit = {},
) {
    // Opening the app with the lock engaged should raise the system prompt at
    // once. Wait for RESUMED: BiometricPrompt is a fragment transaction and is
    // not safe to start during the initial composition. The ViewModel
    // de-duplicates this so rotation and recomposition do not prompt twice.
    LifecycleResumeEffect(Unit) {
        onShown()
        onPauseOrDispose { }
    }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(MaterialTheme.colorScheme.background)
            .safeDrawingPadding()
            .padding(24.dp),
        contentAlignment = Alignment.Center,
    ) {
        PanelCard(modifier = Modifier.fillMaxWidth().widthIn(max = 420.dp)) {
            Column(
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.spacedBy(14.dp),
            ) {
                Icon(
                    imageVector = Icons.Filled.Fingerprint,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.primary,
                    modifier = Modifier.padding(top = 6.dp),
                )
                Text(
                    text = "DigitDNS",
                    style = MaterialTheme.typography.headlineSmall,
                    fontWeight = FontWeight.Bold,
                )
                Text(
                    text = stringResource(R.string.biometric_unlock_title),
                    style = MaterialTheme.typography.titleMedium,
                )
                Text(
                    text = stringResource(R.string.biometric_unlock_subtitle),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                message?.let {
                    Text(
                        text = it,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
                Button(
                    onClick = onUnlock,
                    enabled = !authenticating,
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    if (authenticating) {
                        CircularProgressIndicator(
                            modifier = Modifier.padding(end = 8.dp).size(16.dp),
                            strokeWidth = 2.dp,
                        )
                    }
                    Text(
                        text = stringResource(
                            if (authenticating) R.string.biometric_unlocking
                            else R.string.biometric_unlock_button,
                        ),
                    )
                }
                TextButton(onClick = onSignOut, enabled = !authenticating) {
                    Text(stringResource(R.string.biometric_sign_out))
                }
            }
        }
    }
}
