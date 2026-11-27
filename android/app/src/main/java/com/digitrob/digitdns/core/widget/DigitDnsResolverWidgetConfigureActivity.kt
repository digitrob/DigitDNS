package com.digitrob.digitdns.core.widget

import android.appwidget.AppWidgetManager
import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
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
import com.digitrob.digitdns.ui.system.WidgetAppearanceEditor
import com.digitrob.digitdns.ui.theme.DigitDnsTheme

/**
 * The appearance editor the launcher opens when the widget is added, and the one the
 * widget's gear opens later. It writes the same per-widget preferences as Settings → App
 * → Widget Settings, so all three entry points stay consistent.
 */
class DigitDnsResolverWidgetConfigureActivity : ComponentActivity() {
    private var appWidgetId = AppWidgetManager.INVALID_APPWIDGET_ID

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setResult(RESULT_CANCELED)
        appWidgetId = intent?.extras?.getInt(
            AppWidgetManager.EXTRA_APPWIDGET_ID,
            AppWidgetManager.INVALID_APPWIDGET_ID,
        ) ?: AppWidgetManager.INVALID_APPWIDGET_ID
        if (appWidgetId == AppWidgetManager.INVALID_APPWIDGET_ID) {
            finish()
            return
        }

        val context = this@DigitDnsResolverWidgetConfigureActivity
        val initialTheme = WidgetPrefs.themeOverride(context, appWidgetId)
        val initialAlpha = WidgetPrefs.alpha(context, appWidgetId)
        val initialFont = WidgetPrefs.fontScale(context, appWidgetId)

        setContent {
            WidgetConfigureScreen(
                initialTheme = initialTheme,
                initialAlpha = initialAlpha,
                initialFontScale = initialFont,
                onCancel = { finish() },
                onSave = { theme, alphaPct, fontScale ->
                    WidgetPrefs.setThemeOverride(context, appWidgetId, theme)
                    WidgetPrefs.setAlpha(context, appWidgetId, alphaPct)
                    WidgetPrefs.setFontScale(context, appWidgetId, fontScale)
                    WidgetPrefs.requestRerender(context, intArrayOf(appWidgetId))
                    setResult(RESULT_OK, Intent().putExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, appWidgetId))
                    finish()
                },
            )
        }
    }
}

@Composable
private fun WidgetConfigureScreen(
    initialTheme: String?,
    initialAlpha: Int,
    initialFontScale: Float,
    onCancel: () -> Unit,
    onSave: (String?, Int, Float) -> Unit,
) {
    var themeValue by remember { mutableStateOf(initialTheme) }
    var alpha by remember { mutableIntStateOf(initialAlpha.coerceIn(0, 100)) }
    var fontScale by remember { mutableFloatStateOf(initialFontScale) }

    // The dialog previews the widget's own theme: switching a chip re-themes the
    // editor immediately, so the operator sees the look they are choosing.
    val context = LocalContext.current
    DigitDnsTheme(themeMode = WidgetPrefs.resolveModeFor(context, themeValue)) {
        Surface(
            modifier = Modifier.fillMaxSize(),
            color = MaterialTheme.colorScheme.background,
        ) {
            Column(
                modifier = Modifier
                    .fillMaxSize()
                    .verticalScroll(rememberScrollState())
                    .padding(16.dp),
            ) {
                Text(
                    text = stringResource(R.string.widget_config_title),
                    style = MaterialTheme.typography.titleMedium,
                )
                Spacer(Modifier.height(12.dp))
                WidgetAppearanceEditor(
                    themeValue = themeValue,
                    alphaPercent = alpha,
                    fontScale = fontScale,
                    onThemeChange = { themeValue = it },
                    onAlphaChange = { alpha = it },
                    onFontScaleChange = { fontScale = it },
                )
                Spacer(Modifier.height(16.dp))
                Button(
                    onClick = { onSave(themeValue, alpha, fontScale) },
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    Text(stringResource(R.string.action_save))
                }
                Spacer(Modifier.height(8.dp))
                OutlinedButton(onClick = onCancel, modifier = Modifier.fillMaxWidth()) {
                    Text(stringResource(android.R.string.cancel))
                }
            }
        }
    }
}
