package com.digitrob.digitdns.data

import android.content.Context
import android.database.Cursor
import android.database.sqlite.SQLiteDatabase
import android.net.Uri
import android.provider.OpenableColumns
import com.digitrob.digitdns.core.network.MigrateAnalyzeRequest
import com.digitrob.digitdns.core.network.MigrateApi
import com.digitrob.digitdns.core.network.MigrateApplyRequest
import com.digitrob.digitdns.core.network.MigrateApplyResultDto
import com.digitrob.digitdns.core.network.MigrateBundleDto
import com.digitrob.digitdns.core.network.MigrateClientDto
import com.digitrob.digitdns.core.network.MigrateFilterListDto
import com.digitrob.digitdns.core.network.MigrateFilteringDto
import com.digitrob.digitdns.core.network.MigratePreviewDto
import com.digitrob.digitdns.core.network.MigrateSourceDto
import com.digitrob.digitdns.core.network.MigrateUndoRequest
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.MultipartBody
import okhttp3.RequestBody.Companion.toRequestBody
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.File
import java.io.IOException
import java.util.zip.ZipInputStream
import javax.inject.Inject
import javax.inject.Singleton

private const val BYTES_PER_MB = 1024 * 1024
private const val MAX_IMPORT_BYTES = 64 * BYTES_PER_MB
private const val MAX_ARCHIVE_BYTES = 256 * BYTES_PER_MB
private const val MAX_TOML_BYTES = 1 shl 20
private const val MAX_REPORT_BYTES = 16 * BYTES_PER_MB
private const val READ_BUFFER_BYTES = 8192
private const val DOMAINLIST_ALLOW = 0
private const val DOMAINLIST_BLOCK = 1
private const val DOMAINLIST_REGEX_ALLOW = 2
private const val DOMAINLIST_REGEX_BLOCK = 3

/**
 * The importer's Android half. AdGuard YAML and Pi-hole v5 Teleporter archives
 * go to the server for parsing; a v6 Teleporter is inspected locally so the
 * gravity database is read on the device. Either way the server receives only
 * the normalized, secret-free bundle it already previews in the web panel.
 */
@Singleton
class MigrateRepository @Inject constructor(
    @ApplicationContext private val context: Context,
    private val api: MigrateApi,
) {
    suspend fun analyze(uri: Uri): MigratePreviewDto = withContext(Dispatchers.IO) {
        val name = displayName(uri) ?: "import"
        val bytes = readBytes(uri)
        if (name.endsWith(".zip", ignoreCase = true)) {
            val zip = readZip(bytes)
            if (zip.gravityDb != null || zip.piholeToml != null) {
                api.analyzeBundle(
                    MigrateAnalyzeRequest(
                        bundle = buildV6Bundle(zip, name),
                        piholeToml = zip.piholeToml.orEmpty(),
                    ),
                )
            } else {
                api.analyzeFile(filePart(name, bytes))
            }
        } else {
            api.analyzeFile(filePart(name, bytes))
        }
    }

    suspend fun apply(token: String, upstreams: Boolean): MigrateApplyResultDto =
        api.apply(MigrateApplyRequest(token = token, upstreams = upstreams))

    suspend fun undo(token: String) {
        api.undo(MigrateUndoRequest(token))
    }

    /**
     * Fetch the skipped-regex report for [token] and write it to the operator's
     * chosen document. The report stays on the device, like the source export.
     */
    suspend fun saveReport(uri: Uri, token: String) = withContext(Dispatchers.IO) {
        val bytes = api.report(token).use { body ->
            readLimited(body.byteStream(), MAX_REPORT_BYTES)
        }
        val stream = context.contentResolver.openOutputStream(uri)
            ?: throw IOException("Could not open the selected file.")
        stream.use { it.write(bytes) }
    }

    private fun filePart(name: String, bytes: ByteArray): MultipartBody.Part =
        MultipartBody.Part.createFormData(
            "file",
            name,
            bytes.toRequestBody("application/octet-stream".toMediaType()),
        )

    private fun displayName(uri: Uri): String? {
        return runCatching {
            context.contentResolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)
                ?.use { cursor ->
                    if (!cursor.moveToFirst()) return@use null
                    val index = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
                    if (index >= 0) cursor.getString(index) else null
                }
        }.getOrNull()
    }

    private fun readBytes(uri: Uri): ByteArray {
        val stream = context.contentResolver.openInputStream(uri)
            ?: throw IOException("Could not open the selected file.")
        return stream.use { readLimited(it, MAX_IMPORT_BYTES) }
    }

    private fun readLimited(input: java.io.InputStream, max: Int): ByteArray {
        val out = ByteArrayOutputStream()
        val buffer = ByteArray(READ_BUFFER_BYTES)
        var total = 0
        while (true) {
            val read = input.read(buffer)
            if (read < 0) break
            total += read
            if (total > max) throw IOException("That file is larger than ${max / BYTES_PER_MB} MB.")
            out.write(buffer, 0, read)
        }
        return out.toByteArray()
    }

    private data class ZipContents(
        val gravityDb: ByteArray?,
        val piholeToml: String?,
    )

    @Suppress("NestedBlockDepth")
    private fun readZip(bytes: ByteArray): ZipContents {
        var gravity: ByteArray? = null
        var toml: String? = null
        var total = 0
        ZipInputStream(ByteArrayInputStream(bytes)).use { zip ->
            while (true) {
                val entry = zip.nextEntry ?: break
                if (!entry.isDirectory) {
                    val lower = entry.name.lowercase()
                    when {
                        lower.endsWith("gravity.db") -> {
                            gravity = readLimited(zip, MAX_ARCHIVE_BYTES)
                            total += gravity?.size ?: 0
                        }
                        lower.endsWith("pihole.toml") -> {
                            val body = readLimited(zip, MAX_TOML_BYTES)
                            total += body.size
                            toml = body.toString(Charsets.UTF_8)
                        }
                    }
                    if (total > MAX_ARCHIVE_BYTES) {
                        throw IOException("That Teleporter archive expands past ${MAX_ARCHIVE_BYTES / BYTES_PER_MB} MB.")
                    }
                }
            }
        }
        return ZipContents(gravity, toml)
    }

    private fun buildV6Bundle(zip: ZipContents, name: String): MigrateBundleDto {
        val lists = mutableListOf<MigrateFilterListDto>()
        val blocked = mutableListOf<String>()
        val allowlist = mutableListOf<String>()
        val clients = mutableListOf<MigrateClientDto>()
        val rules = mutableListOf<String>()

        zip.gravityDb?.let { body ->
            val file = File.createTempFile("digitdns-gravity-", ".db", context.cacheDir)
            try {
                file.writeBytes(body)
                SQLiteDatabase.openDatabase(file.path, null, SQLiteDatabase.OPEN_READONLY).use { db ->
                    query(db, "SELECT address, enabled, comment FROM adlist") { cursor ->
                        val url = cursor.stringOr("address")
                        if (url.isNotBlank()) {
                            lists += MigrateFilterListDto(
                                name = cursor.stringOr("comment").ifBlank { url },
                                url = url,
                                enabled = cursor.intOr("enabled", 1) != 0,
                            )
                        }
                    }
                    query(db, "SELECT type, domain, enabled FROM domainlist") { cursor ->
                        val domain = cursor.stringOr("domain").trim()
                        if (domain.isEmpty()) return@query
                        when (cursor.intOr("type", DOMAINLIST_BLOCK)) {
                            DOMAINLIST_ALLOW -> allowlist += domain
                            DOMAINLIST_BLOCK -> blocked += domain
                            DOMAINLIST_REGEX_ALLOW, DOMAINLIST_REGEX_BLOCK -> {
                                val pattern = if (domain.startsWith("/") && domain.endsWith("/")) {
                                    domain
                                } else {
                                    "/" + domain + "/"
                                }
                                rules += if (cursor.intOr("type", DOMAINLIST_BLOCK) == DOMAINLIST_REGEX_ALLOW) {
                                    "@@$pattern"
                                } else {
                                    pattern
                                }
                            }
                        }
                    }
                    query(db, "SELECT ip, name, comment FROM client") { cursor ->
                        val ids = cursor.stringOr("ip")
                            .split(Regex("[,\\s]+"))
                            .filter { it.isNotBlank() }
                        if (ids.isNotEmpty()) {
                            clients += MigrateClientDto(
                                name = cursor.stringOr("name").ifBlank { ids.first() },
                                ids = ids,
                                enabled = true,
                                note = cursor.stringOr("comment"),
                            )
                        }
                    }
                }
            } finally {
                file.delete()
            }
        }

        return MigrateBundleDto(
            source = MigrateSourceDto(kind = "pihole-v6", version = "v6", fileName = name),
            filterLists = lists,
            blocked = blocked,
            allowlist = allowlist,
            clients = clients,
            filtering = if (rules.isEmpty()) null else MigrateFilteringDto(
                customRules = rules.joinToString("\n"),
            ),
        )
    }

    private inline fun query(db: SQLiteDatabase, sql: String, row: (Cursor) -> Unit) {
        runCatching {
            db.rawQuery(sql, null).use { cursor ->
                while (cursor.moveToNext()) row(cursor)
            }
        }
    }

    private fun Cursor.stringOr(column: String): String {
        val index = getColumnIndex(column)
        return if (index >= 0 && !isNull(index)) getString(index).orEmpty() else ""
    }

    private fun Cursor.intOr(column: String, fallback: Int): Int {
        val index = getColumnIndex(column)
        return if (index >= 0 && !isNull(index)) getInt(index) else fallback
    }
}
