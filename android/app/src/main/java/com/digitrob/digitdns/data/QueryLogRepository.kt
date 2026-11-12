package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.QueryLogApi
import com.digitrob.digitdns.core.network.QueryLogItemDto
import com.digitrob.digitdns.core.network.QueryLogSettingsDto
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class QueryLogRepository @Inject constructor(private val api: QueryLogApi) {
    suspend fun search(domain: String, client: String, action: String, proto: String): List<QueryLogItemDto> =
        api.search(domain.ifBlank { null }, client.ifBlank { null }, action.ifBlank { null }, proto.ifBlank { null }).items

    suspend fun settings(): QueryLogSettingsDto = api.settings().settings
    suspend fun save(settings: QueryLogSettingsDto): QueryLogSettingsDto = api.save(settings).settings
    suspend fun clear() = api.clear()
}
