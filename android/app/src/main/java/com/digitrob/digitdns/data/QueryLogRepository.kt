package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.QueryLogApi
import com.digitrob.digitdns.core.network.QueryLogResponse
import com.digitrob.digitdns.core.network.QueryLogSettingsDto
import com.digitrob.digitdns.core.network.QueryLogSettingsEnvelope
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class QueryLogRepository @Inject constructor(private val api: QueryLogApi) {
    @Suppress("LongParameterList")
    suspend fun search(
        domain: String,
        client: String,
        action: String,
        proto: String,
        qtype: String = "",
        rcode: String = "",
        cached: String = "",
        from: String = "",
        before: String = "",
        limit: Int = 100,
    ): QueryLogResponse = api.search(
        domain = domain.ifBlank { null },
        client = client.ifBlank { null },
        action = action.ifBlank { null },
        proto = proto.ifBlank { null },
        qtype = qtype.ifBlank { null },
        rcode = rcode.ifBlank { null },
        cached = cached.ifBlank { null },
        from = from.ifBlank { null },
        before = before.ifBlank { null },
        limit = limit,
    )

    suspend fun settings(): QueryLogSettingsEnvelope = api.settings()
    suspend fun save(settings: QueryLogSettingsDto): QueryLogSettingsEnvelope = api.save(settings)
    suspend fun clear() = api.clear()
}
