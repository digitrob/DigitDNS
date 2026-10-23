package com.digitrob.digitdns.core.security

import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import javax.net.ssl.X509TrustManager

/**
 * An [X509TrustManager] that defers, per handshake, to the manager of whichever
 * install is currently active.
 *
 * **Why this exists.** The OkHttpClient is a DI singleton, so its SSL socket factory —
 * and the trust manager bound into it — is built once for the app's lifetime. The
 * original construction baked the *first* install's host into a [TofuTrustManager],
 * which worked while the app targeted one install and silently corrupted every
 * switch after multi-server arrived: a handshake with the second install was pinned
 * against the first install's fingerprint. Trust stays per-install under this class;
 * what changes is that "per-install" is resolved at verification time instead of at
 * construction time.
 *
 * **Resolution timing.** [active] is consulted inside `checkServerTrusted`, so the
 * host checked is whatever `ServerUrlProvider.current()` reports when the handshake's
 * certificate arrives. A switch already flipped the provider before any request to
 * the new install can leave, which is precisely the property the rebase interceptor
 * relies on too.
 *
 * **The in-flight caveat.** A request that left before a switch still carries the old
 * origin while the manager now verifies the new host. Such a handshake fails with an
 * ordinary TLS error — the same treatment any IOException gets — and the next request
 * after the switch is verified correctly. Correctness of the check always belongs to
 * the host it actually authenticates, which a static manager could never be.
 *
 * [TrustOnFirstUse] and [CertificateChanged] propagate verbatim from the inner
 * manager, so the login dialog's fingerprint capture keeps working unchanged for any
 * host the operator starts.
 */
class SwitchingTofuTrustManager(
    private val active: () -> X509TrustManager?,
) : X509TrustManager {

    override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) {
        // The app never authenticates a client, so this direction is not exercised.
    }

    override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {
        val manager = active()
            ?: throw CertificateException("no active install to verify against")
        manager.checkServerTrusted(chain, authType)
    }

    override fun getAcceptedIssuers(): Array<X509Certificate> =
        active()?.acceptedIssuers ?: emptyArray()
}
