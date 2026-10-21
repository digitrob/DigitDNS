package com.digitrob.digitdns.core.network

import kotlinx.serialization.json.Json
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * The v3 JSON is decoded with `ignoreUnknownKeys = true` because the server is free to
 * add members to a response without that being a breaking change for this client. This
 * test pins that: a payload carrying a field the DTO does not declare must still
 * decode, and the fields it does declare must survive.
 */
class VersionDtoTest {

    private val json = Json { ignoreUnknownKeys = true; explicitNulls = false }

    @Test
    fun decodesTheLiveVersionShapeAndIgnoresUnknownKeys() {
        // Shaped after the real GET /api/v3/version body, plus one field this DTO does
        // not declare.
        val payload = """
            {"version":"3.2.2","display":"v3.2.2-stable","hash":"5ac54360",
             "commit":"ad98558","api":"v3","something_added_later":true}
        """.trimIndent()

        val dto = json.decodeFromString<VersionDto>(payload)

        assertEquals("3.2.2", dto.version)
        assertEquals("v3.2.2-stable", dto.display)
        assertEquals("5ac54360", dto.hash)
        assertEquals("v3", dto.api)
    }

    @Test
    fun missingOptionalFieldsFallBackToDefaults() {
        val dto = json.decodeFromString<VersionDto>("""{"version":"3.2.2"}""")
        assertEquals("3.2.2", dto.version)
        assertEquals("", dto.hash)
    }
}
