package com.digitrob.digitdns.core.security

import java.security.KeyStore
import java.security.MessageDigest
import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import javax.net.ssl.TrustManagerFactory
import javax.net.ssl.X509TrustManager

/**
 * Trust-on-first-use for the install's self-signed certificate.
 *
 * A fresh DigitDNS install serves over a certificate it minted itself, so there is
 * no public CA to validate against and the two obvious shortcuts are both wrong:
 * trusting every certificate defeats TLS, and pinning the certificate at build time
 * cannot work because each install has its own. What is left is the shape SSH has
 * used for decades — show the operator the fingerprint on first contact, remember it
 * once confirmed, and shout if it ever changes.
 *
 * **Host keying.** `X509TrustManager` is not told which host is being verified, and
 * the app targets exactly one install, so this class is scoped to *that* host: the
 * [host] is fixed when the manager is built from the stored connection, and its pin
 * is the only one consulted. That is a deliberate simplification — a single-install
 * companion does not need a per-host pin map, and introducing one would invite the
 * bug where the wrong pin is matched to the wrong origin.
 *
 * **Order of checks.** The platform's own validation runs first. If it succeeds (a
 * real ACME certificate after the operator configures a domain), the chain is
 * accepted with no pin, because demanding a pin for a CA-issued certificate would
 * break every renewal. Only when the platform refuses — the self-signed case — does
 * TOFU apply.
 */
class TofuTrustManager(
    private val host: String,
    private val pins: PinSource,
) : X509TrustManager {

    /** Where pins come from, so this class is testable without Android storage. */
    fun interface PinSource {
        fun pinnedFingerprint(host: String): String?
    }

    override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) {
        // The app never authenticates a client, so this direction is not exercised.
    }

    override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {
        val leaf = chain?.firstOrNull()
            ?: throw CertificateException("empty certificate chain")

        val fingerprint = sha256(leaf)

        // A CA-validated chain needs no pin and must survive certificate renewal.
        if (platformTrusts(chain, authType)) return

        when (val verdict = decide(pins.pinnedFingerprint(host), fingerprint)) {
            is TrustVerdict.Trusted -> return
            is TrustVerdict.FirstUse -> throw TrustOnFirstUse(verdict.fingerprint)
            is TrustVerdict.Changed -> throw CertificateChanged(verdict.previous, verdict.fingerprint)
        }
    }

    override fun getAcceptedIssuers(): Array<X509Certificate> = emptyArray()

    private fun platformTrusts(chain: Array<out X509Certificate>, authType: String?): Boolean =
        runCatching { defaultTrustManager.checkServerTrusted(chain, authType ?: "RSA") }.isSuccess

    private val defaultTrustManager: X509TrustManager by lazy {
        val factory = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm())
        factory.init(null as KeyStore?)
        factory.trustManagers.filterIsInstance<X509TrustManager>().first()
    }

    companion object {
        /** A SHA-256 fingerprint rendered the way `openssl` prints it: colon-separated hex. */
        fun sha256(cert: X509Certificate): String {
            val digest = MessageDigest.getInstance("SHA-256").digest(cert.encoded)
            return digest.joinToString(":") { "%02X".format(it) }
        }

        /**
         * The pure decision behind `checkServerTrusted`: given the pin on record (or
         * null) and the fingerprint just seen, which way does trust go? Extracted so it
         * can be tested without standing up a real TLS handshake and a real certificate.
         */
        fun decide(pinned: String?, fingerprint: String): TrustVerdict = when {
            pinned == null -> TrustVerdict.FirstUse(fingerprint)
            pinned.equals(fingerprint, ignoreCase = true) -> TrustVerdict.Trusted
            else -> TrustVerdict.Changed(previous = pinned, fingerprint = fingerprint)
        }
    }
}

/** The three outcomes of a TOFU check. */
sealed interface TrustVerdict {
    /** Matches the pinned fingerprint, or the chain was CA-validated. */
    data object Trusted : TrustVerdict

    /** Never seen before; the operator must confirm [fingerprint]. */
    data class FirstUse(val fingerprint: String) : TrustVerdict

    /** Seen before but the certificate changed; the operator must re-confirm. */
    data class Changed(val previous: String, val fingerprint: String) : TrustVerdict
}

/** First contact with an unpinned host: the operator must confirm [fingerprint]. */
class TrustOnFirstUse(val fingerprint: String) :
    CertificateException(
        "First connection to this install; certificate $fingerprint is not yet trusted",
    )

/** The pinned certificate changed; [previous] and [current] are both shown. */
class CertificateChanged(val previous: String, val current: String) :
    CertificateException("The install's certificate changed; refusal is the safe default")
