package com.digitrob.digitdns.ui.dashboard

import com.digitrob.digitdns.data.AppPreferences
import com.digitrob.digitdns.data.ConnectionStore
import com.digitrob.digitdns.data.SettingsBundle
import com.digitrob.digitdns.data.SettingsRepository
import com.digitrob.digitdns.data.stream.LiveQueryStream
import com.digitrob.digitdns.data.stream.StreamStatus
import com.digitrob.digitdns.domain.model.ServerConfig
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.LiveStats
import io.mockk.coEvery
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test

/**
 * Construction is the part of this ViewModel most worth pinning down: [init] calls
 * [DashboardViewModel.loadServerIp] synchronously, so every field that function reads
 * must already be initialised when the class body reaches `init`. A counter declared
 * below the init block crashed the dashboard on launch while every other test stayed
 * green — this test exists to keep that from happening again.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class DashboardViewModelTest {

    private val dispatcher = StandardTestDispatcher()

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
    }

    @After
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun `construction succeeds and the install's public ip is fetched`() = runTest {
        val server = mockk<ServerConfig> {
            every { publicIp } returns "203.0.113.9"
            every { country } returns "NL"
        }
        val settingsRepository = mockk<SettingsRepository> {
            coEvery { load() } returns SettingsBundle(server = server, tls = null)
        }

        // The dashboard's init loops over real flows: a relaxed mock's collect
        // completing immediately would make the test read as if it had collected.
        val liveStream = mockk<LiveQueryStream>(relaxed = true) {
            every { queries } returns MutableStateFlow(emptyList())
            every { status } returns MutableStateFlow(StreamStatus.IDLE)
        }
        val appPreferences = mockk<AppPreferences>(relaxed = true) {
            every { dashboardOrder } returns MutableStateFlow(emptyList())
            every { dashboardDisabled } returns MutableStateFlow(emptySet())
        }
        val connectionStore = mockk<ConnectionStore>(relaxed = true) {
            every { activeId } returns MutableStateFlow("install-a")
        }

        val statsRepository = mockk<com.digitrob.digitdns.data.StatsRepository> {
            coEvery { live() } returns mockk<LiveStats>(relaxed = true)
        }
        val upstreamsRepository = mockk<com.digitrob.digitdns.data.UpstreamsRepository> {
            coEvery { stats() } returns emptyList()
        }

        val viewModel = DashboardViewModel(
            statsRepository = statsRepository,
            upstreamsRepository = upstreamsRepository,
            settingsRepository = settingsRepository,
            liveStream = liveStream,
            appPreferences = appPreferences,
            connectionStore = connectionStore,
        )

        dispatcher.scheduler.runCurrent()

        assertEquals("203.0.113.9", viewModel.state.value.serverIp)
        assertEquals("NL", viewModel.state.value.serverCountry)

        // The poll loop never ends on its own; cancel it so runTest's final
        // advance-until-idle does not chase delay() forever.
        viewModel.viewModelScope.cancel()
    }
}
