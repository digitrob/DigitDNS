package com.digitrob.digitdns.core.network

import com.digitrob.digitdns.domain.model.CredentialMode
import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test

/**
 * The central 401 retry. It must retry exactly once after a successful refresh, but
 * must never attempt recovery on an auth route (a 401 there is the answer) or when no
 * refresh is possible.
 */
class AuthRetryInterceptorTest {

    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun client(refresher: SessionRefresher, authState: AuthState): OkHttpClient =
        OkHttpClient.Builder()
            .addInterceptor(AuthRetryInterceptor(refresher, authState))
            .addInterceptor(ApiExceptionInterceptor(Reachability()))
            .build()

    private fun sessionState(): AuthState = mockk(relaxed = true) {
        every { mode } returns CredentialMode.SESSION
    }

    @Test
    fun retriesOnceAfterASuccessfulRefresh() {
        val refresher = mockk<SessionRefresher>()
        every { refresher.canRefresh() } returns true
        every { refresher.refreshBlocking() } returns true
        val client = client(refresher, sessionState())

        server.enqueue(MockResponse().setResponseCode(401).setBody("{}"))
        server.enqueue(MockResponse().setResponseCode(200).setBody("{}"))

        client.newCall(Request.Builder().url(server.url("/api/v3/clients")).build())
            .execute().use { assertEquals(200, it.code) }
        assertEquals(2, server.requestCount)
    }

    @Test
    fun surfacesThe401WhenRefreshFails() {
        val refresher = mockk<SessionRefresher>()
        every { refresher.canRefresh() } returns true
        every { refresher.refreshBlocking() } returns false
        val authState = sessionState()
        val client = client(refresher, authState)

        server.enqueue(MockResponse().setResponseCode(401).setBody("{}"))

        val thrown = runCatching {
            client.newCall(Request.Builder().url(server.url("/api/v3/clients")).build()).execute()
        }.exceptionOrNull()

        assertEquals(ApiException::class.java, thrown?.javaClass)
        assertEquals(1, server.requestCount)
        verify { authState.markSessionExpired() }
    }

    @Test
    fun doesNotRefreshOnTheLoginRoute() {
        val refresher = mockk<SessionRefresher>()
        every { refresher.canRefresh() } returns true
        val authState = sessionState()
        val client = client(refresher, authState)

        server.enqueue(MockResponse().setResponseCode(401).setBody("{}"))

        runCatching {
            client.newCall(Request.Builder().url(server.url("/api/v3/auth/login")).build()).execute()
        }
        assertEquals(1, server.requestCount)
        verify(exactly = 0) { refresher.refreshBlocking() }
    }

    @Test
    fun marksExpiredWhenNoSavedPairExists() {
        val refresher = mockk<SessionRefresher>()
        every { refresher.canRefresh() } returns false
        val authState = sessionState()
        val client = client(refresher, authState)

        server.enqueue(MockResponse().setResponseCode(401).setBody("{}"))

        runCatching {
            client.newCall(Request.Builder().url(server.url("/api/v3/clients")).build()).execute()
        }
        verify { authState.markSessionExpired() }
        verify(exactly = 0) { refresher.refreshBlocking() }
    }
}
