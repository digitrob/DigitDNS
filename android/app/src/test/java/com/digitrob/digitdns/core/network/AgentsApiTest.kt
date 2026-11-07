package com.digitrob.digitdns.core.network

import com.digitrob.digitdns.data.AgentsRepository
import com.jakewharton.retrofit2.converter.kotlinx.serialization.asConverterFactory
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import retrofit2.Retrofit

/**
 * The per-agent IPv6 fields. The switch is a nullable boolean in the update
 * request so an explicit `false` is actually sent: with a plain
 * `Boolean = false`, the JSON writer treats the value as the default and omits
 * it, leaving the agent enabled on the server while the app shows it off.
 */
class AgentsApiTest {

    private lateinit var server: MockWebServer
    private lateinit var repo: AgentsRepository

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
            .create(AgentsApi::class.java)
        repo = AgentsRepository(api)
    }

    @After
    fun tearDown() = server.shutdown()

    @Test
    fun updateSendsAnExplicitIPv6OffAndMapsTheReply() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"agent":{"id":"a1","name":"Test Agent","country":"DE",
                "public_ipv6":"2001:db8::2","ipv6_enabled":false}}""",
            ),
        )

        val updated = repo.update(
            id = "a1",
            name = "Test Agent",
            country = "DE",
            publicIp = "127.0.0.1",
            publicIpv6 = "2001:db8::2",
            ipv6Enabled = false,
        )

        val recorded = server.takeRequest()
        assertEquals("PUT", recorded.method)
        assertEquals("/agents/a1", recorded.path)
        assertEquals(
            """{"name":"Test Agent","country":"DE","public_ip":"127.0.0.1","public_ipv6":"2001:db8::2","ipv6_enabled":false}""",
            recorded.body.readUtf8(),
        )
        assertEquals("2001:db8::2", updated.publicIpv6)
        assertFalse(updated.ipv6Enabled)
    }

    @Test
    fun listParsesTheIPv6AddressAndSwitch() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"agents":[{"id":"a1","name":"Test Agent","country":"DE",
                "public_ipv6":"2001:db8::2","ipv6_enabled":true}]}""",
            ),
        )

        val agents = repo.list()

        assertEquals(1, agents.size)
        assertEquals("2001:db8::2", agents[0].publicIpv6)
        assertTrue(agents[0].ipv6Enabled)
    }
}
