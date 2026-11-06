package com.digitrob.digitdns.ui.system

import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.ProxyLeakSettings
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
class ProxyLeakSettingsViewModelTest {

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
    fun loadMapsTheLivePolicy() = runTest(dispatcher) {
        coEvery { repository.loadProxyLeak() } returns ProxyLeakSettings(
            enabled = false,
            recommended = true,
        )

        val viewModel = ProxyLeakSettingsViewModel(repository)
        advanceUntilIdle()

        val state = viewModel.state.value
        assertFalse(state.enabled)
        assertTrue(state.recommended)
        assertFalse(state.isLoading)
        assertTrue(state.loaded)
    }

    @Test
    fun setEnabledSavesImmediatelyAndUsesTheServerAnswer() = runTest(dispatcher) {
        coEvery { repository.loadProxyLeak() } returns ProxyLeakSettings(enabled = true)
        coEvery { repository.saveProxyLeak(false) } returns ProxyLeakSettings(
            enabled = false,
            recommended = true,
        )

        val viewModel = ProxyLeakSettingsViewModel(repository)
        advanceUntilIdle()
        viewModel.setEnabled(false)
        advanceUntilIdle()

        coVerify { repository.saveProxyLeak(false) }
        assertFalse(viewModel.state.value.enabled)
        assertFalse(viewModel.state.value.isSaving)
        assertTrue(viewModel.state.value.saved)
    }

    @Test
    fun aFailedSaveRestoresThePreviousValue() = runTest(dispatcher) {
        coEvery { repository.loadProxyLeak() } returns ProxyLeakSettings(enabled = true)
        coEvery { repository.saveProxyLeak(false) } throws ApiException(500, null, "boom")

        val viewModel = ProxyLeakSettingsViewModel(repository)
        advanceUntilIdle()
        viewModel.setEnabled(false)
        advanceUntilIdle()

        assertTrue(viewModel.state.value.enabled)
        assertFalse(viewModel.state.value.isSaving)
        assertEquals("boom", viewModel.state.value.error)
    }

    @Test
    fun aPullToRefreshDoesNotUseTheInitialLoadingFlag() = runTest(dispatcher) {
        coEvery { repository.loadProxyLeak() } returns ProxyLeakSettings(enabled = true)

        val viewModel = ProxyLeakSettingsViewModel(repository)
        advanceUntilIdle()
        assertTrue(viewModel.state.value.loaded)

        val gate = CompletableDeferred<ProxyLeakSettings>()
        coEvery { repository.loadProxyLeak() } coAnswers { gate.await() }
        viewModel.load()
        runCurrent()

        val during = viewModel.state.value
        assertTrue(during.isRefreshing)
        assertFalse("a refresh must not look like the first load", during.isLoading)

        gate.complete(ProxyLeakSettings(enabled = true))
        advanceUntilIdle()
        assertFalse(viewModel.state.value.isRefreshing)
        assertFalse(viewModel.state.value.isLoading)
    }

    @Test
    fun anOlderServerShowsTheUnsupportedState() = runTest(dispatcher) {
        coEvery { repository.loadProxyLeak() } throws ApiException(404, null, "not found")

        val viewModel = ProxyLeakSettingsViewModel(repository)
        advanceUntilIdle()

        assertTrue(viewModel.state.value.unsupported)
        assertFalse(viewModel.state.value.isLoading)
        assertTrue(viewModel.state.value.loaded)
    }
}
