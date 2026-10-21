package com.digitrob.digitdns.core.network

import com.digitrob.digitdns.data.AccessRepository
import com.jakewharton.retrofit2.converter.kotlinx.serialization.asConverterFactory
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import retrofit2.Retrofit

/**
 * The access gate. The split that matters: the token/IP save must go to `/access`
 * with `allow_all` ignored by the server, while a mode change must go to the
 * dedicated `/access/mode`. Sending the mode through the wrong route would look like
 * it worked and quietly not change the mode.
 */
class AccessApiTest {

    private lateinit var server: MockWebServer
    private lateinit var repo: AccessRepository

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        val json = Json { ignoreUnknownKeys = true; explicitNulls = false }
        val client = OkHttpClient.Builder().addInterceptor(ApiExceptionInterceptor(Reachability())).build()
        val api = Retrofit.Builder()
            .baseUrl(server.url("/"))
            .client(client)
            .addConverterFactory(json.asConverterFactory("application/json".toMediaType()))
            .build()
            .create(AccessApi::class.java)
        repo = AccessRepository(api)
    }

    @After
    fun tearDown() = server.shutdown()

    @Test
    fun loadReadsTheThreeFields() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"allow_all":false,"doh_tokens":["t1"],"allowed_ips":["203.0.113.0/24"]}""",
            ),
        )

        val settings = repo.load()

        assertEquals(false, settings.allowAll)
        assertEquals(listOf("t1"), settings.dohTokens)
        assertEquals(listOf("203.0.113.0/24"), settings.allowedIps)
    }

    @Test
    fun theTokenSaveGoesToAccessNotTheModeRoute() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"success":true}"""))

        repo.save(allowedIps = listOf("10.0.0.0/8"), dohTokens = listOf("tok"))

        val recorded = server.takeRequest()
        // POST, not PUT: the delegated v3 handler only mounts the write for POST.
        assertEquals("POST", recorded.method)
        assertEquals("/access", recorded.path)
    }

    @Test
    fun theModeChangeGoesToTheModeRoute() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"allow_all":true}"""))

        val ok = repo.setMode(true)

        assertTrue(ok)
        val recorded = server.takeRequest()
        assertEquals("POST", recorded.method)
        assertEquals("/access/mode", recorded.path)
        assertEquals("""{"allow_all":true}""", recorded.body.readUtf8())
    }
}
