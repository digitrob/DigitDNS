package com.digitrob.digitdns.core.network

import com.digitrob.digitdns.data.SystemRepository
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
 * System status, update and cache. The update check returns 200 even on a network
 * failure with `error` set, so the test pins that a caller which only looks at
 * `update_available` would misread a failed check as "up to date".
 */
class SystemApiTest {

    private lateinit var server: MockWebServer
    private lateinit var repo: SystemRepository

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
            .create(SystemApi::class.java)
        repo = SystemRepository(api)
    }

    @After
    fun tearDown() = server.shutdown()

    @Test
    fun summaryReadsVersionAndExposure() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"status":"ok","version":{"version":"3.2.2","channel":"stable","codename":"x",
                "display":"DigitDNS 3.2.2","hash":"abc","commit":"ad98558","go":"go1.26.4"},
                "public_ip":"203.0.113.7","api_bind":"10.0.0.0/8","api_bind_list":["10.0.0.0/8"],
                "api_exposure":"private"}""",
            ),
        )

        val summary = repo.summary()

        assertEquals("ok", summary.status)
        assertEquals("DigitDNS 3.2.2", summary.display)
        assertEquals("ad98558", summary.commit)
        assertEquals("private", summary.apiExposure)
    }

    @Test
    fun checkUpdateReadsAnAvailableUpdate() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"current":"3.2.2","latest":"3.3.0","codename":"y","channel":"stable",
                "update_available":true,"supported":true,"note":"","error":""}""",
            ),
        )

        val update = repo.checkUpdate()

        assertTrue(update.updateAvailable)
        assertTrue(update.canApply)
        assertEquals("/update/check", server.takeRequest().path)
    }

    @Test
    fun aFailedCheckIsA200WithErrorSetAndNeverAppliable() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"current":"3.2.2","latest":"","update_available":false,"supported":false,
                "note":"","error":"could not reach the release server"}""",
            ),
        )

        val update = repo.checkUpdate()

        assertEquals("could not reach the release server", update.error)
        assertFalse(update.updateAvailable)
        assertFalse(update.canApply)
    }

    @Test
    fun applyReportsStarted() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"started":true}"""))

        assertTrue(repo.applyUpdate())
        assertEquals("/update/apply", server.takeRequest().path)
    }

    @Test
    fun updateProgressReadsThePhase() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"running":true,"phase":"download","percent":30,"message":"downloading",
                "restarting":false,"error":""}""",
            ),
        )

        val progress = repo.updateProgress()

        assertTrue(progress.running)
        assertEquals("download", progress.phase)
        assertEquals(30, progress.percent)
    }

    @Test
    fun flushCacheReportsTheFlag() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"flushed":true}"""))

        assertTrue(repo.flushCache())
        assertEquals("/cache/flush", server.takeRequest().path)
    }
}
