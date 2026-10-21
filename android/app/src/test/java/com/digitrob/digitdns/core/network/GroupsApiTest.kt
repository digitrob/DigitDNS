package com.digitrob.digitdns.core.network

import com.digitrob.digitdns.data.GroupsRepository
import com.digitrob.digitdns.data.PendingGroup
import com.digitrob.digitdns.domain.model.GroupAction
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
 * Custom groups. The load-bearing detail is the `items` envelope — the dashboard uses
 * `groups` and the v3 adapter renames it, so a DTO reading the wrong key yields an
 * empty list that looks like "no groups", not an error.
 */
class GroupsApiTest {

    private lateinit var server: MockWebServer
    private lateinit var repo: GroupsRepository

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
            .create(GroupsApi::class.java)
        repo = GroupsRepository(api)
    }

    @After
    fun tearDown() = server.shutdown()

    @Test
    fun listReadsTheItemsEnvelope() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"items":[{"id":"g1","name":"Streaming","action":"proxy",""" +
                    """"domains":["netflix.com"],"enabled":true,"created_at":"2026-10-20T12:00:00Z"}]}""",
            ),
        )

        val groups = repo.list()

        assertEquals(1, groups.size)
        assertEquals("Streaming", groups[0].name)
        assertEquals(GroupAction.Proxy, groups[0].action)
        assertEquals(listOf("netflix.com"), groups[0].domains)
    }

    @Test
    fun anUnknownActionFallsBackRatherThanThrowing() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"items":[{"id":"g2","name":"X","action":"something-new","domains":["x.com"],"enabled":true}]}""",
            ),
        )

        val groups = repo.list()

        assertEquals(GroupAction.Proxy, groups[0].action)
    }

    @Test
    fun createPostsTheFourEditableFields() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(201).setBody(
                """{"id":"g3","name":"Social","action":"block","domains":["tiktok.com"],"enabled":true}""",
            ),
        )

        val created = repo.create(
            PendingGroup(name = "Social", action = GroupAction.Block, domains = listOf("tiktok.com"), enabled = true),
        )

        assertEquals("g3", created.id)
        val recorded = server.takeRequest()
        assertEquals("POST", recorded.method)
        assertEquals("/custom-groups", recorded.path)
        val body = recorded.body.readUtf8()
        assertTrue(body.contains("\"action\":\"block\""))
        assertTrue(body.contains("\"domains\":[\"tiktok.com\"]"))
    }

    @Test
    fun updatePutsToTheIdPath() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"id":"g3","name":"Renamed","action":"direct","domains":["a.com"],"enabled":false}"""),
        )

        repo.update("g3", PendingGroup(name = "Renamed", action = GroupAction.Direct, domains = listOf("a.com"), enabled = false))

        assertEquals("/custom-groups/g3", server.takeRequest().path)
    }

    @Test
    fun deleteHitsTheIdPath() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"success":true}"""))

        repo.delete("g3")

        val recorded = server.takeRequest()
        assertEquals("DELETE", recorded.method)
        assertEquals("/custom-groups/g3", recorded.path)
    }
}
