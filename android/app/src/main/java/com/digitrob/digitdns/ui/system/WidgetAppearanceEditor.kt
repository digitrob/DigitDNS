package com.digitrob.digitdns.ui.system

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Slider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.digitrob.digitdns.R
import com.digitrob.digitdns.core.widget.WidgetPrefs
import kotlin.math.roundToInt

/**
 * The appearance controls shared by the launcher configure screen and Settings → App →
 * Widget Settings. Theme is stored as null (follow the app) or one of the [ThemeMode]
 * names; transparency is 0–100; text size is a multiplier around 1.0.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun WidgetAppearanceEditor(
    themeValue: String?,
    alphaPercent: Int,
    fontScale: Float,
    onThemeChange: (String?) -> Unit,
    onAlphaChange: (Int) -> Unit,
    onFontScaleChange: (Float) -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(modifier = modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Text(
            text = stringResource(R.string.widget_settings_theme),
            style = MaterialTheme.typography.labelLarge,
        )
        FlowRow(
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            ThemeChoice(
                selected = themeValue == null,
                label = R.string.widget_settings_theme_app,
                onClick = { onThemeChange(null) },
            )
            ThemeChoice(
                selected = themeValue == "DARK",
                label = R.string.widget_settings_theme_dark,
                onClick = { onThemeChange("DARK") },
            )
            ThemeChoice(
                selected = themeValue == "GREY",
                label = R.string.widget_settings_theme_grey,
                onClick = { onThemeChange("GREY") },
            )
            ThemeChoice(
                selected = themeValue == "LIGHT",
                label = R.string.widget_settings_theme_light,
                onClick = { onThemeChange("LIGHT") },
            )
        }

        Text(
            text = stringResource(R.string.widget_settings_transparency) + ": " +
                stringResource(R.string.widget_settings_percent, alphaPercent),
            style = MaterialTheme.typography.labelLarge,
            modifier = Modifier.padding(top = 6.dp),
        )
        Slider(
            value = alphaPercent.toFloat(),
            onValueChange = { onAlphaChange(it.roundToInt().coerceIn(0, 100)) },
            valueRange = 0f..100f,
        )
        Text(
            text = stringResource(R.string.widget_settings_opacity_tip),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )

        Text(
            text = stringResource(R.string.widget_settings_font_size) + ": " +
                stringResource(R.string.widget_settings_font_value, (fontScale * 100).roundToInt()),
            style = MaterialTheme.typography.labelLarge,
            modifier = Modifier.padding(top = 6.dp),
        )
        Slider(
            value = fontScale,
            onValueChange = { onFontScaleChange(it.coerceIn(WidgetPrefs.MIN_FONT_SCALE, WidgetPrefs.MAX_FONT_SCALE)) },
            valueRange = WidgetPrefs.MIN_FONT_SCALE..WidgetPrefs.MAX_FONT_SCALE,
        )
        Text(
            text = stringResource(R.string.widget_settings_font_tip),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun ThemeChoice(
    selected: Boolean,
    label: Int,
    onClick: () -> Unit,
) {
    FilterChip(
        selected = selected,
        onClick = onClick,
        label = { Text(stringResource(label)) },
    )
}
