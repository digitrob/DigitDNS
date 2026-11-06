package com.digitrob.digitdns.core.network

import com.digitrob.digitdns.data.HistoryRepository
import com.digitrob.digitdns.domain.model.HistoryRange
import com.digitrob.digitdns.domain.model.TopDimension
import com.jakewharton.retrofit2.converter.kotlinx.serialization.asConverterFactory
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import retrofit2.Retrofit

/**
 * Historical aggregates and the server-wide policy. The load path fans out to
 * eight endpoints (summary, series and six ranked tables), so the request set is
 * asserted as a whole; the settings save body is checked field-for-field because
 * a defaulted serializer would silently omit values the server validates.
 */
class HistoryApiTest {

    private lateinit var server: MockWebServer
    private lateinit var repo: HistoryRepository

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
            .create(HistoryApi::class.java)
        repo = HistoryRepository(api)
    }

    @After
    fun tearDown() = server.shutdown()

    @Test
    fun loadFetchesSummarySeriesAndEveryTopDimension() = runTest {
        // The eight calls run concurrently, so responses are selected by path
        // rather than enqueue order.
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                val path = request.path.orEmpty()
                val body = when {
                    path.contains("/stats/summary") ->
                        """{"range":"24h","queries":12,"active_clients":3}"""
                    path.contains("/stats/series") ->
                        """{"range":"24h","interval":"hour","points":[]}"""
                    else ->
                        """{"items":[{"value":"x","count":1}],"other_count":2}"""
                }
                return MockResponse().setResponseCode(200).setBody(body)
            }
        }

        val snapshot = repo.load(HistoryRange.LAST_7D)

        assertEquals(12L, snapshot.summary.queries)
        assertEquals(3, snapshot.summary.activeClients)
        assertEquals(6, snapshot.tops.size)
        repeat(8) {
            val path = server.takeRequest().path.orEmpty()
            assertTrue("unexpected path $path", path.startsWith("/stats/"))
            assertTrue("missing range in $path", path.contains("range=7d"))
            if (path.startsWith("/stats/top")) {
                assertTrue("top limit is not 5 in $path", path.contains("limit=5"))
            }
        }
        assertTrue(snapshot.tops.containsKey(TopDimension.CLIENT))
    }

    @Test
    fun saveSettingsEmitsEveryFieldIncludingAnEmptyExcludeList() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"success":true,"restart_required":true,"settings":{"enabled":true,"retention_days":30,"top_domains":200,"top_clients":100,"anonymize_clients":true,"exclude_clients":[]}}""",
            ),
        )

        val response = repo.saveSettings(
            StatsSettings(
                enabled = true,
                retentionDays = 30,
                topDomains = 200,
                topClients = 100,
                anonymizeClients = true,
                excludeClients = emptyList(),
            ),
        )

        assertTrue(response.success)
        assertTrue(response.restartRequired)
        val recorded = server.takeRequest()
        assertEquals("POST", recorded.method)
        assertEquals("/stats/settings", recorded.path)
        assertEquals(
            """{"enabled":true,"retention_days":30,"top_domains":200,"top_clients":100,"anonymize_clients":true,"exclude_clients":[]}""",
            recorded.body.readUtf8(),
        )
    }

    @Test
    fun settingsReadCoercesANullExcludeListToEmpty() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"enabled":true,"retention_days":90,"top_domains":1000,"top_clients":500,"anonymize_clients":false,"exclude_clients":null}""",
            ),
        )

        assertEquals(emptyList<String>(), repo.settings().excludeClients.orEmpty())
    }

    @Test
    fun settingsReadMapsSnakeCaseFields() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"enabled":false,"retention_days":45,"top_domains":300,"top_clients":80,"anonymize_clients":true,"exclude_clients":["192.0.2.7"]}""",
            ),
        )

        val settings = repo.settings()

        assertEquals(false, settings.enabled)
        assertEquals(45, settings.retentionDays)
        assertEquals(listOf("192.0.2.7"), settings.excludeClients.orEmpty())
    }
}
