package com.digitrob.digitdns.baselineprofile

import androidx.benchmark.macro.junit4.BaselineProfileRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.uiautomator.By
import androidx.test.uiautomator.Until
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Generates the Baseline Profile from the app's real startup path.
 *
 * The profile is collected from a cold start through the first rendered screen.
 * The wait is advisory: an install with no saved server lands on the login page,
 * which is still a valid startup path and still covered by the profile.
 */
@RunWith(AndroidJUnit4::class)
class BaselineProfileGenerator {

    @get:Rule
    val rule = BaselineProfileRule()

    @Test
    fun startup() {
        rule.collect(
            packageName = "com.digitrob.digitdns",
            includeInStartupProfile = true,
        ) {
            startActivityAndWait()
            device.wait(Until.hasObject(By.text("Dashboard")), 8_000)
            device.waitForIdle()
        }
    }
}
