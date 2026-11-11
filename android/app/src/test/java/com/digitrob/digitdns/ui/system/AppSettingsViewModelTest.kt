package com.digitrob.digitdns.ui.system

import androidx.fragment.app.FragmentActivity
import com.digitrob.digitdns.core.security.BiometricAuthenticator
import com.digitrob.digitdns.core.security.BiometricAvailability
import com.digitrob.digitdns.data.AppPreferences
import com.digitrob.digitdns.data.DashboardCard
import com.digitrob.digitdns.data.ThemeMode
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.just
import io.mockk.mockk
import io.mockk.runs
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class AppSettingsViewModelTest {

    private val dispatcher = StandardTestDispatcher()
    private lateinit var preferences: AppPreferences
    private lateinit var biometric: BiometricAuthenticator

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        preferences = mockk(relaxed = true)
        biometric = mockk(relaxed = true)
        every { preferences.themeMode } returns flowOf(ThemeMode.DARK)
        every { preferences.dashboardOrder } returns flowOf(DashboardCard.defaultOrder)
        every { preferences.dashboardDisabled } returns flowOf(emptySet())
        every { preferences.biometricEnabled } returns flowOf(true)
        coEvery { preferences.setBiometricEnabled(any()) } just runs
        every { biometric.availability() } returns BiometricAvailability.AVAILABLE
    }

    @After
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun biometricPreferenceIsReflectedInState() = runTest(dispatcher) {
        val viewModel = AppSettingsViewModel(preferences, biometric)
        advanceUntilIdle()

        assertTrue(viewModel.state.value.biometricEnabled)
        assertEquals(BiometricAvailability.AVAILABLE, viewModel.state.value.biometricAvailability)
    }

    @Test
    fun enablingRequiresASuccessfulPromptBeforePersisting() = runTest(dispatcher) {
        val activity = mockk<FragmentActivity>(relaxed = true)
        coEvery { biometric.authenticate(any(), any(), any()) } returns true

        val viewModel = AppSettingsViewModel(preferences, biometric)
        advanceUntilIdle()
        viewModel.setBiometricEnabled(activity, true)
        advanceUntilIdle()

        coVerify { biometric.authenticate(activity, any(), any()) }
        coVerify { preferences.setBiometricEnabled(true) }
    }

    @Test
    fun aFailedPromptDoesNotEnableTheLock() = runTest(dispatcher) {
        val activity = mockk<FragmentActivity>(relaxed = true)
        coEvery { biometric.authenticate(any(), any(), any()) } returns false
        every { preferences.biometricEnabled } returns flowOf(false)

        val viewModel = AppSettingsViewModel(preferences, biometric)
        advanceUntilIdle()
        viewModel.setBiometricEnabled(activity, true)
        advanceUntilIdle()

        coVerify(exactly = 0) { preferences.setBiometricEnabled(true) }
        assertFalse(viewModel.state.value.biometricEnabled)
    }

    @Test
    fun disablingIsImmediateAndDoesNotPrompt() = runTest(dispatcher) {
        val viewModel = AppSettingsViewModel(preferences, biometric)
        advanceUntilIdle()
        viewModel.setBiometricEnabled(null, false)
        advanceUntilIdle()

        coVerify { preferences.setBiometricEnabled(false) }
        coVerify(exactly = 0) { biometric.authenticate(any(), any(), any()) }
    }
}
