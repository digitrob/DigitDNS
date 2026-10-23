package com.digitrob.digitdns.ui.clients

import com.digitrob.digitdns.core.network.ClientCreateRequest
import com.digitrob.digitdns.core.network.ClientUpdateRequest
import com.digitrob.digitdns.data.ClientsRepository
import com.digitrob.digitdns.domain.model.Client
import com.digitrob.digitdns.domain.model.ClientExpiry
import com.digitrob.digitdns.domain.model.ClientLinks
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import java.time.Instant
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * The create/edit contract: the app's "blank means lifetime" and "+30 days default"
 * have to map onto the API's absolute expiry, including the explicit zero timestamp
 * that clears one.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class ClientDetailViewModelTest {

    private val dispatcher = StandardTestDispatcher()
    private lateinit var repository: ClientsRepository

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        repository = mockk(relaxed = true)
    }

    @After
    fun tearDown() {
        Dispatchers.resetMain()
    }

    private fun client(expiresAt: Instant? = null) = Client(
        id = "client-1",
        name = "Living room",
        uuid = "uuid-1",
        token = "token-1",
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
        registerSecret = "secret-1",
    )

    @Test
    fun resetPrefillsThirtyDaysAndStaysInCreateMode() {
        val viewModel = ClientDetailViewModel(repository)
        viewModel.resetForNewClient()

        val state = viewModel.state.value
        assertTrue(state.isCreate)
        assertNotNull(ClientLinks.parseExpiryField(state.expiryField))
        assertNull(state.created)
        assertFalse(state.saved)
    }

    @Test
    fun createSendsTheAbsoluteExpiryAndKeepsTheCredentialsOnScreen() = runTest(dispatcher) {
        val created = client(expiresAt = Instant.parse("2026-12-01T12:00:00Z"))
        val request = slot<ClientCreateRequest>()
        coEvery { repository.create(capture(request)) } returns created

        val viewModel = ClientDetailViewModel(repository)
        viewModel.resetForNewClient()
        viewModel.onNameChange("New")
        viewModel.save()
        testScheduler.advanceUntilIdle()

        assertEquals(
            ClientLinks.parseExpiryField(viewModel.state.value.expiryField).toString(),
            request.captured.expiresAt,
        )
        assertEquals(created, viewModel.state.value.created)
        // A successful create must not close the popup: the secret is only visible here.
        assertFalse(viewModel.state.value.saved)
    }

    @Test
    fun blankExpiryOnEditClearsTheStoredDate() = runTest(dispatcher) {
        coEvery { repository.get("client-1") } returns client(Instant.parse("2026-12-01T12:00:00Z"))
        val update = slot<ClientUpdateRequest>()
        coEvery { repository.update("client-1", capture(update)) } returns client()

        val viewModel = ClientDetailViewModel(repository)
        viewModel.open("client-1")
        testScheduler.advanceUntilIdle()
        viewModel.onExpiryChange("")
        viewModel.save()
        testScheduler.advanceUntilIdle()

        assertEquals(ClientExpiry.NEVER, update.captured.expiresAt)
        coVerify(exactly = 1) { repository.update("client-1", any()) }
    }
}
