package com.digitrob.digitdns.core.network

import com.digitrob.digitdns.data.ClientsRepository
import com.digitrob.digitdns.domain.model.ClientAction
import com.digitrob.digitdns.domain.model.ClientExpiry
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
 * The clients surface end to end against a fake server: request paths, the body the
 * PATCH sends, and — the part worth a test of its own — that `listAll` actually
 * follows `next_cursor` instead of stopping at one page.
 */
class ClientsApiTest {

    private lateinit var server: MockWebServer
    private lateinit var repo: ClientsRepository

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        val json = Json { ignoreUnknownKeys = true; explicitNulls = false }
        val client = OkHttpClient.Builder()
            .addInterceptor(ApiExceptionInterceptor(Reachability()))
            .build()
        val api = Retrofit.Builder()
            .baseUrl(server.url("/"))
            .client(client)
            .addConverterFactory(json.asConverterFactory("application/json".toMediaType()))
            .build()
            .create(ClientsApi::class.java)
        repo = ClientsRepository(api)
    }

    @After
    fun tearDown() = server.shutdown()

    @Test
    fun listFollowsTheCursorAcrossPages() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"items":[{"id":"a","name":"A"}],"next_cursor":"200","allow_all":false}""",
            ),
        )
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"items":[{"id":"b","name":"B"}],"next_cursor":"","allow_all":false}""",
            ),
        )

        val snapshot = repo.listAll()

        assertEquals(listOf("a", "b"), snapshot.clients.map { it.id })
        assertFalse(snapshot.allowAll)

        val first = server.takeRequest()
        assertEquals("GET", first.method)
        assertTrue(first.path!!.startsWith("/clients?limit=200"))
        val second = server.takeRequest()
        assertTrue(second.path!!.contains("cursor=200"))
    }

    @Test
    fun createPostsTheNamedFields() = runTest {
        server.enqueue(MockResponse().setResponseCode(201).setBody("""{"id":"x","name":"New"}"""))

        repo.create(
            ClientCreateRequest(
                name = "New",
                expiresAt = "2026-12-01T12:00:00Z",
                allowedIps = listOf("10.0.0.1"),
                trafficLimitGb = 50.0,
                trafficResetCycle = "monthly",
            ),
        )

        val body = server.takeRequest().body.readUtf8()
        assertTrue(body.contains("\"name\":\"New\""))
        assertTrue(body.contains("\"expires_at\":\"2026-12-01T12:00:00Z\""))
        assertTrue(body.contains("\"traffic_reset_cycle\":\"monthly\""))
    }

    @Test
    fun patchOmitsUnchangedNullFields() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"id":"x","name":"Renamed"}"""))

        // Only the name is set; every other member is null and must not appear, or the
        // server would read it as "clear this field".
        repo.update("x", ClientUpdateRequest(name = "Renamed"))

        val body = server.takeRequest().body.readUtf8()
        assertTrue(body.contains("\"name\":\"Renamed\""))
        assertFalse(body.contains("allowed_ips"))
        assertFalse(body.contains("traffic_limit_gb"))
        assertFalse(body.contains("enabled"))
        assertFalse(body.contains("expires_at"))
        assertFalse(body.contains("uuid"))
    }

    @Test
    fun patchClearsExpiryWithTheZeroTimestamp() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"id":"x"}"""))

        repo.update("x", ClientUpdateRequest(expiresAt = ClientExpiry.NEVER))

        val body = server.takeRequest().body.readUtf8()
        assertTrue(body.contains("\"expires_at\":\"0001-01-01T00:00:00Z\""))
    }

    @Test
    fun resetTrafficHitsTheHyphenatedActionPath() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"reset":true}"""))

        val result = repo.runAction("x", ClientAction.ResetTraffic)

        assertTrue(result.success)
        assertEquals("/clients/x/actions/reset-traffic", server.takeRequest().path)
    }

    @Test
    fun regenerateUuidReturnsTheNewValue() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"uuid":"new-uuid-value"}"""))

        val result = repo.runAction("x", ClientAction.RegenerateUuid)

        assertEquals("new-uuid-value", result.uuid)
        assertEquals("/clients/x/actions/regenerate-uuid", server.takeRequest().path)
    }
}
