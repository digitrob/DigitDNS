package com.digitrob.digitdns.ui.system

import android.content.Context
import android.os.Build
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.R
import com.digitrob.digitdns.core.notifications.LiveStreamService
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.google.accompanist.permissions.ExperimentalPermissionsApi
import com.google.accompanist.permissions.isGranted
import com.google.accompanist.permissions.rememberPermissionState

/**
 * The live-alerts opt-in.
 *
 * Turning the switch on does two things in order: request the notification
 * permission (Android 13+; below that it is granted at install), then start the
 * foreground stream service. Turning it off stops the service and the persistent
 * notification with it. The state is persisted by the view model, so the switch and
 * the running service stay in step across launches.
 */
@OptIn(ExperimentalPermissionsApi::class)
@Composable
fun AlertsScreen(viewModel: LiveAlertsViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val context = LocalContext.current

    // POST_NOTIFICATIONS exists from API 33; below it the permission is implicit, so
    // the state is read only where the runtime permission exists.
    val notificationPermission = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
        rememberPermissionState(android.Manifest.permission.POST_NOTIFICATIONS)
    } else {
        null
    }

    val permissionGranted = notificationPermission?.status?.isGranted ?: true

    // The service is a side effect of the persisted intent, not of the click: on
    // launch this reconciles a toggle left on with a service the OS may have killed.
    LaunchedEffect(state.enabled, state.isLoading, permissionGranted) {
        if (state.isLoading) return@LaunchedEffect
        if (state.enabled && permissionGranted) {
            LiveStreamService.start(context)
        } else {
            LiveStreamService.stop(context)
        }
    }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        PanelCard(modifier = Modifier.fillMaxWidth()) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(modifier = Modifier.weight(1f)) {
                    Text(
                        text = stringResource(R.string.alerts_title),
                        style = MaterialTheme.typography.titleMedium,
                    )
                    Text(
                        text = stringResource(R.string.alerts_description),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Switch(
                    checked = state.enabled,
                    enabled = !state.isLoading && state.canEnable,
                    onCheckedChange = { wanted ->
                        if (wanted) {
                            notificationPermission?.launchPermissionRequest()
                        }
                        viewModel.setEnabled(wanted)
                        toggleService(context, wanted)
                    },
                )
            }
        }

        if (state.enabled && !permissionGranted) {
            Text(
                text = stringResource(R.string.alerts_permission_needed),
                style = MaterialTheme.typography.bodySmall,
                color = DigitDnsTheme.colors.warning,
            )
        }
        if (!state.canEnable) {
            Text(
                text = "Sign in first — the stream authenticates with your saved credential.",
                style = MaterialTheme.typography.bodySmall,
                color = DigitDnsTheme.colors.warning,
            )
        }
    }
}

/** Start or stop the stream service to match the switch now, without waiting on state. */
private fun toggleService(context: Context, enabled: Boolean) {
    if (enabled) LiveStreamService.start(context) else LiveStreamService.stop(context)
}
