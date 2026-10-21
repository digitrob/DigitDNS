package com.digitrob.digitdns.ui.components

import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ShowChart
import androidx.compose.material.icons.automirrored.filled.TrendingUp
import androidx.compose.material.icons.filled.Bolt
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.Memory
import androidx.compose.material.icons.filled.Podcasts
import androidx.compose.material.icons.filled.Storage
import androidx.compose.material.icons.filled.Timeline
import androidx.compose.ui.graphics.vector.ImageVector
import com.digitrob.digitdns.data.DashboardCard

/**
 * The icon each dashboard card wears, mapped from the panel's own Feather set.
 *
 * The app has no Feather font, so each glyph is matched to the nearest Material icon
 * rather than invented: `zap`→[Bolt], `database`→[Storage], `share-2`→[ShowChart],
 * `cpu`→[Memory], `server`→[Dns], `activity`→[Timeline], `radio`→[Podcasts]. Keeping
 * the panel's per-card icon is what makes the phone dashboard recognisable to someone
 * who already uses the web panel.
 */
val DashboardCard.icon: ImageVector
    get() = when (this) {
        DashboardCard.QUERY_RATE -> Icons.Filled.Bolt
        DashboardCard.RAM_CACHE -> Icons.Filled.Storage
        DashboardCard.SNI_PROXY -> Icons.AutoMirrored.Filled.ShowChart
        DashboardCard.CPU_USAGE -> Icons.Filled.Memory
        DashboardCard.RAM_USAGE -> Icons.Filled.Dns
        DashboardCard.LIVE_TRAFFIC -> Icons.AutoMirrored.Filled.TrendingUp
        DashboardCard.TELEMETRY_CHART -> Icons.AutoMirrored.Filled.ShowChart
        DashboardCard.UPSTREAM_RACERS -> Icons.Filled.Podcasts
        DashboardCard.RESOLVER_LATENCY -> Icons.Filled.Timeline
    }
