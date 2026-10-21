package com.digitrob.digitdns.ui.components

import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.TrendingFlat
import androidx.compose.material.icons.filled.Block
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Shield
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

/** The four routing quick actions, sharing the panel's own words, icons and colours. */
enum class RouteAction(val label: String, val icon: ImageVector) {
    Proxy("Proxy", Icons.Filled.Shield),
    Block("Block", Icons.Filled.Block),
    Direct("Direct", Icons.AutoMirrored.Filled.TrendingFlat),
    Remove("Remove", Icons.Filled.Delete),
    ;

    /** Proxy reads cyan, Block red, Direct green — matching the web badge colours. */
    @Composable
    fun color(): Color = when (this) {
        Proxy -> DigitDnsTheme.colors.info
        Block -> DigitDnsTheme.colors.danger
        Direct -> DigitDnsTheme.colors.success
        Remove -> DigitDnsTheme.colors.danger
    }
}
