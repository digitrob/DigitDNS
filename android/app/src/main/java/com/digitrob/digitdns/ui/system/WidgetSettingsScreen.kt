package com.digitrob.digitdns.ui.system

import android.content.Context
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.digitrob.digitdns.R
import com.digitrob.digitdns.core.widget.WidgetPrefs
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.components.PanelCard

/**
 * Appearance settings for every placed Resolver widget, reachable from Settings → App →
 * Widget Settings.
 *
 * The hub already draws the section title and back arrow, so this is the content only.
 * The same preferences back the launcher configure screen and the widget's gear, so all
 * three entry points stay in sync. Every change is written immediately and followed by a
 * cache-only re-render broadcast, so the widget updates without being removed.
 */
@Composable
fun WidgetSettingsScreen(modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val ids = remember { WidgetPrefs.widgetIds(context).toList() }

    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text(
            text = stringResource(R.string.settings_widgets_help),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )

        if (ids.isEmpty()) {
            PanelCard(modifier = Modifier.fillMaxWidth()) {
                Text(
                    text = stringResource(R.string.widget_settings_empty),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        } else {
            ids.forEachIndexed { index, widgetId ->
                WidgetAppearanceCard(
                    context = context,
                    widgetId = widgetId,
                    number = index + 1,
                )
            }
        }
    }
}

@Composable
private fun WidgetAppearanceCard(
    context: Context,
    widgetId: Int,
    number: Int,
) {
    var themeValue by remember(widgetId) { mutableStateOf(WidgetPrefs.themeOverride(context, widgetId)) }
    var alpha by remember(widgetId) { mutableIntStateOf(WidgetPrefs.alpha(context, widgetId)) }
    var fontScale by remember(widgetId) { mutableFloatStateOf(WidgetPrefs.fontScale(context, widgetId)) }

    // Render this widget's own controls in the theme that widget will use, so the
    // editor matches the home-screen card — and re-theme it live as a chip is tapped.
    DigitDnsTheme(themeMode = WidgetPrefs.resolveModeFor(context, themeValue)) {
        PanelCard(modifier = Modifier.fillMaxWidth()) {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    text = stringResource(R.string.widget_settings_widget, number),
                    style = MaterialTheme.typography.titleMedium,
                )
                WidgetAppearanceEditor(
                    themeValue = themeValue,
                    alphaPercent = alpha,
                    fontScale = fontScale,
                    onThemeChange = { value ->
                        themeValue = value
                        WidgetPrefs.setThemeOverride(context, widgetId, value)
                        WidgetPrefs.requestRerender(context, intArrayOf(widgetId))
                    },
                    onAlphaChange = { value ->
                        alpha = value
                        WidgetPrefs.setAlpha(context, widgetId, value)
                        WidgetPrefs.requestRerender(context, intArrayOf(widgetId))
                    },
                    onFontScaleChange = { value ->
                        fontScale = value
                        WidgetPrefs.setFontScale(context, widgetId, value)
                        WidgetPrefs.requestRerender(context, intArrayOf(widgetId))
                    },
                )
            }
        }
    }
}
