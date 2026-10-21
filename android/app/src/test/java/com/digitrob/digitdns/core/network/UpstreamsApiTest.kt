package com.digitrob.digitdns.core.network

import com.digitrob.digitdns.data.UpstreamsRepository
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
 * The upstream pool. Addresses are opaque strings to this client, so the tests pin
 * the things that are the client's job: the raw list shape, the `{address}` add, and
 * the full-set `{order}` reorder.
 */
class UpstreamsApiTest {

    private lateinit var server: MockWebServer
    private lateinit var repo: UpstreamsRepository

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
            .create(UpstreamsApi::class.java)
        repo = UpstreamsRepository(api)
    }

    @After
    fun tearDown() = server.shutdown()

    @Test
    fun listReadsBareStrings() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"items":["8.8.8.8:53","tls://dns.google"]}"""))

        val upstreams = repo.list()

        assertEquals(listOf("8.8.8.8:53", "tls://dns.google"), upstreams)
    }

    @Test
    fun addPostsTheAddress() = runTest {
        server.enqueue(MockResponse().setResponseCode(201).setBody("""{"items":["8.8.8.8:53","1.1.1.1:53"]}"""))

        val after = repo.add("1.1.1.1")

        assertEquals(listOf("8.8.8.8:53", "1.1.1.1:53"), after)
        val body = server.takeRequest().body.readUtf8()
        assertEquals("""{"address":"1.1.1.1"}""", body)
    }

    @Test
    fun reorderSendsTheWholeOrderedSet() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"items":["1.1.1.1:53","8.8.8.8:53"]}"""))

        val after = repo.reorder(listOf("1.1.1.1:53", "8.8.8.8:53"))

        assertEquals(listOf("1.1.1.1:53", "8.8.8.8:53"), after)
        val recorded = server.takeRequest()
        assertEquals("/upstreams/reorder", recorded.path)
        assertEquals("""{"order":["1.1.1.1:53","8.8.8.8:53"]}""", recorded.body.readUtf8())
    }

    @Test
    fun removePostsTheAddressToTheDeleteRoute() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"items":["1.1.1.1:53"]}"""))

        val after = repo.remove("8.8.8.8")

        assertEquals(listOf("1.1.1.1:53"), after)
        val recorded = server.takeRequest()
        assertEquals("/upstreams/delete", recorded.path)
        assertEquals("""{"address":"8.8.8.8"}""", recorded.body.readUtf8())
    }

    @Test
    fun benchmarkReportsStarted() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"started":true,"running":true,"message":"go"}"""),
        )

        assertTrue(repo.benchmark())
        assertEquals("/upstreams/benchmark", server.takeRequest().path)
    }
}
