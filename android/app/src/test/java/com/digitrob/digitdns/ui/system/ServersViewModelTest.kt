package com.digitrob.digitdns.ui.system

import app.cash.turbine.test
import com.digitrob.digitdns.data.AppMode
import com.digitrob.digitdns.data.AppPreferences
import com.digitrob.digitdns.data.AuthRepository
import com.digitrob.digitdns.data.ServerSwitchResult
import com.digitrob.digitdns.data.SubscriberSession
import com.digitrob.digitdns.domain.model.Connection
import com.digitrob.digitdns.domain.model.CredentialMode
import com.digitrob.digitdns.domain.model.SubscriberLink
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
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
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * The rename path: the only row mutation that must not disturb the rest of the
 * profile, which is why it never sets the busy state that blocks switching.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class ServersViewModelTest {

    private val dispatcher = StandardTestDispatcher()
    private lateinit var authRepository: AuthRepository
    private lateinit var subscriberSession: SubscriberSession
    private lateinit var appPreferences: AppPreferences

    @Before
    fun setUp() {
        // viewModelScope binds Dispatchers.Main, which does not exist on the JVM;
        // the test dispatcher stands in so refresh()'s launches run under runTest.
        Dispatchers.setMain(dispatcher)
        authRepository = mockk(relaxed = true)
        // Both are relaxed mocks: these tests exercise the admin list, and the
        // subscription row stays null unless a test says otherwise.
        subscriberSession = mockk(relaxed = true)
        appPreferences = mockk(relaxed = true)
    }

    @After
    fun tearDown() {
        Dispatchers.resetMain()
    }

    private fun activeConnection(label: String = "") = Connection(
        host = "docs.example.com",
        port = 27586,
        adminPath = "abc123de45",
        isHttps = true,
        credentialMode = CredentialMode.SESSION,
        label = label,
        id = "id-1",
    )

    @Test
    fun renamePersistsTheLabelAndRefreshesTheList() = runTest(dispatcher) {
        coEvery { authRepository.currentConnection() } returns activeConnection("HQ")
        coEvery { authRepository.listServers() } returnsMany listOf(
            listOf(activeConnection("HQ")),
            listOf(activeConnection("Norwich")),
        )

        val viewModel = ServersViewModel(authRepository, subscriberSession, appPreferences)
        testScheduler.advanceUntilIdle() // the init refresh completes here
        viewModel.state.test {
            // StateFlows conflate intermediate refresh states, so the semantics live
            // in the end state, not in a count of emissions.
            assertEquals("HQ", awaitItem().servers.single().label)
            viewModel.rename("id-1", " Norwich ")
            testScheduler.advanceUntilIdle()
            coVerify(exactly = 1) { authRepository.renameServer("id-1", " Norwich ") }
            var renamed = awaitItem()
            // The switch clears on its own emission path, so wait for the state that
            // is both populated and settled before comparing labels.
            while (renamed.isRefreshing || renamed.servers.isEmpty() || renamed.servers.single().isOnline == null) renamed = awaitItem()
            assertEquals("Norwich", renamed.servers.single().label)
        }
    }

    @Test
    fun rowLabelFallsBackToTheHostWhenTheLabelIsBlank() = runTest(dispatcher) {
        coEvery { authRepository.currentConnection() } returns activeConnection()
        coEvery { authRepository.listServers() } returns listOf(activeConnection())

        val viewModel = ServersViewModel(authRepository, subscriberSession, appPreferences)
        testScheduler.advanceUntilIdle()
        viewModel.state.test {
            assertEquals("docs.example.com", awaitItem().servers.single().label)
        }
    }

    @Test
    fun backgroundRefreshDoesNotRaiseThePullIndicator() = runTest(dispatcher) {
        coEvery { authRepository.currentConnection() } returns activeConnection()
        coEvery { authRepository.listServers() } returns listOf(activeConnection())

        val viewModel = ServersViewModel(authRepository, subscriberSession, appPreferences)
        advanceUntilIdle()
        viewModel.refresh()
        runCurrent()

        assertFalse(viewModel.state.value.isRefreshing)
        advanceUntilIdle()
    }

    @Test
    fun userPullRaisesTheIndicatorUntilItFinishes() = runTest(dispatcher) {
        var calls = 0
        coEvery { authRepository.currentConnection() } returns activeConnection()
        coEvery { authRepository.listServers() } coAnswers {
            calls += 1
            if (calls > 1) delay(1_000)
            listOf(activeConnection())
        }

        val viewModel = ServersViewModel(authRepository, subscriberSession, appPreferences)
        advanceUntilIdle()
        viewModel.refresh(userInitiated = true)
        runCurrent()
        assertTrue(viewModel.state.value.isRefreshing)

        advanceUntilIdle()
        assertFalse(viewModel.state.value.isRefreshing)
    }

    @Test
    fun switchToReadyClosesThePicker() = runTest(dispatcher) {
        coEvery { authRepository.currentConnection() } returns activeConnection()
        coEvery { authRepository.listServers() } returns listOf(activeConnection())
        coEvery { authRepository.switchToServer("id-1") } returns ServerSwitchResult.Ready

        val viewModel = ServersViewModel(authRepository, subscriberSession, appPreferences)
        advanceUntilIdle()
        viewModel.switchTo("id-1")
        advanceUntilIdle()

        assertEquals(ServerSwitchOutcome.Ready, viewModel.state.value.switchOutcome)
        assertEquals(null, viewModel.state.value.error)
        viewModel.consumeSwitchOutcome()
        assertEquals(null, viewModel.state.value.switchOutcome)
    }

    @Test
    fun switchToLoginRequiredAsksTheHostForTheLoginScreen() = runTest(dispatcher) {
        coEvery { authRepository.currentConnection() } returns activeConnection()
        coEvery { authRepository.listServers() } returns listOf(activeConnection())
        coEvery { authRepository.switchToServer("id-1") } returns ServerSwitchResult.LoginRequired

        val viewModel = ServersViewModel(authRepository, subscriberSession, appPreferences)
        advanceUntilIdle()
        viewModel.switchTo("id-1")
        advanceUntilIdle()

        assertEquals(ServerSwitchOutcome.LoginRequired, viewModel.state.value.switchOutcome)
    }

    @Test
    fun transportFailureStaysInTheListAsAnError() = runTest(dispatcher) {
        coEvery { authRepository.currentConnection() } returns activeConnection()
        coEvery { authRepository.listServers() } returns listOf(activeConnection())
        coEvery { authRepository.switchToServer("id-1") } returns
            ServerSwitchResult.Failed("Host not found. Check the address.")

        val viewModel = ServersViewModel(authRepository, subscriberSession, appPreferences)
        advanceUntilIdle()
        viewModel.switchTo("id-1")
        advanceUntilIdle()

        assertEquals(null, viewModel.state.value.switchOutcome)
        assertEquals("Host not found. Check the address.", viewModel.state.value.error)
    }

    @Test
    fun aSubscriberSessionAppearsAboveTheSavedServers() = runTest(dispatcher) {
        val link = SubscriberLink.parse("https://norwich.example:27586/sub/abcdef1234567890")
        assertNotNull(link)
        every { subscriberSession.currentLink() } returns link
        every { subscriberSession.currentLabel() } returns "Norwich UI probe"
        coEvery { authRepository.currentConnection() } returns activeConnection()
        coEvery { authRepository.listServers() } returns listOf(activeConnection())

        val viewModel = ServersViewModel(authRepository, subscriberSession, appPreferences)
        advanceUntilIdle()

        val row = viewModel.state.value.subscription
        assertNotNull(row)
        assertEquals("Norwich UI probe", row?.label)
        assertEquals("norwich.example:27586", row?.authority)
        assertEquals("https://norwich.example:27586/sub/abcdef…7890", row?.maskedUrl)
    }

    @Test
    fun openingTheSubscriptionSwitchesToSubscriberMode() = runTest(dispatcher) {
        every { subscriberSession.currentLink() } returns
            SubscriberLink.parse("https://norwich.example:27586/sub/abcdef1234567890")
        every { subscriberSession.currentLabel() } returns "Norwich UI probe"
        coEvery { authRepository.currentConnection() } returns activeConnection()
        coEvery { authRepository.listServers() } returns emptyList()

        val viewModel = ServersViewModel(authRepository, subscriberSession, appPreferences)
        advanceUntilIdle()
        viewModel.openSubscriber()
        advanceUntilIdle()

        assertTrue(viewModel.state.value.openSubscriber)
        coVerify { appPreferences.setAppMode(AppMode.SUBSCRIBER) }
        coVerify { appPreferences.setSubscriberActive(true) }

        viewModel.consumeOpenSubscriber()
        assertFalse(viewModel.state.value.openSubscriber)
    }
}
