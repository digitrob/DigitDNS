package com.digitrob.digitdns.ui.stream

import com.digitrob.digitdns.core.notifications.QueryEvent
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * The Live Query Log's view derivation: filter, search, sort and pause.
 *
 * These are what the operator actually scans for — "show me the blocked ones", "where
 * is that domain", "newest first" — so they are pure and pinned here rather than left
 * to the composable.
 */
class LiveQueryLogStateTest {

    private fun event(
        id: Long,
        domain: String,
        action: String = "PROXY",
    ) = QueryEvent(id = id, domain = domain, action = action)

    private val buffer = listOf(
        event(3, "c.example", "BLOCK"),
        event(2, "b.google.com", "PROXY"),
        event(1, "a.example", "DIRECT"),
    )

    @Test
    fun newestFirstIsTheDefaultOrder() {
        val state = LiveQueryLogUiState(buffer = buffer)
        assertEquals(listOf(3L, 2L, 1L), state.visible.map { it.id })
    }

    @Test
    fun oldestSortReversesTheOrder() {
        val state = LiveQueryLogUiState(buffer = buffer, sort = LogSort.Oldest)
        assertEquals(listOf(1L, 2L, 3L), state.visible.map { it.id })
    }

    @Test
    fun theActionFilterKeepsOnlyMatchingRows() {
        val state = LiveQueryLogUiState(buffer = buffer, actionFilter = ActionFilter.Block)
        assertEquals(listOf(3L), state.visible.map { it.id })
    }

    @Test
    fun searchMatchesTheDomainCaseInsensitively() {
        val state = LiveQueryLogUiState(buffer = buffer, search = "GOOGLE")
        assertEquals(listOf(2L), state.visible.map { it.id })
    }

    @Test
    fun searchAndFilterCompose() {
        val state = LiveQueryLogUiState(
            buffer = buffer,
            search = "example",
            actionFilter = ActionFilter.Block,
        )
        assertEquals(listOf(3L), state.visible.map { it.id })
    }

    @Test
    fun pauseFreezesTheSnapshotAgainstLaterBufferChanges() {
        val live = LiveQueryLogUiState(buffer = buffer)
        val paused = live.copy(paused = true, pausedBuffer = live.buffer)

        // The stream keeps buffering underneath — a new row arrives.
        val advanced = paused.copy(buffer = listOf(event(4, "d.example")) + buffer)

        // The view still shows the frozen snapshot, not the new row.
        assertEquals(listOf(3L, 2L, 1L), advanced.visible.map { it.id })
    }

    @Test
    fun resumingAdoptsTheCurrentBuffer() {
        val resumed = LiveQueryLogUiState(
            buffer = listOf(event(4, "d.example")),
            paused = false,
            pausedBuffer = null,
        )
        assertEquals(listOf(4L), resumed.visible.map { it.id })
        assertNull(resumed.pausedBuffer)
    }

    @Test
    fun ipv6SinkHasAFriendlyLabelAndProxyTone() {
        val style = queryActionStyle("PROXY_IPV6_SINK")
        assertEquals("Proxy IPv6 Sink", style.label)
        assertEquals(QueryActionTone.Info, style.tone)
    }

    @Test
    fun unknownActionDropsUnderscores() {
        assertEquals("Future Action", queryActionStyle("FUTURE_ACTION").label)
        assertEquals(QueryActionTone.Muted, queryActionStyle("FUTURE_ACTION").tone)
    }

    @Test
    fun proxyFilterIncludesEveryProxyOutcome() {
        val events = listOf(
            event(1, "a", "PROXY"),
            event(2, "b", "PROXY_IPV6_SINK"),
            event(3, "c", "PROXY_FAILED"),
            event(4, "d", "DIRECT"),
        )
        val state = LiveQueryLogUiState(buffer = events, actionFilter = ActionFilter.Proxy)
        assertEquals(listOf(1L, 2L, 3L), state.visible.map { it.id })
    }

    @Test
    fun blockAndDirectFiltersIncludeTheirVariants() {
        val events = listOf(
            event(1, "a", "BLOCK"),
            event(2, "b", "RATELIMIT"),
            event(3, "c", "QUOTA"),
            event(4, "d", "DIRECT"),
            event(5, "e", "CACHED"),
            event(6, "f", "STALE"),
        )
        assertEquals(
            listOf(1L, 2L, 3L),
            LiveQueryLogUiState(buffer = events, actionFilter = ActionFilter.Block).visible.map { it.id },
        )
        assertEquals(
            listOf(4L, 5L, 6L),
            LiveQueryLogUiState(buffer = events, actionFilter = ActionFilter.Direct).visible.map { it.id },
        )
    }
}
