package com.digitrob.digitdns.ui.clients

import com.digitrob.digitdns.core.network.ClientUpdateRequest
import com.digitrob.digitdns.data.AccessRepository
import com.digitrob.digitdns.data.ClientSnapshot
import com.digitrob.digitdns.data.ClientsRepository
import com.digitrob.digitdns.data.ConfigRepository
import com.digitrob.digitdns.data.ConnectionStore
import com.digitrob.digitdns.domain.model.Client
import com.digitrob.digitdns.domain.model.ClientExpiry
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.delay
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.io.IOException
import java.time.Instant

@OptIn(ExperimentalCoroutinesApi::class)
class ClientsViewModelTest {

    private val dispatcher = StandardTestDispatcher()
    private lateinit var clientsRepository: ClientsRepository
    private lateinit var configRepository: ConfigRepository
    private lateinit var connectionStore: ConnectionStore
    private lateinit var accessRepository: AccessRepository

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        clientsRepository = mockk()
        configRepository = mockk()
        connectionStore = mockk(relaxed = true)
        accessRepository = mockk(relaxed = true)
        // The link origin is a convenience: the list must still load when config does
        // not answer, which is the same fallback a pre-3.1 daemon needs.
        coEvery { configRepository.config() } throws IOException("no config")
    }

    @After
    fun tearDown() {
        Dispatchers.resetMain()
    }

    private fun client(expiresAt: Instant? = Instant.parse("2030-01-01T00:00:00Z")) = Client(
        id = "client-1",
        name = "Laptop",
        uuid = "11111111-1111-1111-1111-111111111111",
        token = "token",
        allowedIps = emptyList(),
        trafficLimitGb = 0.0,
        trafficUsedBytes = 0,
        trafficResetCycle = "",
        quotaExceeded = false,
        expiresAt = expiresAt,
        enabled = true,
        note = "",
        customPolicies = emptyList(),
        maxDevices = 0,
        registerSecret = "",
    )

    @Test
    fun backgroundRefreshDoesNotRaiseThePullIndicator() = runTest(dispatcher) {
        coEvery { clientsRepository.listAll() } returns ClientSnapshot(listOf(client()), false)
        val viewModel = ClientsViewModel(clientsRepository, configRepository, connectionStore, accessRepository)
        advanceUntilIdle()

        viewModel.refresh()
        runCurrent()

        assertFalse(viewModel.state.value.isRefreshing)
        assertFalse(viewModel.state.value.isLoading)
        advanceUntilIdle()
    }

    @Test
    fun userPullRaisesTheIndicatorUntilItFinishes() = runTest(dispatcher) {
        var calls = 0
        coEvery { clientsRepository.listAll() } coAnswers {
            calls += 1
            if (calls > 1) delay(1_000)
            ClientSnapshot(listOf(client()), false)
        }
        val viewModel = ClientsViewModel(clientsRepository, configRepository, connectionStore, accessRepository)
        advanceUntilIdle()

        viewModel.refresh(userInitiated = true)
        runCurrent()
        assertTrue(viewModel.state.value.isRefreshing)

        advanceUntilIdle()
        assertFalse(viewModel.state.value.isRefreshing)
    }

    @Test
    fun lifetimeActionSendsTheZeroTimestampAndReportsSuccess() = runTest(dispatcher) {
        val account = client()
        coEvery { clientsRepository.listAll() } returns ClientSnapshot(listOf(account), false)
        coEvery { clientsRepository.update(any(), any()) } returns account
        val viewModel = ClientsViewModel(clientsRepository, configRepository, connectionStore, accessRepository)
        advanceUntilIdle()

        viewModel.makeLifetime(account)
        advanceUntilIdle()

        coVerify {
            clientsRepository.update(
                "client-1",
                match<ClientUpdateRequest> { it.expiresAt == ClientExpiry.NEVER },
            )
        }
        assertNull(viewModel.state.value.busyId)
        assertTrue(viewModel.state.value.message?.contains("expires never") == true)
    }

    @Test
    fun lifetimeIsNotOfferedForAClientWithoutAnExpiry() = runTest(dispatcher) {
        val account = client(expiresAt = null)
        coEvery { clientsRepository.listAll() } returns ClientSnapshot(listOf(account), false)
        val viewModel = ClientsViewModel(clientsRepository, configRepository, connectionStore, accessRepository)
        advanceUntilIdle()

        viewModel.makeLifetime(account)
        advanceUntilIdle()

        coVerify(exactly = 0) { clientsRepository.update(any(), any()) }
        assertEquals(null, viewModel.state.value.busyId)
    }
}
