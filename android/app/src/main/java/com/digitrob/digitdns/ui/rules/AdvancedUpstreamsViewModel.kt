package com.digitrob.digitdns.ui.rules

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.AdvancedUpstreamDto
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.core.network.ResolutionSaveRequest
import com.digitrob.digitdns.core.network.ResolutionSettings
import com.digitrob.digitdns.data.SettingsRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import javax.inject.Inject

private const val MIN_KEEP_WARM_SECONDS = 5
private const val MAX_KEEP_WARM_SECONDS = 600
private const val MIN_PROBATION_SECONDS = 5
private const val MAX_PROBATION_SECONDS = 300
private const val MIN_WARM_TOP = 1
private const val MAX_WARM_TOP = 100

data class AdvancedUpstreamsUiState(
    val bootstrap: String = "",
    val fallback: String = "",
    val keepWarm: String = "active_next",
    val keepWarmSeconds: String = "20",
    val hedge: String = "balanced",
    val probationSeconds: String = "30",
    val warmCache: Boolean = true,
    val warmCacheTop: String = "100",
    val loaded: Boolean = false,
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    val saved: Boolean = false,
    val restartRequired: Boolean = false,
    val error: String? = null,
)

/**
 * The upstream maintenance policy: bootstrap/fallback servers, keep-warm scope,
 * breaker cooldown and startup cache warm. It saves through the resolution
 * endpoint with the rest of the settings preserved, so a save from this screen
 * cannot wipe a strategy or a per-domain rule.
 */
@HiltViewModel
class AdvancedUpstreamsViewModel @Inject constructor(
    private val repository: SettingsRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(AdvancedUpstreamsUiState())
    val state: StateFlow<AdvancedUpstreamsUiState> = _state.asStateFlow()

    private var current: ResolutionSettings? = null

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = true, error = null, saved = false) }
            try {
                val settings = repository.loadResolution()
                current = settings
                val advanced = settings.advanced
                _state.update {
                    it.copy(
                        bootstrap = advanced.bootstrapDns.joinToString(", "),
                        fallback = advanced.fallbackDns.joinToString(", "),
                        keepWarm = advanced.keepWarm,
                        keepWarmSeconds = advanced.keepWarmSeconds.toString(),
                        hedge = advanced.hedge,
                        probationSeconds = advanced.probationSeconds.toString(),
                        warmCache = advanced.warmCacheOnStart,
                        warmCacheTop = advanced.warmCacheTop.toString(),
                        loaded = true,
                        isLoading = false,
                        restartRequired = false,
                        error = null,
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(isLoading = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update {
                    it.copy(isLoading = false, error = e.message ?: "Could not load advanced settings.")
                }
            }
        }
    }

    fun onBootstrapChange(value: String) = edit { it.copy(bootstrap = value) }
    fun onFallbackChange(value: String) = edit { it.copy(fallback = value) }
    fun onKeepWarmChange(value: String) = edit { it.copy(keepWarm = value) }
    fun onKeepWarmSecondsChange(value: String) = edit { it.copy(keepWarmSeconds = value) }
    fun onHedgeChange(value: String) = edit { it.copy(hedge = value) }
    fun onProbationChange(value: String) = edit { it.copy(probationSeconds = value) }
    fun onWarmCacheChange(value: Boolean) = edit { it.copy(warmCache = value) }
    fun onWarmCacheTopChange(value: String) = edit { it.copy(warmCacheTop = value) }

    private fun edit(block: (AdvancedUpstreamsUiState) -> AdvancedUpstreamsUiState) {
        _state.update { block(it).copy(saved = false, restartRequired = false, error = null) }
    }

    fun save() {
        val base = current
        if (base == null) {
            _state.update { it.copy(error = "Could not load the resolution settings — reopen this screen.") }
            return
        }
        val state = _state.value
        val numbers = parseNumbers(state)
        if (numbers.error != null) {
            _state.update { it.copy(error = numbers.error) }
            return
        }
        val advanced = AdvancedUpstreamDto(
            bootstrapDns = splitAddresses(state.bootstrap),
            fallbackDns = splitAddresses(state.fallback),
            keepWarm = state.keepWarm,
            keepWarmSeconds = numbers.keepWarmSeconds,
            hedge = state.hedge,
            probationSeconds = numbers.probation,
            warmCacheOnStart = state.warmCache,
            warmCacheTop = numbers.warmTop,
        )
        viewModelScope.launch {
            _state.update { it.copy(isSaving = true, error = null, saved = false) }
            try {
                val saved = repository.saveResolution(
                    ResolutionSaveRequest(
                        transport = base.transport,
                        transports = base.transports,
                        defaultUpstream = base.defaultUpstream,
                        defaultStrategy = base.defaultStrategy,
                        adaptiveIntervalMinutes = base.adaptiveIntervalMinutes,
                        profiles = base.profiles,
                        advanced = advanced,
                    ),
                )
                current = saved
                val a = saved.advanced
                _state.update {
                    it.copy(
                        bootstrap = a.bootstrapDns.joinToString(", "),
                        fallback = a.fallbackDns.joinToString(", "),
                        keepWarm = a.keepWarm,
                        keepWarmSeconds = a.keepWarmSeconds.toString(),
                        hedge = a.hedge,
                        probationSeconds = a.probationSeconds.toString(),
                        warmCache = a.warmCacheOnStart,
                        warmCacheTop = a.warmCacheTop.toString(),
                        isSaving = false,
                        loaded = true,
                        saved = true,
                        restartRequired = saved.restartRequired,
                        error = null,
                    )
                }
            } catch (e: ApiException) {
                _state.update { it.copy(isSaving = false, error = e.userMessage) }
            } catch (e: IOException) {
                _state.update {
                    it.copy(isSaving = false, error = e.message ?: "Could not save advanced settings.")
                }
            }
        }
    }

    private fun splitAddresses(value: String): List<String> =
        value.split(Regex("[\\s,]+")).map { it.trim() }.filter { it.isNotEmpty() }

    private fun parseNumbers(state: AdvancedUpstreamsUiState): ParsedNumbers {
        val keepWarmSeconds = state.keepWarmSeconds.trim().toIntOrNull()
        val probation = state.probationSeconds.trim().toIntOrNull()
        val warmTop = state.warmCacheTop.trim().toIntOrNull()
        return when {
            keepWarmSeconds == null || keepWarmSeconds !in MIN_KEEP_WARM_SECONDS..MAX_KEEP_WARM_SECONDS ->
                ParsedNumbers(error = "Keep-warm interval must be between 5 and 600 seconds.")
            probation == null || probation !in MIN_PROBATION_SECONDS..MAX_PROBATION_SECONDS ->
                ParsedNumbers(error = "Probation must be between 5 and 300 seconds.")
            warmTop == null || warmTop !in MIN_WARM_TOP..MAX_WARM_TOP ->
                ParsedNumbers(error = "Startup cache warm size must be between 1 and 100 names.")
            else -> ParsedNumbers(
                keepWarmSeconds = keepWarmSeconds,
                probation = probation,
                warmTop = warmTop,
            )
        }
    }
}

private data class ParsedNumbers(
    val keepWarmSeconds: Int = 0,
    val probation: Int = 0,
    val warmTop: Int = 0,
    val error: String? = null,
)
