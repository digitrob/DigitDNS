package com.digitrob.digitdns.core.network

import com.digitrob.digitdns.data.SettingsRepository
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
 * Server settings. Three things the test pins that are easy to get wrong: the read is
 * an `{server,tls}` envelope while the save response is a bare server view; the save
 * uses POST (PUT answers 405 through to a 400); and the save body drops nulls so an
 * absent field means "leave unchanged".
 */
class SettingsApiTest {

    private lateinit var server: MockWebServer
    private lateinit var repo: SettingsRepository

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
            .create(SettingsApi::class.java)
        repo = SettingsRepository(api)
    }

    @After
    fun tearDown() = server.shutdown()

    @Test
    fun loadReadsTheEnvelopeAndTheApiKey() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """
                {"server":{"public_ip":"203.0.113.7","bind_host":"0.0.0.0","web_port":37586,
                "admin_username":"admin","admin_password_weak":true,"api_key":"abcd",
                "api_bind":"10.0.0.0/8","api_bind_list":["10.0.0.0/8"],"session_idle_minutes":60,
                "admin_path":"/x"},"tls":{"domain":"dns.example","panel_https":true}}
                """.trimIndent(),
            ),
        )

        val bundle = repo.load()

        assertEquals("203.0.113.7", bundle.server?.publicIp)
        assertEquals("abcd", bundle.server?.apiKey)
        assertEquals(listOf("10.0.0.0/8"), bundle.server?.apiBindList)
        assertEquals("dns.example", bundle.tls?.domain)
    }

    @Test
    fun savePostsAndReadsTheBareServerView() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"api_key":"new","admin_path":"/y"}"""))

        val saved = repo.save("203.0.113.7", "10.0.0.0/8", null, null)

        val recorded = server.takeRequest()
        assertEquals("POST", recorded.method)
        assertEquals("/settings", recorded.path)
        assertEquals("""{"public_ip":"203.0.113.7","api_bind":"10.0.0.0/8"}""", recorded.body.readUtf8())
        assertEquals("/y", saved.adminPath)
    }

    @Test
    fun saveSendsNonNullOptionalFields() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{}"""))

        repo.save("1.2.3.4", "", listOf("10.0.0.0/8", "192.168.0.0/16"), 30)

        // An empty api_bind is encoded away (it equals the property default and the
        // converter drops defaults). That is correct: the server treats a missing or
        // empty api_bind identically — "leave this one alone" — so the app may omit
        // it without changing the stored value.
        val body = server.takeRequest().body.readUtf8()
        assertEquals(
            """{"public_ip":"1.2.3.4","api_bind_list":["10.0.0.0/8","192.168.0.0/16"],"session_idle_minutes":30}""",
            body,
        )
    }

    @Test
    fun regenerateApiKeyPostsPasswordAndReturnsTheKey() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"success":true,"api_key":"fresh"}"""))

        val key = repo.regenerateApiKey("pw", "123456")

        assertEquals("fresh", key)
        val recorded = server.takeRequest()
        assertEquals("/settings/regenerate-api-key", recorded.path)
        assertEquals("""{"current_password":"pw","code":"123456"}""", recorded.body.readUtf8())
    }

    @Test
    fun regenerateAdminPathPostsConfirmAndReturnsThePath() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"success":true,"admin_path":"/zzz"}"""))

        val path = repo.regenerateAdminPath(confirm = true)

        assertEquals("/zzz", path)
        val recorded = server.takeRequest()
        assertEquals("/settings/regenerate-admin-path", recorded.path)
        assertEquals("""{"confirm":true}""", recorded.body.readUtf8())
    }
}
