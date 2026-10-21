package com.digitrob.digitdns.ui.components

import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.ViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.data.stream.LiveQueryStream
import com.digitrob.digitdns.data.stream.StreamStatus
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.ExtendedColors
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import javax.inject.Inject

/**
 * The live-connection pill shown at the top-right of every live-backed screen.
 *
 * One word per state, shared across the dashboard, the query log, and the pages that
 * carry the badge for consistency: an operator glancing at any of them sees the same
 * vocabulary. The stream itself is the app's single shared SSE socket, so the badge is
 * a pure observer — it never opens or closes the connection.
 */
@Composable
fun ConnectionStatusBadge(
    modifier: Modifier = Modifier,
    viewModel: ConnectionStatusViewModel = hiltViewModel(),
) {
    val status by viewModel.status.collectAsStateWithLifecycle()
    ConnectionStatusBadge(status = status, modifier = modifier)
}

/** The pill itself, for callers that already hold a [StreamStatus]. */
@Composable
fun ConnectionStatusBadge(status: StreamStatus, modifier: Modifier = Modifier) {
    val colors = DigitDnsTheme.colors
    val (label, color) = connectionStatusLabel(status, colors)
    StatusBadge(text = label, color = color, modifier = modifier)
}

/** The word and tint for a connection state, shared by every badge on screen. */
fun connectionStatusLabel(
    status: StreamStatus,
    colors: ExtendedColors,
): Pair<String, Color> = when (status) {
    StreamStatus.LIVE -> "Online" to colors.success
    StreamStatus.CONNECTING -> "Connecting" to colors.warning
    StreamStatus.RETRYING -> "Reconnecting" to colors.warning
    StreamStatus.AUTH_FAILED -> "Sign in again" to colors.danger
    StreamStatus.IDLE, StreamStatus.STOPPED -> "Offline" to colors.textMuted
}

/**
 * Exposes the shared stream's status to the badge. It holds a reference to the shared
 * [LiveQueryStream] while the host screen is on screen, so a page that carries only the
 * badge (the clients list, the proxied list) still shows a live connection without
 * opening a second socket — the stream is reference-counted and shared.
 */
@HiltViewModel
class ConnectionStatusViewModel @Inject constructor(
    private val liveStream: LiveQueryStream,
) : ViewModel() {

    private val _status = MutableStateFlow(liveStream.status.value)
    val status: StateFlow<StreamStatus> = _status.asStateFlow()

    init {
        liveStream.start()
        viewModelScope.launch {
            liveStream.status.collect { _status.value = it }
        }
    }

    override fun onCleared() {
        liveStream.stop()
        super.onCleared()
    }
}
