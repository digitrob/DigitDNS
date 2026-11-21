package com.digitrob.digitdns.core.network

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

/** The v3 icon-search path and the response shape the service editor consumes. */
class IconsApiTest {

    private lateinit var server: MockWebServer
    private lateinit var api: IconsApi

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        val json = Json { ignoreUnknownKeys = true }
        val client = OkHttpClient.Builder()
            .addInterceptor(ApiExceptionInterceptor(Reachability()))
            .build()
        api = Retrofit.Builder()
            .baseUrl(server.url("/"))
            .client(client)
            .addConverterFactory(json.asConverterFactory("application/json".toMediaType()))
            .build()
            .create(IconsApi::class.java)
    }

    @After
    fun tearDown() = server.shutdown()

    @Test
    fun searchReadsTheIconsArray() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"icons":["logos:netflix"]}"""))

        val response = api.search("netflix")

        assertEquals(listOf("logos:netflix"), response.icons)
        val request = server.takeRequest()
        assertEquals("/icons/search?q=netflix", request.path)
    }

    @Test
    fun searchEncodesMultiWordQueries() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"icons":[]}"""))

        api.search("bbc iplayer")

        assertEquals("/icons/search?q=bbc%20iplayer", server.takeRequest().path)
    }
}
