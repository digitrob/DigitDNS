package com.digitrob.digitdns.core.network

import com.jakewharton.retrofit2.converter.kotlinx.serialization.asConverterFactory
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import retrofit2.Retrofit

/**
 * Exercises the login surface against a real HTTP server (MockWebServer), so the
 * pieces Phase 1 adds — the JSON converter, the [ApiExceptionInterceptor], and the
 * generated problem document — are tested as a wired stack rather than in isolation.
 *
 * The URLs here are absolute to the mock server, so [ServerUrlProvider] is exercised
 * only in its "fall through" form (no stored connection ⇒ the request is untouched),
 * which is itself a property worth pinning: a missing connection must not corrupt a
 * request, it must pass it through.
 */
class AuthApiTest {

    private lateinit var server: MockWebServer
    private lateinit var api: AuthApi

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()

        val json = Json { ignoreUnknownKeys = true; explicitNulls = false }
        val client = OkHttpClient.Builder()
            .addInterceptor(ApiExceptionInterceptor(Reachability()))
            .build()
        api = Retrofit.Builder()
            .baseUrl(server.url("/"))
            .client(client)
            .addConverterFactory(json.asConverterFactory("application/json".toMediaType()))
            .build()
            .create(AuthApi::class.java)
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun loginReturnsTheSessionTokenAndIdleWindow() = runTest {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"token":"digitdns_session_abc","username":"admin","expires_in":3600}"""),
        )

        val response = api.login(LoginRequest(username = "admin", password = "secret"))

        assertEquals("digitdns_session_abc", response.token)
        assertEquals(3600, response.expiresIn)

        val recorded = server.takeRequest()
        assertEquals("POST", recorded.method)
        assertEquals("/auth/login", recorded.path)
        // The TOTP code is omitted, not sent as null, when the operator gave none.
        assertTrue(recorded.body.readUtf8().contains("\"password\":\"secret\""))
    }

    @Test
    fun aTwoFactorLoginRaisesAnExceptionCarryingTheFlag() = runTest {
        server.enqueue(
            MockResponse()
                .setResponseCode(401)
                .setHeader("Content-Type", "application/problem+json")
                .setBody(
                    """{"type":"https://digitdns.dev/problems/unauthorized","title":"Unauthorized",""" +
                        """"status":401,"detail":"invalid credentials","twofactor_required":true}""",
                ),
        )

        val failure = runCatching { api.login(LoginRequest("admin", "secret")) }.exceptionOrNull()

        assertTrue(failure is ApiException)
        val apiException = failure as ApiException
        assertTrue(apiException.twoFactorRequired)
        assertEquals(401, apiException.httpStatus)
    }

    @Test
    fun aRefusedAddressSurfacesTheProblemDetail() = runTest {
        server.enqueue(
            MockResponse()
                .setResponseCode(403)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"status":403,"detail":"REST API is not exposed to this address."}"""),
        )

        val failure = runCatching { api.me() }.exceptionOrNull() as ApiException

        assertTrue(failure.isForbidden)
        assertEquals("REST API is not exposed to this address.", failure.userMessage)
    }

    @Test
    fun sseTicketCarriesTheOneTimeValueAndTtl() = runTest {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"ticket":"deadbeef","expires_in":60}"""),
        )

        val ticket = api.sseTicket()

        assertEquals("deadbeef", ticket.ticket)
        assertEquals(60, ticket.expiresIn)
        assertNull(Problem.parseOrNull(""))
    }
}
