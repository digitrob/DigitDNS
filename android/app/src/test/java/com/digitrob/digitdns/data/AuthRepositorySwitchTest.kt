package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.AuthApi
import com.digitrob.digitdns.core.network.AuthState
import com.digitrob.digitdns.core.network.ServerUrlProvider
import com.digitrob.digitdns.core.network.SessionRefresher
import com.digitrob.digitdns.core.security.SecretStore
import com.digitrob.digitdns.domain.model.Connection
import com.digitrob.digitdns.domain.model.CredentialMode
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class AuthRepositorySwitchTest {

    private val active = MutableStateFlow<Connection?>(null)
    private val authApi: AuthApi = mockk(relaxed = true)
    private val authState: AuthState = mockk(relaxed = true)
    private val connectionStore: ConnectionStore = mockk(relaxed = true)
    private val urlProvider: ServerUrlProvider = mockk(relaxed = true)
    private val secretStore: SecretStore = mockk(relaxed = true)
    private val sessionRefresher: SessionRefresher = mockk(relaxed = true)

    private fun connection(id: String) = Connection(
        host = "docs.example.com",
        port = 27586,
        adminPath = "abc123de45",
        isHttps = true,
        credentialMode = CredentialMode.SESSION,
        id = id,
    )

    private fun repository() = AuthRepository(
        authApi = authApi,
        authState = authState,
        connectionStore = connectionStore,
        urlProvider = urlProvider,
        secretStore = secretStore,
        sessionRefresher = sessionRefresher,
    )

    @Test
    fun switchingToARemovedServerFailsInsteadOfProbingThePreviousOne() = runTest {
        active.value = connection("previous")
        every { connectionStore.connection } returns active
        coEvery { connectionStore.setActive("missing") } returns Unit
        every { authState.hasCredential } returns true

        val result = repository().switchToServer("missing")

        assertTrue(result is ServerSwitchResult.Failed)
        assertEquals("That server is no longer saved.", (result as ServerSwitchResult.Failed).message)
        // The previous server is still active; probing it would report a bogus Ready.
        coVerify(exactly = 0) { authApi.me() }
    }
}
