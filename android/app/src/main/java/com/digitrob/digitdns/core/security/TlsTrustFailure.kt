package com.digitrob.digitdns.core.security

/**
 * A TLS handshake failure that is *ours* to resolve, found anywhere in a throwable's
 * cause chain.
 *
 * When [TofuTrustManager] refuses a certificate it throws [TrustOnFirstUse] or
 * [CertificateChanged], but by the time the exception reaches a call site OkHttp has
 * wrapped it: a `javax.net.ssl.SSLHandshakeException` ("Trust anchor for certification
 * path not found") sits on top, with our `CertificateException` buried underneath.
 * The operator-visible decision is in the buried one — "trust this fingerprint?" —
 * and the wrapper only says "TLS failed", so the chain is walked to recover it.
 *
 * Only the two TOFU verdicts are recognised. A generic certificate failure (an expired
 * cert, a genuinely wrong host) is deliberately *not* matched: it is not something the
 * operator can resolve by pressing "trust", and offering that would train them to
 * click through real TLS errors.
 */
sealed interface TlsTrustFailure {
    /** The fingerprint seen. [changedFrom] is set when a previously pinned cert changed. */
    data class Prompt(val fingerprint: String, val changedFrom: String?) : TlsTrustFailure

    companion object {
        /**
         * Walk [throwable]'s cause chain for a TOFU verdict, or return null when this
         * is not a trust decision the operator can make.
         */
        fun find(throwable: Throwable?): Prompt? {
            var current = throwable
            var depth = 0
            var verdict: Prompt? = null
            while (verdict == null && current != null && depth < MAX_CAUSE_DEPTH) {
                verdict = when (current) {
                    is TrustOnFirstUse -> Prompt(current.fingerprint, changedFrom = null)
                    is CertificateChanged -> Prompt(current.current, current.previous)
                    else -> null
                }
                current = current.cause
                depth++
            }
            return verdict
        }

        /**
         * A handshake wraps its cause, and OkHttp may add an aggregate on top of that;
         * a handful of frames covers the real chains without looping forever on a
         * self-referential cause.
         */
        private const val MAX_CAUSE_DEPTH = 8
    }
}
