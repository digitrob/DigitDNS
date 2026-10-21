package com.digitrob.digitdns.core.security

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The three-way decision at the heart of trust-on-first-use. Getting it wrong in the
 * permissive direction (treating a change as trusted) is the failure that matters —
 * it is the exact case a MITM creates — so the "changed" path is asserted to be a
 * refusal, not a silent re-pin.
 */
class TofuTrustManagerTest {

    private val fingerprint = "AA:BB:CC:DD"

    @Test
    fun anUnpinnedHostIsAFirstUse() {
        val verdict = TofuTrustManager.decide(pinned = null, fingerprint = fingerprint)
        assertEquals(TrustVerdict.FirstUse(fingerprint), verdict)
    }

    @Test
    fun aMatchingPinIsTrusted() {
        assertTrue(TofuTrustManager.decide(pinned = fingerprint, fingerprint = fingerprint) is TrustVerdict.Trusted)
    }

    @Test
    fun matchingIsCaseInsensitive() {
        assertTrue(
            TofuTrustManager.decide(pinned = "aa:bb:cc:dd", fingerprint = "AA:BB:CC:DD") is TrustVerdict.Trusted,
        )
    }

    @Test
    fun aChangedCertificateIsRefusedLoudly() {
        val verdict = TofuTrustManager.decide(pinned = "11:22", fingerprint = fingerprint)
        assertEquals(TrustVerdict.Changed(previous = "11:22", fingerprint = fingerprint), verdict)
    }
}
