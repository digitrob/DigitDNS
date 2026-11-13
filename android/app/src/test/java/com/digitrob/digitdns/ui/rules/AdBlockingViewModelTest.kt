package com.digitrob.digitdns.ui.rules

import com.digitrob.digitdns.core.network.FilterListDto
import com.digitrob.digitdns.core.network.FilterPresetDto
import android.content.Context
import com.digitrob.digitdns.core.network.FilteringSettingsDto
import com.digitrob.digitdns.core.network.FilteringStatusDto
import com.digitrob.digitdns.core.network.PresetAddResultDto
import com.digitrob.digitdns.data.FilteringRepository
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class AdBlockingViewModelTest {

    private val dispatcher = StandardTestDispatcher()
    private lateinit var repository: FilteringRepository

    private val pro = FilterPresetDto(
        id = "hagezi-pro",
        name = "HaGeZi Pro",
        group = "base",
        url = "https://example.com/pro.txt",
    )
    private val light = FilterPresetDto(
        id = "hagezi-light",
        name = "HaGeZi Light",
        group = "base",
        url = "https://example.com/light.txt",
    )

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        repository = mockk(relaxed = true)
    }

    @After
    fun tearDown() = Dispatchers.resetMain()

    private fun status(
        lists: List<FilterListDto> = emptyList(),
        settings: FilteringSettingsDto = FilteringSettingsDto(),
    ) = FilteringStatusDto(
        settings = settings,
        lists = lists,
        presets = listOf(pro, light),
        totalDomains = lists.sumOf { it.domainCount },
        totalRules = lists.sumOf { it.ruleCount },
    )

    @Test
    fun `init loads the roster and settings`() = runTest(dispatcher) {
        coEvery { repository.status() } returns status(
            lists = listOf(
                FilterListDto(
                    id = "l1",
                    name = "HaGeZi Pro",
                    presetId = "hagezi-pro",
                    enabled = true,
                    domainCount = 100,
                ),
            ),
            settings = FilteringSettingsDto(enabled = true, blockMode = "nxdomain", blockTtl = 30),
        )

        val viewModel = AdBlockingViewModel(repository, mockk(relaxed = true))
        advanceUntilIdle()

        val state = viewModel.state.value
        assertEquals("nxdomain", state.blockMode)
        assertEquals("30", state.blockTtl)
        assertEquals(1, state.activeCount)
        assertEquals(100, state.totalDomains)
        assertTrue(state.groups.first().rows.first { it.preset.id == "hagezi-pro" }.isInstalled)
    }

    @Test
    fun `ticking an uninstalled preset adds it and refreshes the row`() = runTest(dispatcher) {
        val added = FilterListDto(
            id = "l2",
            name = "HaGeZi Light",
            presetId = "hagezi-light",
            enabled = true,
            domainCount = 50,
        )
        coEvery { repository.status() } returnsMany listOf(status(), status(lists = listOf(added)))
        coEvery { repository.addPreset("hagezi-light") } returns PresetAddResultDto()

        val viewModel = AdBlockingViewModel(repository, mockk(relaxed = true))
        advanceUntilIdle()
        viewModel.addPreset("hagezi-light")
        advanceUntilIdle()

        coVerify(exactly = 1) { repository.addPreset("hagezi-light") }
        val row = viewModel.state.value.groups.first().rows.first { it.preset.id == "hagezi-light" }
        assertTrue(row.isInstalled)
        assertTrue(row.isEnabled)
        assertEquals(50, row.domainCount)
    }

    @Test
    fun `unticing an installed list disables it`() = runTest(dispatcher) {
        val enabledList = FilterListDto(
            id = "l1",
            name = "HaGeZi Pro",
            presetId = "hagezi-pro",
            enabled = true,
            domainCount = 100,
        )
        val disabledList = enabledList.copy(enabled = false)
        coEvery { repository.status() } returnsMany listOf(
            status(lists = listOf(enabledList)),
            status(lists = listOf(disabledList)),
        )

        val viewModel = AdBlockingViewModel(repository, mockk(relaxed = true))
        advanceUntilIdle()
        viewModel.toggleList(enabledList, false)
        advanceUntilIdle()

        coVerify(exactly = 1) { repository.setEnabled("l1", false) }
        assertFalse(viewModel.state.value.groups.first().rows.first { it.preset.id == "hagezi-pro" }.isEnabled)
    }

    @Test
    fun `invalid TTL is rejected before the network call`() = runTest(dispatcher) {
        coEvery { repository.status() } returns status()

        val viewModel = AdBlockingViewModel(repository, mockk(relaxed = true))
        advanceUntilIdle()
        viewModel.setBlockTtl("0")
        viewModel.saveSettings()
        advanceUntilIdle()

        coVerify(exactly = 0) { repository.saveSettings(any()) }
        assertNotNull(viewModel.state.value.error)
    }
}
