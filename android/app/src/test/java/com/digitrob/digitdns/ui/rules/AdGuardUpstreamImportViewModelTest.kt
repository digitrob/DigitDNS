package com.digitrob.digitdns.ui.rules

import com.digitrob.digitdns.core.network.AdGuardApplyRequest
import com.digitrob.digitdns.core.network.AdGuardApplyResultDto
import com.digitrob.digitdns.core.network.AdGuardCountsDto
import com.digitrob.digitdns.core.network.AdGuardNoticeDto
import com.digitrob.digitdns.core.network.AdGuardPreviewDto
import com.digitrob.digitdns.core.network.AdGuardRowDto
import com.digitrob.digitdns.core.network.AdGuardSourceDto
import com.digitrob.digitdns.data.AdGuardSyncRepository
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class AdGuardUpstreamImportViewModelTest {

    private val dispatcher = StandardTestDispatcher()
    private lateinit var repository: AdGuardSyncRepository

    private val preview = AdGuardPreviewDto(
        sourceId = "src1",
        name = "AdGuard Home",
        text = "[/a.example/]1.1.1.1\n[/b.example/]1.1.1.1",
        checksum = "checksum-1",
        upstream = "1.1.1.1",
        counts = AdGuardCountsDto(covered = 1, adguardOnly = 2, digitdnsOnly = 1, conflict = 1),
        rows = listOf(
            row("covered.example", "covered", coveredBy = "covered.example"),
            row("a.example", "adguardOnly"),
            row("b.example", "adguardOnly"),
            row("bad.example", "conflict", coveredBy = "custom_blocked"),
        ),
        managed = listOf(row("c.example", "digitdnsOnly", target = "1.1.1.1")),
        warnings = listOf("Target is not this DigitDNS server; importing the domain will not change AdGuard routing."),
        generatedBlock = "[/c.example/]1.1.1.1",
        candidates = 4,
    )

    private fun row(
        domain: String,
        bucket: String,
        target: String = "1.1.1.1",
        coveredBy: String = "",
    ) = AdGuardRowDto(
        domain = domain,
        bucket = bucket,
        group = "Group A",
        target = target,
        targetIsSelf = false,
        coveredBy = coveredBy,
    )

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        repository = mockk(relaxed = true)
        coEvery { repository.sources() } returns emptyList()
    }

    @After
    fun tearDown() = Dispatchers.resetMain()

    private fun viewModelWithPreview(): AdGuardUpstreamImportViewModel {
        coEvery { repository.preview(any()) } returns preview
        val viewModel = AdGuardUpstreamImportViewModel(repository)
        dispatcher.scheduler.advanceUntilIdle()
        viewModel.setText(preview.text)
        viewModel.preview()
        dispatcher.scheduler.advanceUntilIdle()
        return viewModel
    }

    @Test
    fun `preview selects every importable domain and no exports by default`() = runTest(dispatcher) {
        val viewModel = viewModelWithPreview()

        assertEquals(setOf("a.example", "b.example"), viewModel.state.value.selectedImport)
        assertEquals(emptySet<String>(), viewModel.state.value.selectedManaged)
        assertEquals(preview, viewModel.state.value.preview)
    }

    @Test
    fun `preview keeps a touched selection but prunes rows that disappeared`() = runTest(dispatcher) {
        val viewModel = viewModelWithPreview()
        viewModel.clearImports()
        viewModel.toggleImport("a.example")
        coEvery { repository.preview(any()) } returns preview.copy(
            rows = preview.rows.filter { it.domain != "b.example" },
        )

        viewModel.preview(resetSelection = false)
        dispatcher.scheduler.advanceUntilIdle()

        assertEquals(setOf("a.example"), viewModel.state.value.selectedImport)
    }

    @Test
    fun `bulk controls and visible header act on the right selections`() = runTest(dispatcher) {
        val viewModel = viewModelWithPreview()

        viewModel.clearImports()
        assertEquals(emptySet<String>(), viewModel.state.value.selectedImport)

        viewModel.selectAllToSendBack()
        assertEquals(setOf("c.example"), viewModel.state.value.selectedManaged)
        viewModel.clearExports()
        assertEquals(emptySet<String>(), viewModel.state.value.selectedManaged)

        viewModel.selectAllToAdd()
        assertEquals(setOf("a.example", "b.example"), viewModel.state.value.selectedImport)
        viewModel.toggleVisibleImports(listOf("a.example", "b.example"))
        assertEquals(emptySet<String>(), viewModel.state.value.selectedImport)
        viewModel.toggleVisibleImports(listOf("a.example", "b.example"))
        assertEquals(setOf("a.example", "b.example"), viewModel.state.value.selectedImport)
    }

    @Test
    fun `filtering honours the search and selected-only toggle`() = runTest(dispatcher) {
        val viewModel = viewModelWithPreview()
        val rows = adGuardDisplayRows(preview)

        assertEquals(
            listOf("bad.example"),
            filterAdGuardRows(rows, "BAD", false, emptySet(), emptySet()).map { it.domain },
        )
        viewModel.toggleManaged("c.example")
        assertEquals(
            listOf("c.example"),
            filterAdGuardRows(rows, "", true, emptySet(), viewModel.state.value.selectedManaged).map { it.domain },
        )
    }

    @Test
    fun `apply sends the exact preview text and checksum with the staged selections`() = runTest(dispatcher) {
        val viewModel = viewModelWithPreview()
        coEvery { repository.sources() } returns listOf(AdGuardSourceDto(id = "src1", name = "AdGuard Home"))
        coEvery { repository.apply(any()) } returns AdGuardApplyResultDto(
            success = true,
            source = AdGuardSourceDto(id = "src1"),
            imported = 2,
            generatedBlock = "[/c.example/]1.1.1.1",
        )
        viewModel.toggleManaged("c.example")
        viewModel.setAcknowledgeRisk(true)
        assertTrue(canApplyAdGuardSync(viewModel.state.value))

        viewModel.apply()
        dispatcher.scheduler.advanceUntilIdle()

        val request = slot<AdGuardApplyRequest>()
        coVerify(exactly = 1) { repository.apply(capture(request)) }
        assertEquals(preview.text, request.captured.text)
        assertEquals(preview.checksum, request.captured.checksum)
        assertEquals(setOf("a.example", "b.example"), request.captured.importDomains.toSet())
        assertEquals(setOf("c.example"), request.captured.managedDomains.toSet())
        assertEquals("keep", request.captured.removalMode)
        assertTrue(request.captured.acknowledgeRisk)
        assertEquals("AdGuard sync applied to DigitDNS.", viewModel.state.value.message)
        assertEquals(setOf("a.example", "b.example"), viewModel.state.value.selectedImport)
        assertEquals(emptySet<String>(), viewModel.state.value.selectedManaged)
    }

    @Test
    fun `mirror mode needs explicit confirmation and changed text blocks apply`() = runTest(dispatcher) {
        val viewModel = viewModelWithPreview()
        viewModel.setAcknowledgeRisk(true)
        viewModel.setRemovalMode(ADGUARD_REMOVAL_MIRROR)

        viewModel.apply()
        dispatcher.scheduler.advanceUntilIdle()
        assertNotNull(viewModel.state.value.error)
        coVerify(exactly = 0) { repository.apply(any()) }

        viewModel.setConfirmMirror(true)
        viewModel.setText("edited after preview")
        assertFalse(canApplyAdGuardSync(viewModel.state.value))
        viewModel.apply()
        dispatcher.scheduler.advanceUntilIdle()
        coVerify(exactly = 0) { repository.apply(any()) }
    }

    @Test
    fun `friendliness helpers collapse warnings instead of surfacing raw strings`() = runTest(dispatcher) {
        val derived = friendlyAdGuardNotices(preview)
        assertTrue(derived.any { it.kind == "external_upstream" })
        assertTrue(derived.all { !it.message.contains("Target is not this DigitDNS server") })

        val serverNotices = preview.copy(
            notices = listOf(
                AdGuardNoticeDto(
                    kind = "external_upstream",
                    title = "Routing stays with AdGuard",
                    message = "Change the AdGuard config to move routing.",
                ),
            ),
        )
        assertEquals(1, friendlyAdGuardNotices(serverNotices).size)
        assertEquals("Routing stays with AdGuard", friendlyAdGuardNotices(serverNotices).first().title)
        assertTrue(requiresAcknowledgement(preview))
        assertEquals(0, estimatedRemovals(null, preview, ADGUARD_REMOVAL_KEEP))
    }
}
