package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.core.network.ApiException
import com.digitrob.digitdns.data.SettingsRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.IOException
import java.net.Inet6Address
import java.net.InetAddress
import javax.inject.Inject

private const val HTTP_NOT_FOUND = 404
private const val HTTP_UNAVAILABLE = 503

data class Ipv6SettingsUiState(
    val enabled: Boolean = false,
    val address: String = "",
    val loaded: Boolean = false,
    val isLoading: Boolean = false,
    val isSaving: Boolean = false,
    val unsupported: Boolean = false,
    val saved: Boolean = false,
    val error: String? = null,
)

/**
 * The live proxy-IPv6 policy. The switch is the master gate for proxied AAAA
 * on the hub and is relayed to agents on their next heartbeat, where it also
 * controls whether they bind IPv6 listeners at all.
 */
@HiltViewModel
class Ipv6SettingsViewModel @Inject constructor(
    private val repository: SettingsRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(Ipv6SettingsUiState())
    val state: StateFlow<Ipv6SettingsUiState> = _state.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = true, error = null, saved = false) }
            try {
                val server = repository.load().server
                if (server == null) {
                    _state.update {
                        it.copy(isLoading = false, loaded = true, error = "The server returned no settings.")
                    }
                    return@launch
                }
                _state.update {
                    it.copy(
                        enabled = server.proxyIpv6Enabled,
                        address = server.publicIpv6,
                        loaded = true,
                        isLoading = false,
                        unsupported = false,
                        error = null,
                    )
                }
            } catch (e: ApiException) {
                _state.update {
                    if (e.httpStatus == HTTP_NOT_FOUND || e.httpStatus == HTTP_UNAVAILABLE) {
                        it.copy(isLoading = false, unsupported = true, loaded = true, error = null)
                    } else {
                        it.copy(isLoading = false, error = e.userMessage)
                    }
                }
            } catch (e: IOException) {
                _state.update {
                    it.copy(isLoading = false, error = e.message ?: "Could not load IPv6 settings.")
                }
            }
        }
    }

    fun onEnabledChange(value: Boolean) = _state.update {
        it.copy(enabled = value, saved = false, error = null)
    }

    fun onAddressChange(value: String) = _state.update {
        it.copy(address = value, saved = false, error = null)
    }

    fun save() {
        val current = _state.value
        val address = current.address.trim()
        if (current.enabled && address.isEmpty()) {
            _state.update { it.copy(error = "Enter a public IPv6 address before enabling IPv6.") }
            return
        }
        if (address.isNotEmpty() && !isNumericPublicIPv6(address)) {
            _state.update { it.copy(error = "Enter a valid public IPv6 address.") }
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(isSaving = true, error = null, saved = false) }
            try {
                val saved = repository.saveIpv6(current.enabled, address)
                _state.update {
                    it.copy(
                        enabled = saved.proxyIpv6Enabled,
                        address = saved.publicIpv6,
                        isSaving = false,
                        unsupported = false,
                        loaded = true,
                        saved = true,
                        error = null,
                    )
                }
            } catch (e: ApiException) {
                _state.update {
                    if (e.httpStatus == HTTP_NOT_FOUND || e.httpStatus == HTTP_UNAVAILABLE) {
                        it.copy(isSaving = false, unsupported = true)
                    } else {
                        it.copy(isSaving = false, error = e.userMessage)
                    }
                }
            } catch (e: IOException) {
                _state.update {
                    it.copy(isSaving = false, error = e.message ?: "Could not save IPv6 settings.")
                }
            }
        }
    }
}

private fun isNumericPublicIPv6(raw: String): Boolean {
    if (!raw.contains(':')) return false
    val parsed = runCatching { InetAddress.getByName(raw) }.getOrNull()
    return parsed is Inet6Address &&
        !parsed.isAnyLocalAddress &&
        !parsed.isLoopbackAddress &&
        !parsed.isLinkLocalAddress &&
        !parsed.isMulticastAddress
}
