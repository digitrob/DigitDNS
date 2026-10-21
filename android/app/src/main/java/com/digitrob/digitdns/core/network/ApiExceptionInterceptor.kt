package com.digitrob.digitdns.core.network

import okhttp3.Interceptor
import okhttp3.Response
import okhttp3.ResponseBody
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Turns a non-2xx response into an [ApiException] carrying the problem document.
 *
 * Without this, every call site would catch `retrofit2.HttpException`, re-read the
 * error body off the response, and parse it — the same twenty lines in twenty
 * places. Throwing an [java.io.IOException] subclass from an interceptor is the
 * documented way to fail an OkHttp call, so Retrofit's suspend functions surface
 * [ApiException] directly and callers get a typed failure with the server's own
 * `detail` string already parsed.
 *
 * The response body is read here (it can only be read once), so on the error path the
 * body is consumed and closed; on the success path the response is returned
 * untouched for Retrofit's converter to read.
 */
@Singleton
class ApiExceptionInterceptor @Inject constructor(
    private val reachability: Reachability,
) : Interceptor {

    override fun intercept(chain: Interceptor.Chain): Response {
        val response = chain.proceed(chain.request())
        if (response.isSuccessful) {
            // Any 2xx proves the install is up; let the connection pill say Online at
            // once rather than waiting for the SSE handshake to complete.
            reachability.markReachable()
            return response
        }

        val raw = response.body?.let { body: ResponseBody ->
            runCatching { body.string() }.getOrNull()
        }
        response.close()

        throw ApiException.from(
            status = response.code,
            problem = Problem.parseOrNull(raw),
            rawBody = raw,
        )
    }
}
