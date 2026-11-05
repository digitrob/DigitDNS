package com.digitrob.digitdns.data

import org.junit.Assert.assertEquals
import org.junit.Test

class ThemeModeTest {

    @Test
    fun greyRoundTripsThroughStorage() {
        assertEquals(ThemeMode.GREY, ThemeMode.fromStored("GREY"))
    }

    @Test
    fun unknownOrMissingStoredValueFallsBackToSystem() {
        assertEquals(ThemeMode.SYSTEM, ThemeMode.fromStored(null))
        assertEquals(ThemeMode.SYSTEM, ThemeMode.fromStored("SEPIA"))
    }

    @Test
    fun systemResolvesToTheOsTheme() {
        assertEquals(ThemeMode.DARK, ThemeMode.SYSTEM.effective(systemDark = true))
        assertEquals(ThemeMode.LIGHT, ThemeMode.SYSTEM.effective(systemDark = false))
    }

    @Test
    fun explicitModesIgnoreTheSystem() {
        assertEquals(ThemeMode.GREY, ThemeMode.GREY.effective(systemDark = false))
        assertEquals(ThemeMode.LIGHT, ThemeMode.LIGHT.effective(systemDark = true))
    }

    @Test
    fun loginCycleIsDarkGreyLightDark() {
        assertEquals(ThemeMode.GREY, ThemeMode.DARK.next(systemDark = false))
        assertEquals(ThemeMode.LIGHT, ThemeMode.GREY.next(systemDark = true))
        assertEquals(ThemeMode.DARK, ThemeMode.LIGHT.next(systemDark = false))
    }

    @Test
    fun systemStartsFromTheVisibleTheme() {
        assertEquals(ThemeMode.GREY, ThemeMode.SYSTEM.next(systemDark = true))
        assertEquals(ThemeMode.DARK, ThemeMode.SYSTEM.next(systemDark = false))
    }
}
