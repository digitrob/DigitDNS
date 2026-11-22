package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.AdGuardApplyRequest
import com.digitrob.digitdns.core.network.AdGuardApplyResultDto
import com.digitrob.digitdns.core.network.AdGuardPreviewDto
import com.digitrob.digitdns.core.network.AdGuardPreviewRequest
import com.digitrob.digitdns.core.network.AdGuardSourceDto
import com.digitrob.digitdns.core.network.AdGuardSourceSaveRequest
import com.digitrob.digitdns.core.network.AdGuardSyncApi
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The Android half of the AdGuard upstream-coverage sync. Parsing, coverage,
 * checksum and all rule writes live on the server; this repository only carries
 * the form to it and brings the preview back. No source URL is stored or sent
 * from this screen, and the legacy generated-block export is deliberately not
 * exposed here.
 */
@Singleton
class AdGuardSyncRepository @Inject constructor(
    private val api: AdGuardSyncApi,
) {

    suspend fun sources(): List<AdGuardSourceDto> = api.sources()

    suspend fun saveSource(request: AdGuardSourceSaveRequest): AdGuardSourceDto = api.saveSource(request)

    suspend fun deleteSource(id: String) {
        api.deleteSource(id)
    }

    suspend fun preview(request: AdGuardPreviewRequest): AdGuardPreviewDto = api.preview(request)

    suspend fun apply(request: AdGuardApplyRequest): AdGuardApplyResultDto = api.apply(request)
}
