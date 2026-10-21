package com.digitrob.digitdns.core.notifications

import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

/**
 * The SSE client, against a real HTTP server rather than a fake.
 *
 * What this pins that a unit test cannot: that an EventSource actually delivers the
 * server's `event:`/`data:` frames to the listener with the event name preserved, and
 * that the ticket reaches the server in the query string — the one thing the header
 * interceptor cannot do for this transport.
 */
class LiveStreamClientTest {

    private lateinit var server: MockWebServer
    private lateinit var client: LiveStreamClient

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        client = LiveStreamClient(
            OkHttpClient.Builder()
                .readTimeout(0, TimeUnit.MILLISECONDS)
                .build(),
        )
    }

    @After
    fun tearDown() = server.shutdown()

    private fun sseResponse(body: String) = MockResponse()
        .setResponseCode(200)
        .setHeader("Content-Type", "text/event-stream")
        .setBody(body)

    @Test
    fun deliversQueryFramesToTheListenerAndSendsTheTicket() {
        server.enqueue(
            sseResponse(
                "event: query\ndata: {\"domain\":\"blocked.example\",\"action\":\"BLOCK\"}\n\n",
            ),
        )

        val events = mutableListOf<SseFrame>()
        val latch = CountDownLatch(1)
        val url = server.url("/admin/api/v3/events/stream?ticket=t-1").toString()

        val source = client.open(
            url,
            object : LiveStreamClient.Listener {
                override fun onOpen() = Unit
                override fun onEvent(frame: SseFrame) {
                    events.add(frame)
                    if (events.size >= 1) latch.countDown()
                }
                override fun onClosed(retryable: Boolean, reason: String?) = Unit
            },
        )

        assertTrue("no event received", latch.await(5, TimeUnit.SECONDS))
        source.cancel()

        assertEquals("query", events.first().type)
        assertTrue(events.first().data.contains("blocked.example"))

        val request = server.takeRequest()
        assertEquals("/admin/api/v3/events/stream?ticket=t-1", request.path)
        assertEquals("text/event-stream", request.getHeader("Accept"))
    }

    @Test
    fun reportsAFailureOnANonRetryableStatus() {
        server.enqueue(MockResponse().setResponseCode(401).setBody("unauthorized"))

        val latch = CountDownLatch(1)
        var retryable = true
        val url = server.url("/admin/api/v3/events/stream?ticket=spent").toString()

        val source = client.open(
            url,
            object : LiveStreamClient.Listener {
                override fun onOpen() = Unit
                override fun onEvent(frame: SseFrame) = Unit
                override fun onClosed(retry: Boolean, reason: String?) {
                    retryable = retry
                    latch.countDown()
                }
            },
        )

        assertTrue("close not reported", latch.await(5, TimeUnit.SECONDS))
        source.cancel()

        assertEquals(false, retryable)
    }
}
