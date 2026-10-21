package com.digitrob.digitdns.core.network

import com.digitrob.digitdns.data.SubscriptionRepository
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
 * The subscription portal. The save is POST (PUT fails through to a 405→400), the
 * response carries the recomputed `subscription_origin`, and several derived fields
 * are absent from the request because the server overrides them.
 */
class SubscriptionApiTest {

    private lateinit var server: MockWebServer
    private lateinit var repo: SubscriptionRepository

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
            .create(SubscriptionApi::class.java)
        repo = SubscriptionRepository(api)
    }

    @After
    fun tearDown() = server.shutdown()

    @Test
    fun loadReadsTheSnapshot() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"enabled":true,"listen_ip":"0.0.0.0","domain":"sub.example","port":8443,
                "uri_path":"/sub","title":"Sub","theme_css_source":"url","theme_css_url":"https://x/t.css",
                "use_panel_certificate":true,"auto_renew_acme":true}""",
            ),
        )

        val settings = repo.load()

        assertEquals(true, settings.enabled)
        assertEquals("sub.example", settings.domain)
        assertEquals(8443, settings.port)
        assertEquals("url", settings.themeCssSource)
    }

    @Test
    fun savePostsAndReturnsTheOrigin() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"success":true,"restart_required":false,"subscription":{"enabled":true},
                "subscription_origin":"https://sub.example:8443","listener_error":""}""",
            ),
        )

        val result = repo.save(
            SubscriptionSaveRequest(enabled = true, domain = "sub.example", port = 8443, uriPath = "/sub"),
        )

        val recorded = server.takeRequest()
        assertEquals("POST", recorded.method)
        assertEquals("/settings/subscription", recorded.path)
        assertEquals("https://sub.example:8443", result.origin)
        assertEquals("", result.listenerError)
    }

    @Test
    fun saveSurfacesAListenerErrorWithoutFailing() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"success":true,"subscription_origin":"","listener_error":"port 8443 already in use"}""",
            ),
        )

        val result = repo.save(SubscriptionSaveRequest(enabled = true))

        assertEquals("port 8443 already in use", result.listenerError)
    }
}
