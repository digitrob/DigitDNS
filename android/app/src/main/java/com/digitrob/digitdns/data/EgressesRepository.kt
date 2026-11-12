package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.EgressSaveRequest
import com.digitrob.digitdns.core.network.EgressesApi
import com.digitrob.digitdns.domain.model.EgressInfo
import com.digitrob.digitdns.domain.model.EgressTestResult
import javax.inject.Inject
import javax.inject.Singleton

/** Reads and writes hub-side egresses over v3. Secrets never come back. */
@Singleton
class EgressesRepository @Inject constructor(
    private val api: EgressesApi,
) {
    suspend fun list(): List<EgressInfo> =
        api.list().egresses.map(EgressInfo::from)

    suspend fun create(body: EgressSaveRequest): EgressInfo {
        val response = api.create(body)
        return response.egress?.let(EgressInfo::from) ?: error("the server returned no egress")
    }

    suspend fun update(id: String, body: EgressSaveRequest): EgressInfo {
        val response = api.update(id, body)
        return response.egress?.let(EgressInfo::from) ?: error("the server returned no egress")
    }

    suspend fun delete(id: String) {
        api.delete(id)
    }

    suspend fun setEnabled(id: String, enabled: Boolean) {
        if (enabled) api.enable(id) else api.disable(id)
    }

    suspend fun test(id: String): EgressTestResult {
        val response = api.test(id)
        return EgressTestResult(ok = response.ok, latencyMs = response.latencyMs, error = response.error)
    }
}
