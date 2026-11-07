package com.digitrob.digitdns.ui.system

import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.data.SettingsBundle
import com.digitrob.digitdns.data.SettingsRepository
import com.digitrob.digitdns.domain.model.ServerConfig
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
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class Ipv6SettingsViewModelTest {

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

    private fun server(publicIpv6: String, enabled: Boolean) = ServerConfig(
        publicIp = "203.0.113.9",
        bindHost = "0.0.0.0",
        webPort = 8080,
        adminUsername = "admin",
        adminPasswordWeak = false,
        apiKey = "key",
        apiBind = "127.0.0.1",
        apiBindList = emptyList(),
        sessionIdleMinutes = 15,
        adminPath = "a1b2c3d4e5f67890",
        publicIpv6 = publicIpv6,
        proxyIpv6Enabled = enabled,
    )

    @Test
    fun loadMapsTheLivePolicy() = runTest(dispatcher) {
        coEvery { repository.load() } returns SettingsBundle(
            server = server("2001:db8::1", true),
            tls = null,
        )

        val viewModel = Ipv6SettingsViewModel(repository)
        advanceUntilIdle()

        val state = viewModel.state.value
        assertTrue(state.enabled)
        assertEquals("2001:db8::1", state.address)
        assertTrue(state.loaded)
        assertFalse(state.isLoading)
    }

    @Test
    fun saveSendsTheAddressAndSwitch() = runTest(dispatcher) {
        coEvery { repository.load() } returns SettingsBundle(server = server("", false), tls = null)
        coEvery { repository.saveIpv6(true, "2001:db8::1") } returns server("2001:db8::1", true)

        val viewModel = Ipv6SettingsViewModel(repository)
        advanceUntilIdle()
        viewModel.onAddressChange("2001:db8::1")
        viewModel.onEnabledChange(true)
        viewModel.save()
        advanceUntilIdle()

        coVerify { repository.saveIpv6(true, "2001:db8::1") }
        assertTrue(viewModel.state.value.enabled)
        assertEquals("2001:db8::1", viewModel.state.value.address)
        assertTrue(viewModel.state.value.saved)
    }

    @Test
    fun enablingWithoutAnAddressIsRejectedLocally() = runTest(dispatcher) {
        coEvery { repository.load() } returns SettingsBundle(server = server("", false), tls = null)

        val viewModel = Ipv6SettingsViewModel(repository)
        advanceUntilIdle()
        viewModel.onEnabledChange(true)
        viewModel.save()
        advanceUntilIdle()

        coVerify(exactly = 0) { repository.saveIpv6(any(), any()) }
        assertTrue(viewModel.state.value.error?.contains("IPv6") == true)
    }

    @Test
    fun aNonIPv6AddressIsRejectedLocally() = runTest(dispatcher) {
        coEvery { repository.load() } returns SettingsBundle(server = server("", false), tls = null)

        val viewModel = Ipv6SettingsViewModel(repository)
        advanceUntilIdle()
        viewModel.onAddressChange("example.com")
        viewModel.onEnabledChange(true)
        viewModel.save()
        advanceUntilIdle()

        coVerify(exactly = 0) { repository.saveIpv6(any(), any()) }
        assertEquals("Enter a valid public IPv6 address.", viewModel.state.value.error)
    }

    @Test
    fun anOlderServerShowsTheUnsupportedState() = runTest(dispatcher) {
        coEvery { repository.load() } throws ApiException(404, null, "not found")

        val viewModel = Ipv6SettingsViewModel(repository)
        advanceUntilIdle()

        assertTrue(viewModel.state.value.unsupported)
        assertFalse(viewModel.state.value.isLoading)
        assertTrue(viewModel.state.value.loaded)
    }
}
