package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.SubscriberErrorDto
import com.digitrob.digitdns.core.network.SubscriberRegisterRequest
import com.digitrob.digitdns.core.network.SubscriberRegisterResponse
import com.digitrob.digitdns.core.network.SubscriberStatusDto
import com.digitrob.digitdns.core.security.SecretStore
import com.digitrob.digitdns.core.security.TlsTrustFailure
import com.digitrob.digitdns.core.security.TofuTrustManager
import com.digitrob.digitdns.domain.model.SubscriberLink
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import timber.log.Timber
import java.io.IOException
import java.net.HttpURLConnection
import java.security.SecureRandom
import java.util.concurrent.TimeUnit
import javax.inject.Inject
import javax.inject.Singleton
import javax.net.ssl.SSLContext
import javax.net.ssl.SSLPeerUnverifiedException
import javax.net.ssl.X509TrustManager

/** The three outcomes the subscriber screens care about. */
sealed interface SubscriberResult<out T> {
    data class Ok<T>(val value: T) : SubscriberResult<T>

    /** A user-actionable failure, already phrased for the screen. */
    data class Failed(val message: String) : SubscriberResult<Nothing>

    /** The certificate is unknown or changed; the caller shows the trust prompt. */
    data class TrustRequired(val fingerprint: String, val changedFrom: String?) : SubscriberResult<Nothing>
}

/**
 * The subscriber portal API.
 *
 * This deliberately does **not** use the admin OkHttp stack: that client rewrites
 * every request onto the active install and attaches a session/API key. Subscriber
 * calls are unauthenticated — the token in the URL is the credential — and they go to
 * the portal origin, which may not be an admin connection at all. The client is built
 * per portal host so TOFU pins the right certificate.
 *
 * No logging interceptor is installed: the token lives in the path, and a headers log
 * would print it.
 */
@Singleton
class SubscriberRepository @Inject constructor(
    private val json: Json,
    private val secretStore: SecretStore,
) {
    private data class CachedClient(val host: String, val client: OkHttpClient)

    @Volatile
    private var cached: CachedClient? = null

    private fun clientFor(host: String): OkHttpClient {
        cached?.let { if (it.host == host) return it.client }
        val trust = TofuTrustManager(
            host = host,
            pins = TofuTrustManager.PinSource { h -> secretStore.pinnedFingerprint(h) },
        )
        val ssl = SSLContext.getInstance("TLS").apply {
            init(null, arrayOf<X509TrustManager>(trust), SecureRandom())
        }
        val client = OkHttpClient.Builder()
            .connectTimeout(TIMEOUT_SECONDS, TimeUnit.SECONDS)
            .readTimeout(TIMEOUT_SECONDS, TimeUnit.SECONDS)
            .writeTimeout(TIMEOUT_SECONDS, TimeUnit.SECONDS)
            .sslSocketFactory(ssl.socketFactory, trust)
            .build()
        cached = CachedClient(host, client)
        return client
    }

    /** Remember a confirmed fingerprint for the portal host. */
    fun trust(host: String, fingerprint: String) = secretStore.pinFingerprint(host, fingerprint)

    suspend fun loadStatus(link: SubscriberLink): SubscriberResult<SubscriberStatusDto> =
        withContext(Dispatchers.IO) {
            try {
                val request = Request.Builder().url(link.statusUrl).get().build()
                clientFor(link.host).newCall(request).execute().use { response ->
                    val body = response.body?.string().orEmpty()
                    if (response.isSuccessful) {
                        runCatching { json.decodeFromString<SubscriberStatusDto>(body) }
                            .map { SubscriberResult.Ok(it) }
                            .getOrElse { SubscriberResult.Failed(INVALID_LINK) }
                    } else {
                        SubscriberResult.Failed(httpFailure(response.code, body))
                    }
                }
            } catch (e: IOException) {
                transportFailure(e)
            }
        }

    suspend fun register(
        link: SubscriberLink,
        secret: String,
        ip: String? = null,
    ): SubscriberResult<SubscriberRegisterResponse> = withContext(Dispatchers.IO) {
        try {
            val payload = json.encodeToString(
                SubscriberRegisterRequest(secret = secret, ip = ip?.trim()?.ifBlank { null }),
            )
            val request = Request.Builder()
                .url(link.registerUrl)
                .post(payload.toRequestBody(JSON_MEDIA))
                .build()
            clientFor(link.host).newCall(request).execute().use { response ->
                val body = response.body?.string().orEmpty()
                if (response.isSuccessful) {
                    runCatching { json.decodeFromString<SubscriberRegisterResponse>(body) }
                        .map { SubscriberResult.Ok(it) }
                        .getOrElse { SubscriberResult.Failed("The server's answer could not be read.") }
                } else {
                    SubscriberResult.Failed(httpFailure(response.code, body))
                }
            }
        } catch (e: IOException) {
            transportFailure(e)
        }
    }

    private fun transportFailure(e: IOException): SubscriberResult<Nothing> {
        Timber.w(e, "subscriber request failed")
        val trust = TlsTrustFailure.find(e)
        return when {
            trust != null -> SubscriberResult.TrustRequired(trust.fingerprint, trust.changedFrom)
            isHostnameMismatch(e) -> SubscriberResult.Failed(HOSTNAME_MISMATCH)
            else -> SubscriberResult.Failed(UNREACHABLE)
        }
    }

    /** OkHttp verifies the hostname after TLS; a mismatch arrives wrapped, not as TOFU. */
    private fun isHostnameMismatch(error: Throwable?): Boolean {
        var current = error
        var depth = 0
        while (current != null && depth < CAUSE_DEPTH) {
            if (current is SSLPeerUnverifiedException) return true
            current = current.cause
            depth++
        }
        return false
    }

    /** Map the portal's HTTP failures to the sentence a subscriber can act on. */
    private fun httpFailure(code: Int, body: String): String {
        val reason = runCatching { json.decodeFromString<SubscriberErrorDto>(body).reason }
            .getOrNull()
            .orEmpty()
        return when {
            code == HttpURLConnection.HTTP_UNAUTHORIZED -> "The registration secret is not valid."
            code == HttpURLConnection.HTTP_NOT_FOUND -> INVALID_LINK
            code == HttpURLConnection.HTTP_CONFLICT ->
                "That address is already registered to another subscription."
            code == HttpURLConnection.HTTP_FORBIDDEN && reason == "suspended" ->
                "This subscription is suspended. Contact your provider."
            code == HttpURLConnection.HTTP_FORBIDDEN && reason == "expired" ->
                "This subscription has expired."
            code == HttpURLConnection.HTTP_FORBIDDEN && reason == "quota" ->
                "This subscription's traffic allowance is exhausted."
            else -> "The server answered HTTP $code. Try again in a moment."
        }
    }

    private companion object {
        const val TIMEOUT_SECONDS = 15L
        const val CAUSE_DEPTH = 8
        const val HOSTNAME_MISMATCH =
            "The server's certificate is issued for a different name. Use the hostname " +
                "from the certificate in your subscription link, not its IP address."
        const val UNREACHABLE =
            "Could not reach the server. Check the link and your connection, then try again."
        const val INVALID_LINK = "That subscription link is invalid or has expired."
        val JSON_MEDIA = "application/json; charset=utf-8".toMediaType()
    }
}
