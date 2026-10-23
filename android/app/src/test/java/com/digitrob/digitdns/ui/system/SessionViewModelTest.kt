package com.digitrob.digitdns.ui.system

import com.digitrob.digitdns.core.network.AuthState
import com.digitrob.digitdns.data.AuthRepository
import com.digitrob.digitdns.domain.model.Connection
import com.digitrob.digitdns.domain.model.CredentialMode
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class SessionViewModelTest {

    private val dispatcher = StandardTestDispatcher()
    private val activeConnection = MutableStateFlow<Connection?>(null)
    private val credentialRevision = MutableStateFlow(0)
    private val authCredentialRevision = MutableStateFlow(0)
    private lateinit var authRepository: AuthRepository
    private lateinit var authState: AuthState

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        authRepository = mockk(relaxed = true)
        every { authRepository.activeConnection } returns activeConnection
        every { authRepository.credentialRevision } returns credentialRevision
        authState = mockk(relaxed = true)
        every { authState.credentialRevision } returns authCredentialRevision
        every { authState.activeId } returns "id-1"
        every { authState.hasCredential } returns false
    }

    @After
    fun tearDown() {
        Dispatchers.resetMain()
    }

    private fun connection() = Connection(
        host = "docs.example.com",
        port = 27586,
        adminPath = "abc123de45",
        isHttps = true,
        credentialMode = CredentialMode.SESSION,
        id = "id-1",
    )

    @Test
    fun savingCredentialsRefreshesTheCardWithoutAConnectionChange() = runTest(dispatcher) {
        every { authRepository.hasSavedCredentials("id-1") } returns false
        activeConnection.value = connection()

        val viewModel = SessionViewModel(authRepository, authState)
        advanceUntilIdle()
        assertFalse(viewModel.state.value.credentialsSaved)

        every { authRepository.hasSavedCredentials("id-1") } returns true
        credentialRevision.value += 1
        advanceUntilIdle()

        assertTrue(viewModel.state.value.credentialsSaved)
    }

    @Test
    fun switchingConnectionReReadsTheNewServersSavedState() = runTest(dispatcher) {
        every { authRepository.hasSavedCredentials("id-1") } returns true
        every { authRepository.hasSavedCredentials("id-2") } returns false
        activeConnection.value = connection()

        val viewModel = SessionViewModel(authRepository, authState)
        advanceUntilIdle()
        assertTrue(viewModel.state.value.credentialsSaved)

        activeConnection.value = connection().copy(id = "id-2", host = "other.example.com")
        advanceUntilIdle()

        assertFalse(viewModel.state.value.credentialsSaved)
        assertTrue(viewModel.state.value.authority.startsWith("other.example.com"))
    }

    @Test
    fun aServerThatWasNeverAuthenticatedIsNotPresentedAsASession() = runTest(dispatcher) {
        every { authRepository.hasSavedCredentials("id-1") } returns false
        activeConnection.value = connection()
        every { authState.hasCredential } returns false

        val viewModel = SessionViewModel(authRepository, authState)
        advanceUntilIdle()
        assertFalse("no live credential means no session to show", viewModel.state.value.isAuthenticated)

        // Signing in flips it: the credential revision fires and the state re-reads.
        every { authState.hasCredential } returns true
        authCredentialRevision.value += 1
        advanceUntilIdle()
        assertTrue(viewModel.state.value.isAuthenticated)
    }
}
