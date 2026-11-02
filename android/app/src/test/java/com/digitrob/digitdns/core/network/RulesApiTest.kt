package com.digitrob.digitdns.core.network

import com.digitrob.digitdns.data.RulesRepository
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
 * The rules surface against a fake server. The properties worth pinning are the ones
 * a wrong guess silently breaks: the five-key read shape, and the partial POST that
 * must carry *only* the edited list so it cannot clobber the others.
 */
class RulesApiTest {

    private lateinit var server: MockWebServer
    private lateinit var repo: RulesRepository

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
            .create(RulesApi::class.java)
        repo = RulesRepository(api)
    }

    @After
    fun tearDown() = server.shutdown()

    @Test
    fun readDecodesAllFiveKeys() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"custom_proxied":["a.com"],"custom_proxied_disabled":["b.com"],""" +
                    """"custom_blocked":["c.com"],"custom_direct":["d.com"],""" +
                    """"custom_records":{"pin.com":"10.0.0.1"}}""",
            ),
        )

        val lists = repo.load()

        assertEquals(listOf("a.com"), lists.proxied)
        assertEquals(listOf("b.com"), lists.proxiedDisabled)
        assertEquals(listOf("c.com"), lists.blocked)
        assertEquals(listOf("d.com"), lists.direct)
        assertEquals("10.0.0.1", lists.records["pin.com"])
    }

    @Test
    fun aPartialListSaveSendsOnlyTheEditedKey() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"success":true}"""))

        repo.saveList(com.digitrob.digitdns.domain.model.RuleList.Blocked, listOf("ads.example.com"))

        val recorded = server.takeRequest()
        // POST, not PUT: the delegated v3 handler only mounts the write for POST.
        assertEquals("POST", recorded.method)
        val body = recorded.body.readUtf8()
        assertTrue(body.contains("\"custom_blocked\":[\"ads.example.com\"]"))
        // The other four must be absent, not sent empty: an empty array would wipe them.
        assertFalse(body.contains("custom_proxied"))
        assertFalse(body.contains("custom_direct"))
        assertFalse(body.contains("custom_records"))
    }

    @Test
    fun recordsAreReplacedWholesale() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"success":true}"""))

        repo.saveRecords(mapOf("nas.home" to "10.0.0.5"))

        val body = server.takeRequest().body.readUtf8()
        assertTrue(body.contains("\"custom_records\":{\"nas.home\":\"10.0.0.5\"}"))
    }

    @Test
    fun servicesRoundTripOnTheirOwnKeys() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"services":{"bbc":"BBC iPlayer"},""" +
                    """"domain_services":{"bbc.co.uk":"bbc"},""" +
                    """"service_icons":{"bbc":"https://icons.duckduckgo.com/ip3/bbc.co.uk.ico"}}""",
            ),
        )
        val lists = repo.load()
        assertEquals("BBC iPlayer", lists.services["bbc"])
        assertEquals("bbc", lists.domainServices["bbc.co.uk"])
        assertEquals("https://icons.duckduckgo.com/ip3/bbc.co.uk.ico", lists.serviceIcons["bbc"])

        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"success":true}"""))
        repo.save(lists)
        // The load above is still first in the queue; the save is the request whose
        // body matters here.
        server.takeRequest()
        val body = server.takeRequest().body.readUtf8()
        // The same keys the panel writes, so a service is shared between the two clients.
        assertTrue(body, body.contains("\"services\":{\"bbc\":\"BBC iPlayer\"}"))
        assertTrue(body.contains("\"domain_services\":{\"bbc.co.uk\":\"bbc\"}"))
        assertTrue(body.contains("\"service_icons\""))
    }

    @Test
    fun saveReportsTheServerAcknowledgement() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"success":true}"""))
        assertTrue(repo.saveRecords(emptyMap()))
    }
}
