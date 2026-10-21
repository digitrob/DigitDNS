package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.UpstreamAddRequest
import com.digitrob.digitdns.core.network.UpstreamDeleteRequest
import com.digitrob.digitdns.core.network.UpstreamReorderRequest
import com.digitrob.digitdns.core.network.UpstreamStat
import com.digitrob.digitdns.core.network.UpstreamsApi
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The upstream pool. Addresses come back ordered and canonical; the client treats
 * them as opaque strings, because only the server knows the canonical form of a
 * `tls://` or `https://` entry.
 */
@Singleton
class UpstreamsRepository @Inject constructor(
    private val api: UpstreamsApi,
) {
    suspend fun list(): List<String> = api.list().items

    /** Per-upstream latency/health, already sorted fastest-first by the server. */
    suspend fun stats(): List<UpstreamStat> = api.stats().items

    suspend fun add(address: String): List<String> = api.add(UpstreamAddRequest(address)).items

    /** Remove the canonical match for [address]; returns the remaining pool. */
    suspend fun remove(address: String): List<String> =
        api.delete(UpstreamDeleteRequest(address)).items

    /** Reorder the pool. `order` must be a permutation of the current set. */
    suspend fun reorder(order: List<String>): List<String> =
        api.reorder(UpstreamReorderRequest(order)).items

    /** Start a latency benchmark; the pool refreshes as probes land. */
    suspend fun benchmark(): Boolean = api.benchmark().started
}
