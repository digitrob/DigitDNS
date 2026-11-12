package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.RelayModeApi
import com.digitrob.digitdns.core.network.RelayModeDto
import com.digitrob.digitdns.core.network.RelayModeGlobalRequest
import com.digitrob.digitdns.core.network.RelayModeRequest
import com.digitrob.digitdns.core.network.RelayModeTestResponse
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class RelayModeRepository @Inject constructor(
    private val api: RelayModeApi,
) {
    suspend fun envelope(): com.digitrob.digitdns.core.network.RelayModeResponse = api.get()
    suspend fun list(): List<RelayModeDto> = api.get().relayHubs
    suspend fun load(): RelayModeDto = api.get().relayMode

    suspend fun save(
        id: String,
        name: String,
        enabled: Boolean,
        hubUrl: String,
        agentKey: String,
        token: String,
    ): List<RelayModeDto> = api.save(
        RelayModeRequest(
            id = id.ifBlank { null },
            name = name.ifBlank { null },
            enabled = enabled,
            hubUrl = hubUrl,
            agentKey = agentKey.ifBlank { null },
            token = token.ifBlank { null },
        ),
    ).relayHubs

    suspend fun delete(id: String): List<RelayModeDto> = api.delete(id).relayHubs

    suspend fun setGlobal(enabled: Boolean): List<RelayModeDto> =
        api.setGlobal(RelayModeGlobalRequest(enabled)).relayHubs

    suspend fun test(): RelayModeTestResponse = api.test()
    suspend fun testHub(id: String): RelayModeTestResponse = api.testHub(id)
}
