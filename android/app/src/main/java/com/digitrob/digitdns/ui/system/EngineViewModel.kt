package com.digitrob.digitdns.ui.system

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.digitrob.digitdns.data.EngineStore
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.launch
import javax.inject.Inject

@HiltViewModel
class EngineViewModel @Inject constructor(
    private val store: EngineStore,
) : ViewModel() {
    val enabled = store.enabled

    init {
        viewModelScope.launch {
            if (store.enabled.value == null) runCatching { store.refresh() }
        }
    }

    fun setEnabled(enabled: Boolean) {
        viewModelScope.launch { runCatching { store.setEnabled(enabled) } }
    }
}
