package com.digitrob.digitdns.core.security

import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import javax.net.ssl.X509TrustManager
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The switch-aware layer over per-host TOFU. The OkHttpClient singleton means one
 * trust manager serves every install, so these tests pin that verification always
 * follows the *current* install — the whole bug the class exists to fix — and that
 * the fingerprints, refusals and first-use prompts of the inner managers surface
 * unchanged.
 */
class SwitchingTofuTrustManagerTest {

    private class RecordingManager(
        var verified: Boolean = false,
        val onVerify: () -> Unit = {},
        val issuers: Array<X509Certificate> = arrayOf(),
    ) : X509TrustManager {

        override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {
            verified = true
            onVerify()
        }

        override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) = Unit

        override fun getAcceptedIssuers(): Array<X509Certificate> = issuers
    }

    private val chain = arrayOf<X509Certificate>()

    @Test
    fun verificationFollowsWhicheverInstallIsActiveAtHandshakeTime() {
        val hostA = RecordingManager()
        val hostB = RecordingManager()
        var active: X509TrustManager? = hostA
        val switching = SwitchingTofuTrustManager { active }

        switching.checkServerTrusted(chain, "TLS")
        assertTrue(hostA.verified)

        // The install switches between requests; the next handshake verifies host B
        // with host B's manager, not whatever was active when the app started.
        active = hostB
        switching.checkServerTrusted(chain, "TLS")
        assertTrue(hostB.verified)
    }

    @Test
    fun firstUseAndChangedVerdictsPropagate() {
        val trustOnFirstUse = TrustOnFirstUse(fingerprint = "AA:BB")
        var active: X509TrustManager? = RecordingManager(
            onVerify = { throw trustOnFirstUse },
        )
        val switching = SwitchingTofuTrustManager { active }

        val firstUse = assertThrows(TrustOnFirstUse::class.java) {
            switching.checkServerTrusted(chain, "TLS")
        }
        assertEquals(trustOnFirstUse, firstUse)

        active = RecordingManager(
            onVerify = { throw CertificateChanged(previous = "11:22", current = "AA:BB") },
        )
        val changed = assertThrows(CertificateChanged::class.java) {
            switching.checkServerTrusted(chain, "TLS")
        }
        assertEquals("11:22", changed.previous)
    }

    @Test
    fun noActiveInstallIsAnOrdinaryTlsRefusal() {
        val switching = SwitchingTofuTrustManager { null }

        assertThrows(CertificateException::class.java) {
            switching.checkServerTrusted(chain, "TLS")
        }
    }

    @Test
    fun acceptedIssuersComeFromTheActiveInstaller() {
        var active: X509TrustManager? = RecordingManager()
        val switching = SwitchingTofuTrustManager { active }

        assertEquals(0, switching.acceptedIssuers.size)

        active = null
        assertEquals(0, switching.acceptedIssuers.size)
    }
}
