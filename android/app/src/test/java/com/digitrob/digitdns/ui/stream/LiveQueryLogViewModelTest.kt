package com.digitrob.digitdns.ui.stream

import com.digitrob.digitdns.core.notifications.QueryEvent
import com.digitrob.digitdns.data.RulesRepository
import com.digitrob.digitdns.data.stream.LiveQueryStream
import com.digitrob.digitdns.data.stream.StreamStatus
import com.digitrob.digitdns.domain.model.RuleLists
import com.digitrob.digitdns.ui.components.RouteAction
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** The quick action must only mark history after the rule write actually succeeds. */
@OptIn(ExperimentalCoroutinesApi::class)
class LiveQueryLogViewModelTest {

    private val dispatcher = StandardTestDispatcher()
    private lateinit var stream: LiveQueryStream
    private lateinit var rules: RulesRepository
    private lateinit var queries: MutableStateFlow<List<QueryEvent>>
    private val row = QueryEvent(id = 42, domain = "ads.example", action = "DIRECT")

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        stream = mockk(relaxed = true)
        rules = mockk(relaxed = true)
        queries = MutableStateFlow(listOf(row))
        coEvery { rules.load() } returns RuleLists()
        io.mockk.every { stream.queries } returns queries
        io.mockk.every { stream.status } returns MutableStateFlow(StreamStatus.IDLE)
    }

    @After
    fun tearDown() = Dispatchers.resetMain()

    private fun viewModel(): LiveQueryLogViewModel = LiveQueryLogViewModel(stream, rules)

    @Test
    fun aSuccessfulSaveMarksTheHistoricalRow() = runTest(dispatcher) {
        coEvery { rules.route(any(), any(), any()) } returns true
        val model = viewModel()
        model.onResume()
        advanceUntilIdle()

        model.applyQuickAction(row.domain, RouteAction.Proxy, listOf(row.domain))
        advanceUntilIdle()

        val change = model.state.value.changeFor(row)
        assertEquals("DIRECT", change?.from)
        assertEquals("PROXY", change?.to)
        assertTrue(change!!.at > 0)
        assertEquals("DIRECT", model.state.value.buffer.first().action)
    }

    @Test
    fun aFailedOrThrowingSaveDoesNotMarkTheRow() = runTest(dispatcher) {
        coEvery { rules.route(any(), any(), any()) } returns false
        val failed = viewModel()
        failed.onResume()
        advanceUntilIdle()
        failed.applyQuickAction(row.domain, RouteAction.Block, listOf(row.domain))
        advanceUntilIdle()
        assertNull(failed.state.value.changeFor(row))

        coEvery { rules.route(any(), any(), any()) } throws IllegalStateException("offline")
        val thrown = viewModel()
        thrown.onResume()
        advanceUntilIdle()
        thrown.applyQuickAction(row.domain, RouteAction.Block, listOf(row.domain))
        advanceUntilIdle()
        assertNull(thrown.state.value.changeFor(row))
    }

    @Test
    fun removeNeverMarksAHistoricalRow() = runTest(dispatcher) {
        coEvery { rules.route(any(), any(), any()) } returns true
        val model = viewModel()
        model.onResume()
        advanceUntilIdle()

        model.applyQuickAction(row.domain, RouteAction.Remove, listOf(row.domain))
        advanceUntilIdle()

        assertNull(model.state.value.changeFor(row))
    }

    @Test
    fun aPausedRowIsMarkedAndClearingTheLogDropsMarkers() = runTest(dispatcher) {
        coEvery { rules.route(any(), any(), any()) } returns true
        val model = viewModel()
        model.onResume()
        advanceUntilIdle()
        model.setPaused(true)

        model.applyQuickAction(row.domain, RouteAction.Block, listOf(row.domain))
        advanceUntilIdle()

        assertEquals("BLOCK", model.state.value.changeFor(row)?.to)
        model.clear()
        assertNull(model.state.value.changeFor(row))
    }
}
