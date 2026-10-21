package com.digitrob.digitdns.core.network

import com.digitrob.digitdns.core.security.SecretStore
import com.digitrob.digitdns.domain.model.CredentialMode
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import io.mockk.verify
import kotlinx.coroutines.test.runTest
import javax.inject.Provider
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The silent re-login path. It has three jobs and each has a failure mode worth
 * pinning: decide whether a refresh is even possible, replay the saved pair for a
 * fresh token, and never mint a second session while one is still warm.
 */
class SessionRefresherTest {

    private val authApi = mockk<AuthApi>()

    private fun refresher(
        authState: AuthState,
        secretStore: SecretStore,
        api: AuthApi = authApi,
    ) = SessionRefresher(
        authState = authState,
        secretStore = secretStore,
        authApi = Provider { api },
    )

    private fun state(mode: CredentialMode = CredentialMode.SESSION, id: String? = "install-1"): AuthState {
        val authState = mockk<AuthState>(relaxed = true)
        every { authState.mode } returns mode
        every { authState.activeId } returns id
        return authState
    }

    @Test
    fun cannotRefreshWithoutASavedPair() {
        val authState = state()
        val secretStore = mockk<SecretStore>(relaxed = true)
        every { secretStore.hasSavedCredentials("install-1") } returns false
        val refresher = refresher(authState, secretStore)

        assertFalse(refresher.canRefresh())
    }

    @Test
    fun cannotRefreshAnApiKeyInstall() {
        val authState = state(mode = CredentialMode.API_KEY)
        val secretStore = mockk<SecretStore>(relaxed = true)
        every { secretStore.hasSavedCredentials("install-1") } returns true
        val refresher = refresher(authState, secretStore)

        assertFalse(refresher.canRefresh())
    }

    @Test
    fun refreshesByReplayingTheSavedPairAndInstallsTheNewToken() = runTest {
        val authState = state()
        val secretStore = mockk<SecretStore>(relaxed = true)
        every { secretStore.savedUsername("install-1") } returns "admin"
        every { secretStore.savedPassword("install-1") } returns "hunter2"
        val login = slot<LoginRequest>()
        coEvery { authApi.login(capture(login)) } returns LoginResponse(token = "digitdns_session_new")
        val refresher = refresher(authState, secretStore)

        assertTrue(refresher.refresh())
        assertEquals("admin", login.captured.username)
        assertEquals("hunter2", login.captured.password)
        verify { authState.updateSessionToken("digitdns_session_new") }
    }

    @Test
    fun aBlankTokenIsAFailedRefresh() = runTest {
        val authState = state()
        val secretStore = mockk<SecretStore>(relaxed = true)
        every { secretStore.savedUsername("install-1") } returns "admin"
        every { secretStore.savedPassword("install-1") } returns "hunter2"
        coEvery { authApi.login(any()) } returns LoginResponse(token = "")
        val refresher = refresher(authState, secretStore)

        assertFalse(refresher.refresh())
    }

    @Test
    fun aSuccessfulRefreshIsNotRepeatedWithinTheCooldown() = runTest {
        val authState = state()
        val secretStore = mockk<SecretStore>(relaxed = true)
        every { secretStore.savedUsername("install-1") } returns "admin"
        every { secretStore.savedPassword("install-1") } returns "hunter2"
        coEvery { authApi.login(any()) } returns LoginResponse(token = "digitdns_session_new")
        val refresher = refresher(authState, secretStore)

        assertTrue(refresher.refresh())
        assertTrue(refresher.refresh())
        // A burst of concurrent 401s must mint at most one session per window.
        coVerify(exactly = 1) { authApi.login(any()) }
    }
}
