package com.digitrob.digitdns.core.branding

import android.content.ComponentName
import android.content.Context
import android.content.pm.PackageManager
import com.digitrob.digitdns.data.IconStyle
import timber.log.Timber

/**
 * Switches the home-screen icon between the two activity aliases declared in the
 * manifest. The application icon itself stays the colour mark (settings, recents
 * and the splash screen); only the launcher component changes.
 *
 * The desired alias is enabled before the other is disabled, and DONT_KILL_APP
 * keeps the running process alive. Some launchers cache the icon for a moment or
 * until the next restart; that is a launcher behaviour, not an app failure.
 */
object LauncherIcon {
    private const val COLOR_ALIAS = ".LauncherColor"
    private const val GRAYSCALE_ALIAS = ".LauncherGrayscale"

    fun apply(context: Context, style: IconStyle) {
        val pkg = context.packageName
        val pm = context.packageManager
        val color = ComponentName(pkg, pkg + COLOR_ALIAS)
        val grayscale = ComponentName(pkg, pkg + GRAYSCALE_ALIAS)
        val wantColor = style == IconStyle.COLOR

        setEnabled(pm, if (wantColor) color else grayscale, true)
        setEnabled(pm, if (wantColor) grayscale else color, false)
    }

    private fun setEnabled(pm: PackageManager, component: ComponentName, enabled: Boolean) {
        val desired = if (enabled) {
            PackageManager.COMPONENT_ENABLED_STATE_ENABLED
        } else {
            PackageManager.COMPONENT_ENABLED_STATE_DISABLED
        }
        if (pm.getComponentEnabledSetting(component) == desired) return
        try {
            pm.setComponentEnabledSetting(
                component,
                desired,
                PackageManager.DONT_KILL_APP,
            )
        } catch (e: IllegalArgumentException) {
            Timber.w(e, "Launcher alias %s is missing", component.className)
        } catch (e: SecurityException) {
            Timber.w(e, "Could not change launcher alias %s", component.className)
        }
    }
}
