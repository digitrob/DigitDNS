package com.digitrob.digitdns.core.quicktile

import android.service.quicksettings.Tile
import android.service.quicksettings.TileService
import com.digitrob.digitdns.R
import com.digitrob.digitdns.core.network.FilteringSettingsRequest
import com.digitrob.digitdns.data.AuthRepository
import com.digitrob.digitdns.data.FilteringRepository
import dagger.hilt.android.AndroidEntryPoint
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import javax.inject.Inject

/**
 * Quick Settings toggle for the resolver's ad blocking.
 *
 * The tile talks to the same filtering endpoints as the Ad Blocking screen, so
 * it cannot drift from the panel. It loads the active install and mints a
 * session on first listen: Quick Settings can open the tile process without the
 * activity ever having run in this process.
 *
 * A tile has no room for a full error message, so a failed read or write marks
 * it unavailable and says "Sign in to DigitDNS" rather than pretending the
 * toggle worked.
 */
@AndroidEntryPoint
class FilteringTileService : TileService() {

    @Inject lateinit var authRepository: AuthRepository
    @Inject lateinit var filteringRepository: FilteringRepository

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    override fun onStartListening() {
        super.onStartListening()
        scope.launch {
            val status = runCatching {
                authRepository.migrateLegacyIfNeeded()
                authRepository.loadActiveCredential()
                if (!authRepository.hasCredential()) return@runCatching null
                authRepository.ensureSession()
                filteringRepository.status()
            }.getOrNull()
            if (status == null) {
                render(available = false, enabled = false)
            } else {
                render(available = true, enabled = status.settings.enabled)
            }
        }
    }

    override fun onClick() {
        super.onClick()
        scope.launch {
            val status = runCatching { filteringRepository.status() }.getOrNull()
            if (status == null) {
                render(available = false, enabled = false)
                return@launch
            }
            val settings = status.settings
            val next = !settings.enabled
            val saved = runCatching {
                filteringRepository.saveSettings(
                    FilteringSettingsRequest(
                        enabled = next,
                        blockMode = settings.blockMode,
                        blockTtl = settings.blockTtl,
                        updateIntervalHours = settings.updateIntervalHours,
                        allowlist = settings.allowlist ?: emptyList(),
                    ),
                )
            }.getOrNull()
            if (saved == null) {
                render(available = false, enabled = settings.enabled)
            } else {
                render(available = true, enabled = saved.settings.enabled)
            }
        }
    }

    override fun onDestroy() {
        scope.cancel()
        super.onDestroy()
    }

    private suspend fun render(available: Boolean, enabled: Boolean) {
        withContext(Dispatchers.Main) {
            val tile = qsTile ?: return@withContext
            tile.state = when {
                !available -> Tile.STATE_UNAVAILABLE
                enabled -> Tile.STATE_ACTIVE
                else -> Tile.STATE_INACTIVE
            }
            tile.label = getString(
                when {
                    !available -> R.string.tile_filtering_offline
                    enabled -> R.string.tile_filtering_on
                    else -> R.string.tile_filtering_off
                },
            )
            tile.updateTile()
        }
    }
}
