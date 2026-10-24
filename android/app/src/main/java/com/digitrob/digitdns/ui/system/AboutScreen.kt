package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.core.network.GeoipStatusDto
import com.digitrob.digitdns.ui.components.CountryFlag
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.PanelSnackbarHost
import com.digitrob.digitdns.ui.components.showPanelToast
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import java.util.Locale

/**
 * About & legal, matching the panel's `About & Legal` card: the app identity, the
 * running server's system and update status (relocated from the retired Updates page),
 * and the AGPL attribution. The server version comes from the same summary the Update
 * card reads, so the two never disagree.
 */
@Composable
fun AboutScreen(viewModel: MaintenanceViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }

    LaunchedEffect(state.message) {
        val message = state.message ?: return@LaunchedEffect
        snackbar.showPanelToast(message)
        viewModel.consumeMessage()
    }

    Scaffold(snackbarHost = { PanelSnackbarHost(snackbar) }, contentWindowInsets = WindowInsets(0)) { padding ->
        PanelRefreshBox(
            isRefreshing = state.isRefreshing,
            onRefresh = viewModel::loadSummary,
            modifier = Modifier.fillMaxSize().padding(padding),
        ) {
            Column(
                modifier = Modifier
                    .fillMaxSize()
                    .verticalScroll(rememberScrollState())
                    .padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                PanelCard(modifier = Modifier.fillMaxWidth()) {
                    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                        Text("DigitDNS Companion", style = MaterialTheme.typography.titleMedium)
                        Text(
                            text = "Android companion for the DigitDNS panel.",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }

                state.summary?.let { SystemCard(it) }

                GeoipCard(state, viewModel)

                UpdateCard(state, viewModel)

                PanelCard(modifier = Modifier.fillMaxWidth()) {
                    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                        Text("License", style = MaterialTheme.typography.titleMedium)
                        Text(
                            text = "DigitDNS is free software, licensed under the GNU Affero " +
                                "General Public License v3.0 or later.",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }
        }
    }
}

/**
 * The server's country flag and the GeoIP database behind it, plus a manual
 * refresh. The database downloads daily on its own; this button exists for the
 * case that matters — a fresh install with no file yet, or a lookup that came
 * back wrong and is worth re-fetching.
 */
@Composable
private fun GeoipCard(state: MaintenanceUiState, viewModel: MaintenanceViewModel) {
    val status = state.geoip
    PanelCard(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(
                    modifier = Modifier.weight(1f),
                    verticalArrangement = Arrangement.spacedBy(3.dp),
                ) {
                    Text("Server Location", style = MaterialTheme.typography.titleMedium)
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        val code = status?.country.orEmpty()
                        if (code.isNotBlank()) {
                            CountryFlag(code)
                            Spacer(Modifier.width(8.dp))
                        }
                        Text(countryLabel(code), style = MaterialTheme.typography.bodyMedium)
                    }
                    Text(
                        text = geoipDatabaseLine(status),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Spacer(Modifier.width(8.dp))
                OutlinedButton(
                    onClick = viewModel::updateGeoip,
                    enabled = !state.geoipUpdating,
                ) {
                    if (state.geoipUpdating) {
                        CircularProgressIndicator(
                            modifier = Modifier.size(14.dp),
                            strokeWidth = 2.dp,
                        )
                    } else {
                        Icon(
                            imageVector = Icons.Filled.Refresh,
                            contentDescription = null,
                            modifier = Modifier.size(16.dp),
                        )
                    }
                    Spacer(Modifier.width(6.dp))
                    Text(if (state.geoipUpdating) "Updating…" else "Update")
                }
            }
            state.geoipError?.let {
                Text(it, style = MaterialTheme.typography.bodySmall, color = DigitDnsTheme.colors.danger)
            }
            status?.error?.takeIf { it.isNotBlank() }?.let {
                Text(it, style = MaterialTheme.typography.bodySmall, color = DigitDnsTheme.colors.danger)
            }
        }
    }
}

/** "Unknown" rather than an empty line when the database has no answer. */
private fun countryLabel(code: String): String {
    if (code.isBlank()) return "Unknown"
    val upper = code.uppercase(Locale.ROOT)
    val name = Locale("", upper).getDisplayCountry(Locale.getDefault())
    return name.ifBlank { upper }
}

private fun geoipDatabaseLine(status: GeoipStatusDto?): String = when {
    status == null -> "GeoIP database: checking…"
    !status.available -> "GeoIP database: not downloaded yet"
    else -> "GeoIP database: ${status.databaseDate.ifBlank { "date unknown" }} · updates daily"
}
