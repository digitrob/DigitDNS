package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.RelayModeApi
import com.digitrob.digitdns.core.network.RelayModeDto
import com.digitrob.digitdns.core.network.RelayModeRequest
import com.digitrob.digitdns.core.network.RelayModeTestResponse
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class RelayModeRepository @Inject constructor(
    private val api: RelayModeApi,
) {
    suspend fun load(): RelayModeDto = api.get().relayMode

    suspend fun save(
        enabled: Boolean,
        hubUrl: String,
        agentKey: String,
        token: String,
    ): RelayModeDto = api.save(
        RelayModeRequest(
            enabled = enabled,
            hubUrl = hubUrl,
            agentKey = agentKey.ifBlank { null },
            token = token.ifBlank { null },
        ),
    ).relayMode

    suspend fun test(): RelayModeTestResponse = api.test()
}
