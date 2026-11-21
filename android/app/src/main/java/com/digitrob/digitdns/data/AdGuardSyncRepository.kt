package com.digitrob.digitdns.data

import android.content.Context
import android.net.Uri
import com.digitrob.digitdns.core.network.AdGuardApplyRequest
import com.digitrob.digitdns.core.network.AdGuardApplyResultDto
import com.digitrob.digitdns.core.network.AdGuardExportDto
import com.digitrob.digitdns.core.network.AdGuardPreviewDto
import com.digitrob.digitdns.core.network.AdGuardPreviewRequest
import com.digitrob.digitdns.core.network.AdGuardRefreshResultDto
import com.digitrob.digitdns.core.network.AdGuardSourceDto
import com.digitrob.digitdns.core.network.AdGuardSourceSaveRequest
import com.digitrob.digitdns.core.network.AdGuardSyncApi
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.IOException
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The Android half of the AdGuard upstream-coverage sync. Parsing, coverage and
 * the checksum all live on the server; this repository only carries the form to
 * it, brings the preview back, and writes the generated block to a document the
 * operator picks. No source URL is stored or sent from this screen.
 */
@Singleton
class AdGuardSyncRepository @Inject constructor(
    @ApplicationContext private val context: Context,
    private val api: AdGuardSyncApi,
) {

    suspend fun sources(): List<AdGuardSourceDto> = api.sources()

    suspend fun saveSource(request: AdGuardSourceSaveRequest): AdGuardSourceDto = api.saveSource(request)

    suspend fun deleteSource(id: String) {
        api.deleteSource(id)
    }

    suspend fun refreshSource(id: String): AdGuardRefreshResultDto = api.refreshSource(id)

    suspend fun exportSource(id: String): AdGuardExportDto = api.exportSource(id)

    suspend fun preview(request: AdGuardPreviewRequest): AdGuardPreviewDto = api.preview(request)

    suspend fun apply(request: AdGuardApplyRequest): AdGuardApplyResultDto = api.apply(request)

    /** Write the proposed AdGuard block to the document chosen through the system picker. */
    suspend fun saveBlock(uri: Uri, block: String) = withContext(Dispatchers.IO) {
        val stream = context.contentResolver.openOutputStream(uri)
            ?: throw IOException("Could not open the selected file.")
        stream.use { it.write(block.toByteArray(Charsets.UTF_8)) }
    }
}
