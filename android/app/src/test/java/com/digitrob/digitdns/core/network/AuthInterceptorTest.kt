package com.digitrob.digitdns.core.network

import com.digitrob.digitdns.data.ConnectionStore
import com.digitrob.digitdns.domain.model.Connection
import com.digitrob.digitdns.domain.model.CredentialMode
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.flow.flowOf
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * The interceptor does two jobs, and both are load-bearing:
 *  1. Rewrite the placeholder origin onto the stored install — path, host, port and
 *     admin prefix. Without it a typed `@GET("clients")` would hit `127.0.0.1`.
 *  2. Attach exactly one credential, in the shape the chosen mode calls for.
 *
 * Both are asserted against a live socket so the assertion is about the request that
 * actually left, not a re-derivation of what should have.
 */
class AuthInterceptorTest {

    private lateinit var server: MockWebServer
    /** The store the provider reads; mocked so no Android Context is needed. */
    private lateinit var store: ConnectionStore

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        store = mockk(relaxed = true)
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun clientFor(connection: Connection, authState: AuthState): OkHttpClient {
        every { store.connection } returns flowOf(connection)
        val provider = ServerUrlProvider(store)
        val interceptor = AuthInterceptor(authState, provider)
        return OkHttpClient.Builder().addInterceptor(interceptor).build()
    }

    private fun stubAuthState(connection: Connection): AuthState {
        val authState = mockk<AuthState>(relaxed = true)
        every { authState.mode } returns connection.credentialMode
        return authState
    }

    @Test
    fun rewritesTheOriginAndPathOntoTheStoredInstallIncludingAdminPath() {
        val connection = Connection(
            host = server.hostName,
            port = server.port,
            adminPath = "ad90ecbb08bfa890",
            isHttps = false,
        )
        val authState = stubAuthState(connection)
        every { authState.sessionToken } returns "digitdns_session_tok"
        val client = clientFor(connection, authState)

        server.enqueue(MockResponse().setResponseCode(200).setBody("{}"))
        client.newCall(
            // A typed interface path, relative to the API root, exactly as Retrofit sends it.
            Request.Builder().url("https://127.0.0.1/clients?limit=1").build(),
        ).execute().close()

        val recorded = server.takeRequest()
        // The `api/v3` segment must be present: the server strips the admin prefix and
        // matches the rest against its route table, which only knows `/api/v3/...`.
        assertEquals("/ad90ecbb08bfa890/api/v3/clients?limit=1", recorded.path)
    }

    @Test
    fun insertsTheApiVersionSegmentTheServerMountsHandlersUnder() {
        // Regression: the `api/v3` segment was once dropped, so every call 404ed and
        // the auth layer reported it as a rejected credential. Pin the full path here
        // so it cannot silently disappear again.
        val connection = Connection(
            host = server.hostName,
            port = server.port,
            adminPath = "ad90ecbb08bfa890",
            isHttps = false,
        )
        val authState = stubAuthState(connection)
        every { authState.sessionToken } returns "digitdns_session_tok"
        val client = clientFor(connection, authState)

        server.enqueue(MockResponse().setResponseCode(200).setBody("{}"))
        client.newCall(Request.Builder().url("https://127.0.0.1/auth/me").build()).execute().close()

        val recorded = server.takeRequest()
        assertEquals("/ad90ecbb08bfa890/api/v3/auth/me", recorded.path)
    }

    @Test
    fun sessionModeSendsBearerAndNotTheApiKey() {
        val connection = Connection(server.hostName, server.port, "admin", isHttps = false)
        val authState = stubAuthState(connection)
        every { authState.sessionToken } returns "digitdns_session_tok"
        every { authState.apiKey } returns "digitdns_live_shouldNotAppear"
        val client = clientFor(connection, authState)

        server.enqueue(MockResponse().setResponseCode(200).setBody("{}"))
        client.newCall(Request.Builder().url("https://127.0.0.1/version").build()).execute().close()

        val recorded = server.takeRequest()
        assertEquals("Bearer digitdns_session_tok", recorded.getHeader("Authorization"))
        assertNull(recorded.getHeader("X-API-Key"))
    }

    @Test
    fun apiKeyModeSendsTheHeaderAndNoBearer() {
        val connection = Connection(
            server.hostName, server.port, "admin",
            credentialMode = CredentialMode.API_KEY, isHttps = false,
        )
        val authState = stubAuthState(connection)
        every { authState.apiKey } returns "digitdns_live_abc"
        every { authState.sessionToken } returns null
        val client = clientFor(connection, authState)

        server.enqueue(MockResponse().setResponseCode(200).setBody("{}"))
        client.newCall(Request.Builder().url("https://127.0.0.1/version").build()).execute().close()

        val recorded = server.takeRequest()
        assertEquals("digitdns_live_abc", recorded.getHeader("X-API-Key"))
        assertNull(recorded.getHeader("Authorization"))
    }

    @Test
    fun noConnectionFailsFastInsteadOfContactingThePlaceholderHost() {
        // Regression: with no stored connection the request kept its `127.0.0.1`
        // placeholder origin and was actually sent there, surfacing as
        // "Failed to connect to 127.0.0.1:443" — an address the operator never typed.
        // It must fail with a typed error naming the real state instead.
        every { store.connection } returns flowOf(null)
        val provider = ServerUrlProvider(store)
        val authState = mockk<AuthState>(relaxed = true)
        every { authState.mode } returns CredentialMode.SESSION
        every { authState.sessionToken } returns null
        val client = OkHttpClient.Builder()
            .addInterceptor(AuthInterceptor(authState, provider))
            .build()

        val call = client.newCall(Request.Builder().url("https://127.0.0.1/clients").build())
        val thrown = runCatching { call.execute() }.exceptionOrNull()
        assertTrue(
            "expected NoConnectionException, got $thrown",
            thrown is NoConnectionException ||
                (thrown?.cause is NoConnectionException),
        )
    }

    @Test
    fun anAlreadyAbsoluteApiUrlPassesThroughRebaseUnchanged() {
        // Regression: the SSE stream builds its absolute URL itself and still rides
        // this client, so rebase prepended the admin + api/v3 prefix a second time and
        // the stream 404ed forever. An already-rebased URL must be left alone.
        val connection = Connection(
            host = server.hostName,
            port = server.port,
            adminPath = "ad90ecbb08bfa890",
            isHttps = false,
        )
        val authState = stubAuthState(connection)
        every { authState.sessionToken } returns "digitdns_session_tok"
        val client = clientFor(connection, authState)

        server.enqueue(MockResponse().setResponseCode(200).setBody("{}"))
        val absolute = server.url("/ad90ecbb08bfa890/api/v3/events/stream?ticket=abc")
        client.newCall(Request.Builder().url(absolute).build()).execute().close()

        assertEquals(
            "/ad90ecbb08bfa890/api/v3/events/stream?ticket=abc",
            server.takeRequest().path,
        )
    }
}
