package com.digitrob.digitdns.core.network

import com.digitrob.digitdns.data.DiagnosticsRepository
import com.jakewharton.retrofit2.converter.kotlinx.serialization.asConverterFactory
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test
import retrofit2.Retrofit

/**
 * Reachability diagnostics. The list uses a `targets` envelope rather than the `items`
 * convention of the other lists, so the test guards against a DTO that would silently
 * decode an empty list. Add and delete both return the refreshed snapshot.
 */
class DiagnosticsApiTest {

    private lateinit var server: MockWebServer
    private lateinit var repo: DiagnosticsRepository

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
            .create(DiagnosticsApi::class.java)
        repo = DiagnosticsRepository(api)
    }

    @After
    fun tearDown() = server.shutdown()

    @Test
    fun targetsReadsTheTargetsEnvelopeNotItems() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"items":["ignored"],"targets":[{"name":"Google","host":"8.8.8.8"}]}"""),
        )

        val targets = repo.targets()

        assertEquals(1, targets.size)
        assertEquals("Google", targets.first().name)
    }

    @Test
    fun addPostsAndReturnsTheRefreshedList() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"targets":[{"name":"A","host":"1.1.1.1"},{"name":"B","host":"8.8.8.8"}]}"""),
        )

        val after = repo.addTarget("B", "8.8.8.8")

        assertEquals(2, after.size)
        val recorded = server.takeRequest()
        assertEquals("POST", recorded.method)
        assertEquals("/diagnostics/targets", recorded.path)
        assertEquals("""{"name":"B","host":"8.8.8.8"}""", recorded.body.readUtf8())
    }

    @Test
    fun deleteSendsTheHostQueryAndReturnsTheList() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"targets":[]}"""))

        val after = repo.deleteTarget("8.8.8.8")

        assertEquals(0, after.size)
        val recorded = server.takeRequest()
        assertEquals("DELETE", recorded.method)
        assertEquals("/diagnostics/targets?host=8.8.8.8", recorded.path)
    }

    @Test
    fun runReadsTheReport() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"overall_score":95,"overall_quality":"excellent","avg_latency_ms":12.5,
                "reachable":2,"total":2,"results":[{"name":"Google","target":"8.8.8.8","success":true,"latency_ms":12.5}],
                "timestamp":"2026-01-01T00:00:00Z"}""",
            ),
        )

        val report = repo.run()

        assertEquals(95, report.overallScore)
        assertEquals(2, report.total)
        assertEquals(1, report.results.size)
        assertEquals(12.5, report.results.first().latencyMs, 0.001)
    }
}
