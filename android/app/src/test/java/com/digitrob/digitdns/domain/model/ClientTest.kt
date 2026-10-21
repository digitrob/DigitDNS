package com.digitrob.digitdns.domain.model

import com.digitrob.digitdns.core.network.ClientDto
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/**
 * The derivations a client row depends on: is it active, how full is the quota, when
 * does it lapse. These are the rules that decide what colour and label the UI shows,
 * so they are pinned rather than left to be re-derived in a composable.
 */
class ClientTest {

    private fun dto(
        limitGb: Double = 0.0,
        usedBytes: Long = 0,
        enabled: Boolean = true,
        quotaExceeded: Boolean = false,
        expiresAt: String = "",
    ) = ClientDto(
        id = "c1",
        name = "Test",
        trafficLimitGb = limitGb,
        trafficUsedBytes = usedBytes,
        enabled = enabled,
        quotaExceeded = quotaExceeded,
        expiresAt = expiresAt,
    )

    @Test
    fun unlimitedQuotaReportsNoFractionAndNoCap() {
        val client = Client.from(dto(limitGb = 0.0, usedBytes = 1_000_000))
        assertFalse(client.hasQuota)
        assertEquals(0f, client.quotaFraction, 0.001f)
    }

    @Test
    fun quotaFractionIsUsedOverLimit() {
        // 5 GB of a 10 GB cap is half full.
        val client = Client.from(dto(limitGb = 10.0, usedBytes = (5.0 * Client.BYTES_PER_GB).toLong()))
        assertEquals(0.5f, client.quotaFraction, 0.01f)
    }

    @Test
    fun quotaFractionIsClampedWhenOverLimit() {
        val client = Client.from(dto(limitGb = 1.0, usedBytes = (3.0 * Client.BYTES_PER_GB).toLong()))
        assertEquals(1f, client.quotaFraction, 0.001f)
    }

    @Test
    fun anEmptyExpiryMeansNoExpiryNotTheEpoch() {
        val client = Client.from(dto(expiresAt = ""))
        assertNull(client.expiresAt)
        assertNull(client.daysUntilExpiry)
        assertFalse(client.isExpired)
    }

    @Test
    fun aPastExpiryIsExpired() {
        val client = Client.from(dto(expiresAt = Instant.now().minusSeconds(3600).toString()))
        assertTrue(client.isExpired)
    }

    @Test
    fun activityRequiresEnabledAndNotExceededAndNotExpired() {
        assertTrue(Client.from(dto()).isActive)
        assertFalse(Client.from(dto(enabled = false)).isActive)
        assertFalse(Client.from(dto(quotaExceeded = true)).isActive)
        assertFalse(Client.from(dto(expiresAt = Instant.now().minusSeconds(60).toString())).isActive)
    }

    @Test
    fun dayDeltaCountsWholeDays() {
        val inSixDays = Client.from(dto(expiresAt = Instant.now().plusSeconds(6 * 86_400).toString()))
        assertEquals(6L, inSixDays.daysUntilExpiry)
    }
}
