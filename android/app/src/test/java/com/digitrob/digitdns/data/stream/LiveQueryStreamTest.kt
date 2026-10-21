package com.digitrob.digitdns.data.stream

import com.digitrob.digitdns.core.network.AuthApi
import com.digitrob.digitdns.core.network.Reachability
import com.digitrob.digitdns.core.network.SessionRefresher
import com.digitrob.digitdns.core.network.SseTicketResponse
import com.digitrob.digitdns.core.notifications.LiveStreamClient
import com.digitrob.digitdns.core.notifications.SseFrame
import com.digitrob.digitdns.core.notifications.SseFrameParser
import com.digitrob.digitdns.data.ConnectionStore
import com.digitrob.digitdns.domain.model.Connection
import io.mockk.coEvery
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.flow.flowOf
import okhttp3.sse.EventSource
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.concurrent.CopyOnWriteArrayList

/**
 * The shared socket's buffer semantics: it seeds from the history snapshot, prepends
 * live queries, and caps at 200 newest-first.
 *
 * The connect path is stubbed — a mocked [LiveStreamClient] hands back the listener the
 * stream registers — so the test drives frames directly and asserts on what the log
 * would render. No network, no Android.
 */
class LiveQueryStreamTest {

    private val connection = Connection(host = "vps.example", port = 37586, adminPath = "abc")

    private val listeners = CopyOnWriteArrayList<LiveStreamClient.Listener>()

    private fun newStream(): LiveQueryStream {
        val authApi = mockk<AuthApi>()
        coEvery { authApi.sseTicket() } returns SseTicketResponse(ticket = "one-shot")
        val store = mockk<ConnectionStore>()
        every { store.connection } returns flowOf(connection)
        every { store.activeId } returns flowOf(connection.id.ifBlank { "any" })

        val client = mockk<LiveStreamClient>()
        val eventSource = mockk<EventSource>(relaxed = true)
        val captured = slot<LiveStreamClient.Listener>()
        every { client.open(any(), capture(captured)) } answers {
            listeners.add(captured.captured)
            eventSource
        }
        return LiveQueryStream(
            authApi,
            store,
            client,
            SseFrameParser(),
            mockk<SessionRefresher>(relaxed = true),
            Reachability(),
        )
    }

    /** Wait for the async connect to register its listener. */
    private fun awaitListener(): LiveStreamClient.Listener {
        val deadline = System.currentTimeMillis() + 2_000
        while (listeners.isEmpty() && System.currentTimeMillis() < deadline) {
            Thread.sleep(10)
        }
        assertTrue("stream listener was never registered", listeners.isNotEmpty())
        return listeners.first()
    }

    private fun awaitSize(stream: LiveQueryStream, size: Int) {
        val deadline = System.currentTimeMillis() + 2_000
        while (stream.queries.value.size != size && System.currentTimeMillis() < deadline) {
            Thread.sleep(10)
        }
    }

    private fun queryFrame(id: Long): SseFrame = SseFrame(
        type = "query",
        data = """{"id":$id,"domain":"d$id.example","action":"PROXY"}""",
    )

    @Test
    fun historySeedsTheBufferNewestFirst() {
        val stream = newStream()
        stream.start()
        val listener = awaitListener()

        listener.onEvent(
            SseFrame(
                type = "history",
                data = """[{"id":1,"domain":"one.example"},{"id":2,"domain":"two.example"}]""",
            ),
        )

        awaitSize(stream, 2)
        // The server's history is oldest-first; the buffer is newest-first.
        assertEquals(listOf(2L, 1L), stream.queries.value.map { it.id })
        stream.stop()
    }

    @Test
    fun liveQueriesPrependAheadOfTheBuffer() {
        val stream = newStream()
        stream.start()
        val listener = awaitListener()

        listener.onEvent(queryFrame(1))
        awaitSize(stream, 1)
        listener.onEvent(queryFrame(2))
        awaitSize(stream, 2)

        assertEquals(listOf(2L, 1L), stream.queries.value.map { it.id })
        stream.stop()
    }

    @Test
    fun theBufferIsCappedAtTwoHundred() {
        val stream = newStream()
        stream.start()
        val listener = awaitListener()

        repeat(205) { listener.onEvent(queryFrame(it.toLong())) }
        awaitSize(stream, 200)

        assertEquals(200, stream.queries.value.size)
        // Newest is the last published (204) and the oldest kept is 5.
        assertEquals(204L, stream.queries.value.first().id)
        assertEquals(5L, stream.queries.value.last().id)
        stream.stop()
    }

    @Test
    fun clearBufferEmptiesTheRingButLeavesLaterEventsFlowing() {
        val stream = newStream()
        stream.start()
        val listener = awaitListener()

        listener.onEvent(queryFrame(1))
        awaitSize(stream, 1)

        stream.clearBuffer()
        assertEquals(0, stream.queries.value.size)

        listener.onEvent(queryFrame(2))
        awaitSize(stream, 1)
        assertEquals(listOf(2L), stream.queries.value.map { it.id })
        stream.stop()
    }
}
