package com.digitrob.digitdns.core.network

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

/**
 * RFC 9457 problem document, the shape every v3 error uses.
 *
 * The server varies the media type by route — data routes answer
 * `application/problem+json` through `writeProblemAt`, while a refused address gets
 * `httpx.WriteJSONError` — but the body is the same five members plus, on a login
 * that needs a second factor, an extra `twofactor_required` flag that the spec calls
 * out by name. Both are modelled, and [Problem] keeps the extra flag so the login
 * flow can branch on it rather than pattern-matching a string.
 */
@Serializable
data class Problem(
    val type: String = "",
    val title: String = "",
    val status: Int = 0,
    val detail: String = "",
    val instance: String = "",
    @SerialName("twofactor_required") val twoFactorRequired: Boolean = false,
) {
    /** The message to show a person, preferring `detail` and never returning blank. */
    val message: String
        get() = detail.ifBlank { title }.ifBlank { "Request failed (HTTP $status)" }

    companion object {
        private val json = Json { ignoreUnknownKeys = true; explicitNulls = false }

        /** Parse a problem body, or null if [body] is not one. */
        fun parseOrNull(body: String?): Problem? {
            if (body.isNullOrBlank()) return null
            return runCatching { json.decodeFromString<Problem>(body) }.getOrNull()
        }
    }
}
