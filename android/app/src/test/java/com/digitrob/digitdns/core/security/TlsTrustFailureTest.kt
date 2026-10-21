package com.digitrob.digitdns.core.security

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import java.io.IOException
import java.security.cert.CertificateException
import javax.net.ssl.SSLHandshakeException

/**
 * [TlsTrustFailure.find] is what turns a buried TOFU verdict into an operator-facing
 * decision. By the time a handshake failure reaches a call site, OkHttp has wrapped
 * our `CertificateException` inside an `SSLHandshakeException`, so the fingerprint —
 * the only actionable part — is one or more `cause` links down. These pin the walk.
 */
class TlsTrustFailureTest {

    private val fingerprint = "AB:CD:EF:00"

    @Test
    fun findsAFirstUseBuriedUnderTheHandshakeWrapper() {
        val wrapped = SSLHandshakeException("Trust anchor for certification path not found").apply {
            initCause(TrustOnFirstUse(fingerprint))
        }

        val found = TlsTrustFailure.find(wrapped)

        assertEquals(TlsTrustFailure.Prompt(fingerprint, changedFrom = null), found)
    }

    @Test
    fun findsAChangedCertificateAndCarriesThePreviousPin() {
        val wrapped = SSLHandshakeException("cert").apply {
            initCause(CertificateChanged(previous = "OLD:PIN", current = fingerprint))
        }

        val found = TlsTrustFailure.find(wrapped)

        assertEquals(TlsTrustFailure.Prompt(fingerprint, changedFrom = "OLD:PIN"), found)
    }

    @Test
    fun walksADeepChain() {
        val wrapped = SSLHandshakeException("outer").apply {
            initCause(IOException("wrapper").apply { initCause(TrustOnFirstUse(fingerprint)) })
        }

        assertEquals(TlsTrustFailure.Prompt(fingerprint, null), TlsTrustFailure.find(wrapped))
    }

    @Test
    fun aGenericCertificateFailureIsNotAPrompt() {
        // An expired or wrong-host certificate is not something "trust" resolves;
        // offering the prompt for it would train the operator to click through real
        // TLS errors.
        val expired = SSLHandshakeException("expired").apply {
            initCause(CertificateException("certificate expired"))
        }

        assertNull(TlsTrustFailure.find(expired))
    }

    @Test
    fun aPlainTransportFailureIsNotAPrompt() {
        assertNull(TlsTrustFailure.find(IOException("connection reset")))
        assertNull(TlsTrustFailure.find(null))
    }
}
