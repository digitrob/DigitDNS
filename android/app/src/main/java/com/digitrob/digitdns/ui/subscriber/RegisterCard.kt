package com.digitrob.digitdns.ui.subscriber

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Key
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

/**
 * Registration Secret and Registered Address, at the bottom of the Status tab.
 *
 * Two fields and two actions in four compact rows: the tab has no page scroll, so
 * this card cannot afford the stacked labels and full-width buttons the old Connect
 * layout used.
 */
@Composable
internal fun RegisterCard(
    state: SubscriberUiState,
    form: RegisterForm,
    onChange: (RegisterForm) -> Unit,
    onRegisterDevice: () -> Unit,
    onRegisterCustom: () -> Unit,
    modifier: Modifier = Modifier,
) {
    SubscriberCard(
        verticalPadding = 10.dp,
        title = "Registration Secret",
        icon = Icons.Filled.Key,
        modifier = modifier,
        fillHeight = true,
    ) {
        Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Spacer(Modifier.weight(1f))
            Text(
                "Save on this device",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Spacer(Modifier.width(8.dp))
            Switch(
                checked = form.saveSecret,
                onCheckedChange = { onChange(form.copy(saveSecret = it)) },
                modifier = Modifier.height(28.dp),
            )
        }
        OutlinedTextField(
            value = form.secret,
            onValueChange = { onChange(form.copy(secret = it)) },
            placeholder = {
                Text(
                    if (state.secretSaved) "Saved on this device" else "From your provider",
                    style = MaterialTheme.typography.bodySmall,
                )
            },
            singleLine = true,
            visualTransformation = PasswordVisualTransformation(),
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
            textStyle = MaterialTheme.typography.bodySmall,
            modifier = Modifier.fillMaxWidth(),
        )
        // The card takes a share of the screen, so the address half is anchored at
        // its bottom instead of leaving the extra height as a void under the fields.
        Spacer(Modifier.weight(1f))
        Text(
            "Registered Address",
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        OutlinedTextField(
            value = form.customIp,
            onValueChange = { onChange(form.copy(customIp = it)) },
            placeholder = { Text("203.0.113.9 or 10.0.0.0/24", style = MaterialTheme.typography.bodySmall) },
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
            textStyle = MaterialTheme.typography.bodySmall,
            modifier = Modifier.fillMaxWidth(),
        )
        RegisterActions(
            busy = state.registerBusy,
            canRegisterCustom = form.customIp.isNotBlank(),
            onRegisterDevice = onRegisterDevice,
            onRegisterCustom = onRegisterCustom,
        )
        state.error?.let { message ->
            Text(
                message,
                color = DigitDnsTheme.colors.danger,
                style = MaterialTheme.typography.bodySmall,
            )
        }
    }
}

@Composable
private fun RegisterActions(
    busy: Boolean,
    canRegisterCustom: Boolean,
    onRegisterDevice: () -> Unit,
    onRegisterCustom: () -> Unit,
) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Button(
            onClick = onRegisterDevice,
            enabled = !busy,
            contentPadding = PaddingValues(horizontal = 10.dp, vertical = 8.dp),
            modifier = Modifier.weight(1f),
        ) {
            if (busy) {
                CircularProgressIndicator(modifier = Modifier.size(14.dp), strokeWidth = 2.dp)
                Spacer(Modifier.width(6.dp))
            }
            Text("Register This Device", style = MaterialTheme.typography.labelMedium)
        }
        Button(
            onClick = onRegisterCustom,
            enabled = !busy && canRegisterCustom,
            contentPadding = PaddingValues(horizontal = 10.dp, vertical = 8.dp),
            modifier = Modifier.weight(1f),
        ) {
            Text("Register Address", style = MaterialTheme.typography.labelMedium)
        }
    }
}
