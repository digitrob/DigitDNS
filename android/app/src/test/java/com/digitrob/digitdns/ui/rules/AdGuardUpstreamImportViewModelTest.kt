package com.digitrob.digitdns.ui.rules

import com.digitrob.digitdns.core.network.AdGuardApplyRequest
import com.digitrob.digitdns.core.network.AdGuardApplyResultDto
import com.digitrob.digitdns.core.network.AdGuardCountsDto
import com.digitrob.digitdns.core.network.AdGuardPreviewDto
import com.digitrob.digitdns.core.network.AdGuardPreviewRequest
import com.digitrob.digitdns.core.network.AdGuardRowDto
import com.digitrob.digitdns.core.network.AdGuardSourceDto
import com.digitrob.digitdns.core.network.AdGuardSourceSaveRequest
import com.digitrob.digitdns.data.AdGuardSyncRepository
import com.digitrob.digitdns.data.RulesRepository
import com.digitrob.digitdns.domain.model.RuleLists
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
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
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
    private lateinit var rulesRepository: RulesRepository

    private val preview = AdGuardPreviewDto(
        sourceId = "src1",
        name = "AdGuard Home",
        text = "[/a.example/b.example/]1.1.1.1",
        checksum = "checksum-1",
        upstream = "1.1.1.1",
        counts = AdGuardCountsDto(covered = 1, adguardOnly = 2, conflict = 1),
        rows = listOf(
            row("covered.example", "covered", coveredBy = "covered.example"),
            row("a.example", "adguardOnly"),
            row("b.example", "adguardOnly"),
            row("bad.example", "conflict", coveredBy = "custom_blocked"),
            row("*.wild.example", "adguardOnly"),
            row("digitdns.example", "digitdnsOnly"),
        ),
        candidates = 5,
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
        suggestedAction = "proxy",
    )

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        repository = mockk(relaxed = true)
        rulesRepository = mockk(relaxed = true)
        coEvery { rulesRepository.load() } returns RuleLists(proxied = listOf("existing.example"))
    }

    @After
    fun tearDown() = Dispatchers.resetMain()

    private fun viewModelWithPreview(): AdGuardUpstreamImportViewModel {
        coEvery { repository.preview(any()) } returns preview
        val viewModel = AdGuardUpstreamImportViewModel(repository, rulesRepository)
        dispatcher.scheduler.advanceUntilIdle()
        viewModel.setText(preview.text)
        viewModel.preview()
        dispatcher.scheduler.advanceUntilIdle()
        return viewModel
    }

    @Test
    fun `preview selects every importable domain and loads DigitDNS patterns`() = runTest(dispatcher) {
        val viewModel = viewModelWithPreview()

        assertEquals(
            setOf("a.example", "b.example", "*.wild.example"),
            viewModel.state.value.selectedImport,
        )
        assertEquals(preview, viewModel.state.value.preview)
        assertEquals(setOf("existing.example"), viewModel.state.value.digitDnsPatterns)
        assertEquals(AdGuardFilter.All, viewModel.state.value.filter)
    }

    @Test
    fun `selection helpers keep a touched selection and prune rows that disappear`() = runTest(dispatcher) {
        val viewModel = viewModelWithPreview()
        viewModel.toggleVisibleImports(listOf("a.example", "b.example", "*.wild.example"))
        viewModel.toggleImport("a.example")
        coEvery { repository.preview(any()) } returns preview.copy(
            rows = preview.rows.filter { it.domain != "b.example" },
        )

        viewModel.preview(resetSelection = false)
        dispatcher.scheduler.advanceUntilIdle()

        assertEquals(setOf("a.example"), viewModel.state.value.selectedImport)

        // A header tick selects the whole visible set; the next one clears it.
        viewModel.toggleVisibleImports(listOf("a.example", "*.wild.example"))
        assertEquals(setOf("a.example", "*.wild.example"), viewModel.state.value.selectedImport)
        viewModel.toggleVisibleImports(listOf("a.example", "*.wild.example"))
        assertEquals(emptySet<String>(), viewModel.state.value.selectedImport)
    }

    @Test
    fun `review filters are mutually exclusive and repeat clicks clear them`() = runTest(dispatcher) {
        val viewModel = viewModelWithPreview()
        val rows = adGuardDisplayRows(preview)

        viewModel.setFilter(AdGuardFilter.AlreadyPresent)
        assertEquals(AdGuardFilter.AlreadyPresent, viewModel.state.value.filter)
        assertEquals(
            listOf("covered.example"),
            filterAdGuardRows(rows, "", AdGuardFilter.AlreadyPresent, emptySet()).map { it.domain },
        )

        viewModel.setFilter(AdGuardFilter.SelectedOnly)
        assertEquals(AdGuardFilter.SelectedOnly, viewModel.state.value.filter)
        assertEquals(
            listOf("a.example", "b.example", "*.wild.example"),
            filterAdGuardRows(
                rows,
                "",
                AdGuardFilter.SelectedOnly,
                viewModel.state.value.selectedImport,
            ).map { it.domain },
        )

        viewModel.setFilter(AdGuardFilter.SelectedOnly)
        assertEquals(AdGuardFilter.All, viewModel.state.value.filter)
        assertFalse(
            filterAdGuardRows(rows, "", AdGuardFilter.All, emptySet())
                .any { it.bucket == "digitdnsOnly" },
        )
        assertEquals(
            listOf("bad.example"),
            filterAdGuardRows(rows, "BAD", AdGuardFilter.All, emptySet()).map { it.domain },
        )
    }

    @Test
    fun `a staged wildcard scope replaces its origin in import and actions`() = runTest(dispatcher) {
        val viewModel = viewModelWithPreview()
        viewModel.setRoute("a.example", ADGUARD_ACTION_DIRECT, listOf("*.a.example"))

        assertEquals(setOf("b.example", "*.wild.example", "*.a.example"), adGuardActiveTargets(viewModel.state.value))
        assertEquals("direct", viewModel.state.value.domainActions["*.a.example"])
        assertFalse("a.example" in viewModel.state.value.domainActions)
        assertTrue(viewModel.state.value.reviewDirty)

        viewModel.setBulkRoute(ADGUARD_ACTION_BLOCK)
        assertEquals("block", viewModel.state.value.domainActions["*.a.example"])
        assertFalse("a.example" in viewModel.state.value.domainActions)
    }

    @Test
    fun `a listed wildcard keeps its exact pattern and offers no derived scope`() {
        assertEquals(
            listOf("*.wild.example"),
            adGuardScopeTargets("*.wild.example", MatchScope.Exact),
        )
        assertEquals(emptyList<String>(), adGuardScopeTargets("*.wild.example", MatchScope.Host))
        assertEquals(emptyList<String>(), adGuardScopeTargets("*.wild.example", MatchScope.Parent))
    }

    @Test
    fun `explicit covered row edits ride in apply without becoming imports`() = runTest(dispatcher) {
        val viewModel = viewModelWithPreview()
        coEvery { repository.sources() } returns listOf(AdGuardSourceDto(id = "src1", name = "AdGuard Home"))
        coEvery { repository.apply(any()) } returns AdGuardApplyResultDto(
            success = true,
            source = AdGuardSourceDto(id = "src1"),
        )

        viewModel.toggleVisibleImports(listOf("a.example", "b.example", "*.wild.example"))
        viewModel.setRoute("covered.example", ADGUARD_ACTION_BLOCK, listOf("covered.example"))
        assertTrue(viewModel.state.value.existingRoutes.contains("covered.example"))
        assertTrue(viewModel.state.value.selectedImport.isEmpty())
        assertTrue(canApplyAdGuardSync(viewModel.state.value))

        viewModel.apply()
        dispatcher.scheduler.advanceUntilIdle()

        val request = slot<AdGuardApplyRequest>()
        coVerify(exactly = 1) { repository.apply(capture(request)) }
        assertEquals(listOf("covered.example"), request.captured.importDomains)
        assertEquals("block", request.captured.domainActions["covered.example"])
        // Apply ends with a fresh preview, so the staged edit is intentionally cleared.
        assertEquals(ADGUARD_ACTION_PROXY, adGuardRowAction(preview.rows.first(), viewModel.state.value))
    }

    @Test
    fun `apply sends exact preview text, checksum and add-only guardrails`() = runTest(dispatcher) {
        val viewModel = viewModelWithPreview()
        coEvery { repository.sources() } returns listOf(AdGuardSourceDto(id = "src1", name = "AdGuard Home"))
        coEvery { repository.apply(any()) } returns AdGuardApplyResultDto(
            success = true,
            source = AdGuardSourceDto(id = "src1"),
            imported = 3,
        )

        viewModel.setRoute("a.example", ADGUARD_ACTION_BLOCK, listOf("a.example"))
        assertTrue(viewModel.state.value.reviewDirty)
        assertTrue(canApplyAdGuardSync(viewModel.state.value))

        viewModel.apply()
        dispatcher.scheduler.advanceUntilIdle()

        coVerify(atLeast = 2) { repository.preview(any()) }
        val request = slot<AdGuardApplyRequest>()
        coVerify(exactly = 1) { repository.apply(capture(request)) }
        assertEquals(preview.text, request.captured.text)
        assertEquals(preview.checksum, request.captured.checksum)
        assertEquals("src1", request.captured.sourceId)
        assertEquals("keep", request.captured.removalMode)
        assertFalse(request.captured.confirmMirror)
        assertTrue(request.captured.acknowledgeRisk)
        assertEquals("block", request.captured.domainActions["a.example"])
        assertFalse(viewModel.state.value.reviewDirty)
        assertEquals("AdGuard upstream list applied to DigitDNS.", viewModel.state.value.message)
    }

    @Test
    fun `changed source text blocks stale apply and starts a fresh preview`() = runTest(dispatcher) {
        val viewModel = viewModelWithPreview()
        viewModel.setText("edited after preview")
        assertFalse(canApplyAdGuardSync(viewModel.state.value))

        viewModel.apply()
        dispatcher.scheduler.advanceUntilIdle()

        coVerify(exactly = 0) { repository.apply(any()) }
        coVerify(atLeast = 2) { repository.preview(any()) }
        assertNotNull(viewModel.state.value.message)
    }

    @Test
    fun `delete uses the confirmation state and keeps imported domains on the server`() = runTest(dispatcher) {
        coEvery { repository.sources() } returns listOf(AdGuardSourceDto(id = "src1", name = "AdGuard Home"))
        val viewModel = AdGuardUpstreamImportViewModel(repository, rulesRepository)
        dispatcher.scheduler.advanceUntilIdle()

        // Like the web card, startup does not auto-load a saved list; picking it does.
        assertEquals("", viewModel.state.value.selectedSourceId)
        viewModel.loadSource("src1")
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals("src1", viewModel.state.value.selectedSourceId)
        viewModel.requestDelete()
        assertTrue(viewModel.state.value.deleteConfirmation)
        viewModel.confirmDelete()
        dispatcher.scheduler.advanceUntilIdle()

        coVerify(exactly = 1) { repository.deleteSource("src1") }
        assertFalse(viewModel.state.value.deleteConfirmation)
        assertEquals("", viewModel.state.value.selectedSourceId)
        assertEquals("List deleted", viewModel.state.value.status)
    }

    @Test
    fun `saved count parses bracketed groups and bare wildcard rules`() {
        assertEquals(
            3,
            adGuardSourceDomainCount(
                AdGuardSourceDto(
                    text = "[a.example/b.example]tls://dns.example\n||*.c.example^\n!comment\n# note",
                ),
            ),
        )
    }

    @Test
    fun `request payloads never send enabled and always acknowledge risk`() {
        val json = Json { encodeDefaults = false }
        val save = json.encodeToString(AdGuardSourceSaveRequest(sourceId = "s", name = "n", text = "t"))
        assertFalse(save.contains("enabled"))
        assertFalse(save.contains("managed_upstream"))

        val previewRequest = json.encodeToString(
            AdGuardPreviewRequest(sourceId = "s", name = "n", text = "t"),
        )
        assertFalse(previewRequest.contains("enabled"))
        assertTrue(previewRequest.contains("\"acknowledge_risk\":true"))
        assertTrue(previewRequest.contains("\"removal_mode\":\"keep\""))

        val apply = json.encodeToString(
            AdGuardApplyRequest(sourceId = "s", name = "n", text = "t", checksum = "c"),
        )
        assertFalse(apply.contains("enabled"))
        assertFalse(apply.contains("managed_upstream"))
        assertFalse(apply.contains("adblock_domains"))
        assertTrue(apply.contains("\"acknowledge_risk\":true"))
        assertTrue(apply.contains("\"domain_actions\":{}"))
    }
}
