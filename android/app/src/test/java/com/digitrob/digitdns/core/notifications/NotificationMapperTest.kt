package com.digitrob.digitdns.core.notifications

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * The filter that decides what is worth the operator's attention.
 *
 * The whole stream is every query; notifying on all of it would be a firehose. The
 * mapper's job is to surface only the blocked ones — the events that are a policy
 * decision rather than background noise — and the tests pin that it stays a filter.
 */
class NotificationMapperTest {

    private val mapper = NotificationMapper()

    @Test
    fun aBlockedQueryBecomesAnAlert() {
        val alert = mapper.map(
            QueryEvent(domain = "ads.example", accountName = "Kids", protocol = "DoH", action = "BLOCK"),
        )

        assertNotNull(alert)
        assertEquals("Blocked: ads.example", alert!!.title)
        assertEquals("Kids tried ads.example over DoH", alert.text)
    }

    @Test
    fun aProxiedQueryIsNotAnAlert() {
        assertNull(mapper.map(QueryEvent(domain = "game.example", action = "PROXY")))
    }

    @Test
    fun aDirectQueryIsNotAnAlert() {
        assertNull(mapper.map(QueryEvent(domain = "site.example", action = "DIRECT")))
    }

    @Test
    fun theBlockComparisonIsCaseInsensitive() {
        assertNotNull(mapper.map(QueryEvent(domain = "x.example", action = "block")))
    }

    @Test
    fun theAlertIdIsKeyedOnTheDomainSoRepeatsCollapse() {
        val first = mapper.map(QueryEvent(domain = "ads.example", action = "BLOCK"))!!
        val second = mapper.map(QueryEvent(domain = "ads.example", action = "BLOCK", clientIp = "10.0.0.2"))!!
        val other = mapper.map(QueryEvent(domain = "tracker.example", action = "BLOCK"))!!

        assertEquals(first.id, second.id)
        assertEquals(other.id, "tracker.example".hashCode())
    }

    @Test
    fun aBlockedQueryWithNoAccountFallsBackToTheClientIp() {
        val alert = mapper.map(QueryEvent(domain = "x.example", clientIp = "10.0.0.5", action = "BLOCK"))!!

        assertEquals("10.0.0.5 tried x.example over DNS", alert.text)
    }
}
