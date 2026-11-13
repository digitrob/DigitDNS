package com.digitrob.digitdns.core.network

import com.digitrob.digitdns.data.FilteringRepository
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

/** The v3 filtering paths and the status document the roster consumes. */
class FilteringApiTest {

    private lateinit var server: MockWebServer
    private lateinit var repo: FilteringRepository

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
            .create(FilteringApi::class.java)
        repo = FilteringRepository(api)
    }

    @After
    fun tearDown() = server.shutdown()

    @Test
    fun statusParsesSettingsListsPresetsAndCounts() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """
                {
                  "settings": {"enabled": true, "block_mode": "null_ip", "block_ttl": 10,
                               "update_interval_hours": 24, "allowlist": null},
                  "lists": [
                    {"id": "l1", "name": "HaGeZi Pro", "preset_id": "hagezi-pro",
                     "enabled": true, "domain_count": 247757, "rule_count": 247757,
                     "allowed_count": 0, "skipped": 0, "last_updated": "2026-11-04T15:01:16Z"}
                  ],
                  "presets": [
                    {"id": "hagezi-pro", "name": "HaGeZi Pro", "group": "base",
                     "url": "https://example.com/pro.txt", "homepage": "https://example.com",
                     "license": "GPL-3.0", "recommended": true, "base": true}
                  ],
                  "preset_counts": [
                    {"preset_id": "hagezi-light", "domain_count": 80509, "last_error": ""}
                  ],
                  "preset_counts_running": true,
                  "total_rules": 247757,
                  "total_domains": 247757,
                  "updating": []
                }
                """.trimIndent(),
            ),
        )

        val status = repo.status()

        assertEquals(true, status.settings.enabled)
        assertNull(status.settings.allowlist)
        assertEquals(1, status.lists.size)
        assertEquals("hagezi-pro", status.lists.first().presetId)
        assertEquals(247757, status.lists.first().domainCount)
        assertEquals("base", status.presets.first().group)
        assertEquals(80509, status.presetCounts.first().domainCount)
        assertEquals(true, status.presetCountsRunning)
        assertEquals(247757, status.totalDomains)
    }

    @Test
    fun addPresetPostsTheId() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"added":[],"skipped":[],"failed":[]}"""),
        )

        repo.addPreset("hagezi-pro")

        val request = server.takeRequest()
        assertEquals("/filtering/presets", request.path)
        assertEquals("""{"ids":["hagezi-pro"]}""", request.body.readUtf8())
    }

    @Test
    fun enableUsesPatchAndTheEnabledBody() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"success":true}"""))

        repo.setEnabled("l1", false)

        val request = server.takeRequest()
        assertEquals("PATCH", request.method)
        assertEquals("/filtering/lists/l1", request.path)
        assertEquals("""{"enabled":false}""", request.body.readUtf8())
    }

    @Test
    fun deleteAndUpdateUseTheItemRoutes() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"success":true}"""))
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"success":true}"""))

        repo.delete("l1")
        repo.update("l1")

        val delete = server.takeRequest()
        assertEquals("DELETE", delete.method)
        assertEquals("/filtering/lists/l1", delete.path)

        val update = server.takeRequest()
        assertEquals("POST", update.method)
        assertEquals("/filtering/lists/l1/update", update.path)
    }

    @Test
    fun saveSettingsSendsEveryFieldEvenWhenTheValueIsTheDefault() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"settings":{},"lists":[],"presets":[]}"""))

        repo.saveSettings(FilteringSettingsRequest(true, "null_ip", 15, 24, emptyList(), ""))

        val request = server.takeRequest()
        assertEquals("/filtering/settings", request.path)
        assertEquals(
            """{"enabled":true,"block_mode":"null_ip","block_ttl":15,"update_interval_hours":24,"allowlist":[],"custom_rules":""}""",
            request.body.readUtf8(),
        )
    }

    @Test
    fun addListSendsUploadedContent() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"id":"new","name":"Uploaded","url":"","homepage":"","license":"","enabled":true}""",
            ),
        )

        repo.addList("Uploaded", "", "", "0.0.0.0 ads.example\n")

        val request = server.takeRequest()
        assertEquals("/filtering/lists", request.path)
        val body = request.body.readUtf8()
        assertTrue(body.contains("\"name\":\"Uploaded\""))
        assertTrue(body.contains("\"content\":\"0.0.0.0 ads.example\\n\""))
    }
}
