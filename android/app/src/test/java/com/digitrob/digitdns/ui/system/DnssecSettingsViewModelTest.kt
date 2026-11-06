package com.digitrob.digitdns.ui.system

import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.DnssecSettings
import com.digitrob.digitdns.data.SettingsRepository
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class DnssecSettingsViewModelTest {

    private val dispatcher = StandardTestDispatcher()
    private lateinit var repository: SettingsRepository

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        repository = mockk()
    }

    @After
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun loadMapsTheSavedPolicyAndRunningState() = runTest(dispatcher) {
        coEvery { repository.loadDnssec() } returns DnssecSettings(
            enabled = true,
            trustAnchors = listOf(". 172800 IN DS 20326 8 2 ABCD"),
            defaultAnchors = listOf(". 172800 IN DS 38696 8 2 EFGH"),
            running = true,
            restartRequired = false,
        )

        val viewModel = DnssecSettingsViewModel(repository)
        advanceUntilIdle()

        val state = viewModel.state.value
        assertTrue(state.enabled)
        assertTrue(state.running)
        assertFalse(state.restartRequired)
        assertEquals(". 172800 IN DS 20326 8 2 ABCD", state.anchors)
        assertEquals(listOf(". 172800 IN DS 38696 8 2 EFGH"), state.defaultAnchors)
        assertFalse(state.isLoading)
        assertTrue(state.loaded)
    }

    @Test
    fun saveSplitsLinesAndReportsThePendingRestart() = runTest(dispatcher) {
        coEvery { repository.loadDnssec() } returns DnssecSettings()
        coEvery { repository.saveDnssec(any(), any()) } returns DnssecSettings(
            enabled = true,
            running = false,
            restartRequired = true,
        )

        val viewModel = DnssecSettingsViewModel(repository)
        advanceUntilIdle()
        viewModel.onEnabledChange(true)
        viewModel.onAnchorsChange("  . 172800 IN DS 20326 8 2 ABCD  \n\n. 172800 IN DS 38696 8 2 EFGH ")
        viewModel.save()
        advanceUntilIdle()

        coVerify {
            repository.saveDnssec(
                true,
                listOf(". 172800 IN DS 20326 8 2 ABCD", ". 172800 IN DS 38696 8 2 EFGH"),
            )
        }
        assertTrue(viewModel.state.value.saved)
        assertTrue(viewModel.state.value.restartRequired)
    }

    @Test
    fun aPullToRefreshDoesNotUseTheInitialLoadingFlag() = runTest(dispatcher) {
        coEvery { repository.loadDnssec() } returns DnssecSettings(enabled = true)

        val viewModel = DnssecSettingsViewModel(repository)
        advanceUntilIdle()
        assertTrue(viewModel.state.value.loaded)

        val gate = CompletableDeferred<DnssecSettings>()
        coEvery { repository.loadDnssec() } coAnswers { gate.await() }
        viewModel.load()
        runCurrent()

        val during = viewModel.state.value
        assertTrue(during.isRefreshing)
        assertFalse("a refresh must not disable the Save button via isLoading", during.isLoading)

        gate.complete(DnssecSettings(enabled = true))
        advanceUntilIdle()
        assertFalse(viewModel.state.value.isRefreshing)
        assertFalse(viewModel.state.value.isLoading)
    }

    @Test
    fun anOlderServerShowsTheUnsupportedState() = runTest(dispatcher) {
        coEvery { repository.loadDnssec() } throws ApiException(404, null, "not found")

        val viewModel = DnssecSettingsViewModel(repository)
        advanceUntilIdle()

        assertTrue(viewModel.state.value.unsupported)
        assertFalse(viewModel.state.value.isLoading)
        assertTrue(viewModel.state.value.loaded)
    }
}
