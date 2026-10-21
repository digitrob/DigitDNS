package com.digitrob.digitdns.domain.model

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * `Connection.baseUrl` is the single place the install's address is assembled, and it
 * has to tolerate how an operator actually pastes a value — the deployed install's
 * admin path is `/ad90ecbb08bfa890/`, and a person copying it from the address bar
 * will bring the slashes with them. Every one of those must resolve to the same URL,
 * or the app is unusable for the one install it is built for.
 */
class ConnectionTest {

    @Test
    fun buildsHttpsUrlWithAdminPath() {
        val conn = Connection(host = "77.90.63.9", port = 37586, adminPath = "ad90ecbb08bfa890")
        assertEquals("https://77.90.63.9:37586/ad90ecbb08bfa890/", conn.baseUrl)
    }

    @Test
    fun trimsSurroundingSlashesFromTheAdminPath() {
        val bare = Connection("host", 443, "abc").baseUrl
        val wrapped = Connection("host", 443, "/abc/").baseUrl
        val doubled = Connection("host", 443, "//abc//").baseUrl
        assertEquals(bare, wrapped)
        assertEquals(bare, doubled)
        assertEquals("https://host:443/abc/", bare)
    }

    @Test
    fun emptyAdminPathStillYieldsARootedBase() {
        assertEquals("https://host:443/", Connection("host", 443, "").baseUrl)
        assertEquals("https://host:443/", Connection("host", 443, "/").baseUrl)
    }

    @Test
    fun cleartextIsHonouredWhenTheOperatorTurnsHttpsOff() {
        assertEquals("http://10.0.0.5:80/", Connection("10.0.0.5", 80, "", isHttps = false).baseUrl)
    }

    @Test
    fun displayNameFallsBackToAuthority() {
        assertEquals("host:443", Connection("host", 443, "").displayName)
        assertEquals("My server", Connection("host", 443, "", label = "My server").displayName)
    }

    @Test
    fun apiV3BaseUrlCarriesTheVersionSegmentUnderTheAdminPath() {
        // The single source of truth for where the server mounts v3: after the hidden
        // admin path, not at the install root. A missing `api/v3` is a silent 404.
        val conn = Connection("77.90.63.9", 37586, "ad90ecbb08bfa890")
        assertEquals(
            "https://77.90.63.9:37586/ad90ecbb08bfa890/api/v3/",
            conn.apiV3BaseUrl,
        )
    }

    @Test
    fun apiV3BaseUrlWorksWithAnEmptyAdminPath() {
        assertEquals("https://host:443/api/v3/", Connection("host", 443, "").apiV3BaseUrl)
    }

    @Test
    fun streamUrlAppendsTheTicketToTheApiPath() {
        val conn = Connection("77.90.63.9", 37586, "ad90ecbb08bfa890")

        assertEquals(
            "https://77.90.63.9:37586/ad90ecbb08bfa890/api/v3/events/stream?ticket=abc123",
            conn.streamUrl("abc123"),
        )
    }

    @Test
    fun streamUrlPercentEncodesTheTicket() {
        // A ticket is opaque server-minted text; `+`, `/` and `=` are all possible
        // and all change meaning unescaped in a query string.
        val url = Connection("host", 443, "admin").streamUrl("a+b/c=")

        assertEquals("https://host:443/admin/api/v3/events/stream?ticket=a%2Bb%2Fc%3D", url)
    }
}
