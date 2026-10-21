package com.digitrob.digitdns.ui.rules

import androidx.compose.foundation.gestures.detectDragGesturesAfterLongPress
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.DragHandle
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.zIndex
import kotlin.math.roundToInt
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.digitrob.digitdns.ui.components.PanelCard
import com.digitrob.digitdns.ui.components.PanelRefreshBox
import com.digitrob.digitdns.ui.components.PanelScreenHeader
import com.digitrob.digitdns.ui.components.PanelSnackbarHost
import com.digitrob.digitdns.ui.components.SwipeToDeleteRow
import com.digitrob.digitdns.ui.components.showPanelToast
import com.digitrob.digitdns.ui.theme.DigitDnsTheme
import com.digitrob.digitdns.ui.theme.PanelMono

/**
 * The upstream resolver pool, most-preferred first. Add, reorder and remove; a removal
 * is confirmed before it happens, so a stray swipe cannot empty the pool.
 */
@Composable
fun UpstreamsScreen(viewModel: UpstreamsViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }

    LaunchedEffect(state.message) {
        val message = state.message ?: return@LaunchedEffect
        snackbar.showPanelToast(message)
        viewModel.dismissMessage()
    }

    Scaffold(snackbarHost = { PanelSnackbarHost(snackbar) }) { padding ->
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            PanelScreenHeader(showBadge = false)

            state.error?.let {
                Text(
                    it,
                    color = DigitDnsTheme.colors.danger,
                    style = MaterialTheme.typography.bodySmall,
                    modifier = Modifier.padding(horizontal = 12.dp),
                )
            }

            Row(
                modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                OutlinedTextField(
                    value = state.newAddress,
                    onValueChange = viewModel::onNewAddressChange,
                    label = { Text("Add address") },
                    placeholder = { Text("8.8.8.8, tls://dns.google") },
                    singleLine = true,
                    modifier = Modifier.weight(1f),
                )
                Spacer(Modifier.width(8.dp))
                Button(onClick = viewModel::add, enabled = !state.isSaving && state.newAddress.isNotBlank()) {
                    Text("Add")
                }
            }

            UpstreamList(state = state, viewModel = viewModel)
        }
    }
}

/** The reorderable pool list, with the pull-to-refresh wrapper the panel uses everywhere. */
@Composable
private fun UpstreamList(state: UpstreamsUiState, viewModel: UpstreamsViewModel) {
    var rowHeightPx by remember { mutableFloatStateOf(0f) }
    var draggingIndex by remember { mutableIntStateOf(NO_DRAG) }
    var dragOffset by remember { mutableFloatStateOf(0f) }

    PanelRefreshBox(
        isRefreshing = state.isLoading && state.upstreams.isNotEmpty(),
        onRefresh = viewModel::refresh,
        modifier = Modifier.fillMaxSize(),
    ) {
        when {
            state.isLoading && state.upstreams.isEmpty() -> CenterBox {
                CircularProgressIndicator(color = MaterialTheme.colorScheme.primary)
            }
            state.upstreams.isEmpty() -> CenterBox {
                Text("No upstreams configured.", color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            else -> LazyColumn(
                modifier = Modifier.fillMaxSize(),
                contentPadding = PaddingValues(12.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                itemsIndexed(state.upstreams, key = { _, address -> address }) { index, address ->
                    val isDragging = index == draggingIndex
                    UpstreamRow(
                        address = address,
                        position = index + 1,
                        isDragging = isDragging,
                        dragOffset = if (isDragging) dragOffset else 0f,
                        onMeasured = { if (rowHeightPx == 0f) rowHeightPx = it.toFloat() },
                        onRemove = { viewModel.remove(address) },
                        dragHandle = {
                            Modifier.pointerInput(state.upstreams, index, rowHeightPx) {
                                detectDragGesturesAfterLongPress(
                                    onDragStart = {
                                        draggingIndex = index
                                        dragOffset = 0f
                                    },
                                    onDragEnd = {
                                        draggingIndex = NO_DRAG
                                        dragOffset = 0f
                                    },
                                    onDragCancel = {
                                        draggingIndex = NO_DRAG
                                        dragOffset = 0f
                                    },
                                    onDrag = { change, delta ->
                                        change.consume()
                                        if (rowHeightPx <= 0f) return@detectDragGesturesAfterLongPress
                                        dragOffset += delta.y
                                        val steps = (dragOffset / rowHeightPx).roundToInt()
                                        if (steps != 0) {
                                            val current = state.upstreams.indexOf(address)
                                            val target = (current + steps).coerceIn(state.upstreams.indices)
                                            if (target != current) {
                                                viewModel.moveTo(current, target)
                                                dragOffset -= (target - current) * rowHeightPx
                                                draggingIndex = target
                                            }
                                        }
                                    },
                                )
                            }
                        },
                    )
                }
            }
        }
    }
}

private const val NO_DRAG = -1

@Composable
private fun UpstreamRow(
    address: String,
    position: Int,
    isDragging: Boolean,
    dragOffset: Float,
    onMeasured: (Int) -> Unit,
    onRemove: () -> Unit,
    dragHandle: () -> Modifier,
) {
    SwipeToDeleteRow(
        onDelete = onRemove,
        confirmTitle = "Remove upstream?",
        confirmText = "$address will be removed from the resolver pool.",
        confirmLabel = "Remove",
    ) {
        PanelCard(
            modifier = Modifier
                .fillMaxWidth()
                .onSizeChanged { onMeasured(it.height) }
                .zIndex(if (isDragging) 1f else 0f)
                .graphicsLayer { translationY = dragOffset },
        ) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(
                    Icons.Filled.DragHandle,
                    contentDescription = "Drag to reorder $address",
                    tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.size(24.dp).then(dragHandle()),
                )
                Spacer(Modifier.width(8.dp))
                Text(
                    text = "$position",
                    style = MaterialTheme.typography.titleMedium.copy(fontFamily = PanelMono),
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.width(28.dp),
                )
                Text(
                    text = address,
                    style = MaterialTheme.typography.bodyLarge.copy(fontFamily = PanelMono),
                    color = MaterialTheme.colorScheme.onSurface,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f),
                )
            }
        }
    }
}

@Composable
private fun CenterBox(content: @Composable () -> Unit) {
    Column(
        modifier = Modifier.fillMaxSize(),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) { content() }
}
