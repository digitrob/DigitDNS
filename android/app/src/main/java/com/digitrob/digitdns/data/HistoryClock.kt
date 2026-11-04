package com.digitrob.digitdns.data

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * When the aggregate history was last committed, shared across screens.
 *
 * Only the dashboard owns the history poll, but every live-backed screen carries the
 * same Online pill in the same corner. Keeping the timestamp here lets those headers
 * put the same clock directly beside that pill instead of each screen inventing a
 * second, drifting timestamp.
 */
object HistoryClock {
    private val _updatedAtEpochMs = MutableStateFlow<Long?>(null)
    val updatedAtEpochMs: StateFlow<Long?> = _updatedAtEpochMs.asStateFlow()

    /** A successful history load; null never clears the last known clock. */
    fun markUpdated(epochMs: Long) {
        _updatedAtEpochMs.value = epochMs
    }
}
