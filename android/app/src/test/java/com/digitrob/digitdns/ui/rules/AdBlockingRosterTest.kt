package com.digitrob.digitdns.ui.rules

import com.digitrob.digitdns.core.network.FilterListDto
import com.digitrob.digitdns.core.network.FilterPresetDto
import com.digitrob.digitdns.core.network.FilteringStatusDto
import com.digitrob.digitdns.core.network.PresetCountDto
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** The one-roster projection: preset groups, installed matching and custom lists. */
class AdBlockingRosterTest {

    private val pro = FilterPresetDto(
        id = "hagezi-pro",
        name = "HaGeZi Pro",
        group = "base",
        url = "https://example.com/pro.txt",
        recommended = true,
    )
    private val light = FilterPresetDto(
        id = "hagezi-light",
        name = "HaGeZi Light",
        group = "base",
        url = "https://example.com/light.txt",
    )
    private val urlhaus = FilterPresetDto(
        id = "urlhaus",
        name = "URLhaus",
        group = "security",
        url = "https://example.com/urlhaus.txt",
    )

    private fun status(
        lists: List<FilterListDto> = emptyList(),
        counts: List<PresetCountDto> = emptyList(),
        countsRunning: Boolean = false,
        updating: List<String> = emptyList(),
    ) = FilteringStatusDto(
        lists = lists,
        presets = listOf(pro, light, urlhaus),
        presetCounts = counts,
        presetCountsRunning = countsRunning,
        updating = updating,
    )

    @Test
    fun `installed presets are merged into their catalogue rows`() {
        val rows = buildGroups(
            status(
                lists = listOf(
                    FilterListDto(
                        id = "l1",
                        name = "HaGeZi Pro",
                        presetId = "hagezi-pro",
                        enabled = true,
                        domainCount = 247757,
                    ),
                ),
            ),
            adding = emptySet(),
            search = "",
        )

        val base = rows.first { it.key == "base" }
        assertEquals(2, base.rows.size)
        val installed = base.rows.first { it.preset.id == "hagezi-pro" }
        assertTrue(installed.isInstalled)
        assertTrue(installed.isEnabled)
        assertEquals(247757, installed.domainCount)
        val notInstalled = base.rows.first { it.preset.id == "hagezi-light" }
        assertFalse(notInstalled.isInstalled)
        assertNull(notInstalled.domainCount)
    }

    @Test
    fun `an older list without preset id still matches by url`() {
        val rows = buildGroups(
            status(
                lists = listOf(
                    FilterListDto(
                        id = "l1",
                        name = "HaGeZi Pro",
                        url = "https://example.com/pro.txt",
                        enabled = false,
                        domainCount = 12,
                    ),
                ),
            ),
            adding = emptySet(),
            search = "",
        )

        val installed = rows.first { it.key == "base" }.rows.first { it.preset.id == "hagezi-pro" }
        assertTrue(installed.isInstalled)
        assertFalse(installed.isEnabled)
        assertEquals(12, installed.domainCount)
    }

    @Test
    fun `uninstalled presets use the cached domain count`() {
        val rows = buildGroups(
            status(counts = listOf(PresetCountDto(presetId = "hagezi-light", domainCount = 80509))),
            adding = emptySet(),
            search = "",
        )

        val lightRow = rows.first { it.key == "base" }.rows.first { it.preset.id == "hagezi-light" }
        assertEquals(80509, lightRow.domainCount)
        assertFalse(lightRow.counting)
        assertFalse(lightRow.countUnavailable)
    }

    @Test
    fun `a running scan marks uncached presets as counting`() {
        val rows = buildGroups(status(countsRunning = true), adding = emptySet(), search = "")

        val lightRow = rows.first { it.key == "base" }.rows.first { it.preset.id == "hagezi-light" }
        assertTrue(lightRow.counting)
        assertNull(lightRow.domainCount)
    }

    @Test
    fun `adding and updating flags follow the request`() {
        val rows = buildGroups(
            status(
                lists = listOf(
                    FilterListDto(id = "l1", name = "HaGeZi Pro", presetId = "hagezi-pro", enabled = true),
                ),
                updating = listOf("l1"),
            ),
            adding = setOf("hagezi-light"),
            search = "",
        )

        val base = rows.first { it.key == "base" }
        assertTrue(base.rows.first { it.preset.id == "hagezi-light" }.adding)
        assertTrue(base.rows.first { it.preset.id == "hagezi-pro" }.updating)
    }

    @Test
    fun `search filters both preset groups and custom lists`() {
        val data = status(
            lists = listOf(
                FilterListDto(id = "c1", name = "My list", url = "https://custom.example/list.txt", enabled = true),
                FilterListDto(id = "l1", name = "HaGeZi Pro", presetId = "hagezi-pro", enabled = true),
            ),
        )

        val rows = buildGroups(data, emptySet(), search = "urlhaus")
        assertEquals(listOf("security"), rows.map { it.key })
        assertEquals(1, rows.single().rows.size)

        assertEquals(1, buildCustomLists(data, search = "custom").size)
        assertEquals(0, buildCustomLists(data, search = "urlhaus").size)
    }

    @Test
    fun `custom lists exclude every preset-backed list`() {
        val data = status(
            lists = listOf(
                FilterListDto(id = "l1", name = "HaGeZi Pro", presetId = "hagezi-pro", enabled = true),
                FilterListDto(id = "c1", name = "My list", url = "https://custom.example/list.txt", enabled = true),
            ),
        )

        val custom = buildCustomLists(data, search = "")

        assertEquals(listOf("c1"), custom.map { it.id })
    }
}
