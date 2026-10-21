package com.digitrob.digitdns.core.notifications

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The stream's wire contract, tested without a socket.
 *
 * Two event names carry the whole stream (`query` and `history`); everything else —
 * a keep-alive, a future event a newer server adds, a malformed payload — must be
 * dropped rather than guessed at, because a half-decoded event would surface as a
 * wrong notification.
 */
class SseFrameParserTest {

    private val parser = SseFrameParser()

    @Test
    fun parsesAQueryFrame() {
        val frame = SseFrame(
            type = "query",
            data = """{"id":7,"client_ip":"10.0.0.9","account_name":"Kids","protocol":"DoH",
                "domain":"ads.example","rule_matched":"ads","action":"BLOCK","latency_ms":3.5,"cached":false}""",
        )

        val event = parser.parse(frame)

        assertTrue(event is StreamEvent.Query)
        val query = (event as StreamEvent.Query).event
        assertEquals("ads.example", query.domain)
        assertEquals("BLOCK", query.action)
        assertEquals(3.5, query.latencyMs, 0.001)
    }

    @Test
    fun parsesAHistoryFrame() {
        val frame = SseFrame(
            type = "history",
            data = """[{"domain":"a.example","action":"DIRECT"},{"domain":"b.example","action":"BLOCK"}]""",
        )

        val event = parser.parse(frame)

        assertTrue(event is StreamEvent.History)
        assertEquals(2, (event as StreamEvent.History).events.size)
    }

    @Test
    fun anEmptyHistoryArrayIsAValidFrame() {
        val event = parser.parse(SseFrame(type = "history", data = "[]"))

        assertTrue(event is StreamEvent.History)
        assertEquals(0, (event as StreamEvent.History).events.size)
    }

    @Test
    fun ignoresAnUnknownEventName() {
        assertNull(parser.parse(SseFrame(type = "stats", data = """{"qps":1}""")))
    }

    @Test
    fun ignoresAFrameWithNoEventName() {
        // OkHttp strips `: comment` keep-alives, but a bare data frame with no event
        // name is still possible and is not one of ours.
        assertNull(parser.parse(SseFrame(type = null, data = "hello")))
    }

    @Test
    fun ignoresBlankData() {
        assertNull(parser.parse(SseFrame(type = "query", data = "")))
        assertNull(parser.parse(SseFrame(type = "query", data = "   ")))
    }

    @Test
    fun aMalformedQueryPayloadIsDroppedNotCrashing() {
        assertNull(parser.parse(SseFrame(type = "query", data = "{not json")))
    }

    @Test
    fun unknownFieldsInAQueryAreIgnored() {
        val frame = SseFrame(
            type = "query",
            data = """{"domain":"x.example","action":"PROXY","future_field":42}""",
        )

        val event = parser.parse(frame) as StreamEvent.Query

        assertEquals("x.example", event.event.domain)
    }
}
