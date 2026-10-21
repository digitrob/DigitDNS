package com.digitrob.digitdns.core.network

import com.digitrob.digitdns.data.TlsRepository
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
 * Certificate issuance. The issue call is purpose-scoped and only *starts* a run
 * (`issuing`), so the test pins both the request shape and the two response cases:
 * a started run versus an already-valid certificate that was simply applied.
 */
class TlsApiTest {

    private lateinit var server: MockWebServer
    private lateinit var repo: TlsRepository

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
            .create(TlsApi::class.java)
        repo = TlsRepository(api)
    }

    @After
    fun tearDown() = server.shutdown()

    @Test
    fun issuePostsPurposeAndReportsAStartedRun() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"success":true,"domain":"dns.example","issuing":true,"detail":"requesting"}"""),
        )

        val result = repo.issue("dns.example", "me@example.com", "dot")

        assertTrue(result.issuing)
        assertEquals("dns.example", result.domain)
        val recorded = server.takeRequest()
        assertEquals("/tls/issue", recorded.path)
        assertEquals("""{"domain":"dns.example","email":"me@example.com","purpose":"dot"}""", recorded.body.readUtf8())
    }

    @Test
    fun issueOmitsTheDefaultPanelPurpose() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"success":true,"domain":"dns.example","issuing":true,"detail":""}"""),
        )

        repo.issue("dns.example", "", "panel")

        // "panel" is the DTO default and the converter drops defaults, so it is not on
        // the wire. The server defaults a missing purpose to panel, so this is correct.
        assertEquals("""{"domain":"dns.example"}""", server.takeRequest().body.readUtf8())
    }

    @Test
    fun issueReportsAnAlreadyAppliedCertificate() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"success":true,"domain":"dns.example","issuing":false,"detail":"valid cert on disk"}"""),
        )

        val result = repo.issue("dns.example", "", "dot")

        assertEquals(false, result.issuing)
        assertEquals("valid cert on disk", result.detail)
    }

    @Test
    fun autoRenewPostsTheFlagAndReturnsTheAppliedValue() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"success":true,"enabled":true}"""))

        val applied = repo.setAutoRenew(true)

        assertTrue(applied)
        val recorded = server.takeRequest()
        assertEquals("/tls/auto-renew", recorded.path)
        assertEquals("""{"enabled":true}""", recorded.body.readUtf8())
    }

    @Test
    fun acmeStatusReadsTheRun() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"running":true,"purpose":"panel","domain":"dns.example","stage":"download",
                "progress":40,"detail":"fetching","error":"","last_detail":"ok","last_run_at":"2026-01-01T00:00:00Z"}""",
            ),
        )

        val run = repo.acmeStatus()

        assertTrue(run.running)
        assertEquals("download", run.stage)
        assertEquals(40, run.progress)
        assertEquals("2026-01-01T00:00:00Z", run.lastRunAt)
    }
}
