package com.digitrob.digitdns.domain.model

/**
 * The operations `POST /clients/{id}/actions/{action}` accepts, each paired with the
 * wire name the server switches on and the payload it reads.
 *
 * The wire names are lowercase and hyphenated (`reset-traffic`), which is not
 * something a Kotlin enum name can be, so [wireName] carries the mapping. Keeping the
 * payload on the variant means the repository never has to translate a loose bundle
 * of nullable fields into "which action was this".
 */
sealed interface ClientAction {
    /** Add [extendDays] days to the expiry. Server rejects a non-positive value. */
    data class Renew(val extendDays: Int) : ClientAction

    /** Enable or disable the account. */
    data class Toggle(val enabled: Boolean) : ClientAction

    /** Add an IP or CIDR to the allowlist. */
    data class AddIp(val ip: String) : ClientAction

    /** Remove an IP or CIDR from the allowlist. */
    data class RemoveIp(val ip: String) : ClientAction

    /** Zero the traffic counter. */
    data object ResetTraffic : ClientAction

    /** Issue a new UUID (invalidates the old one). */
    data object RegenerateUuid : ClientAction

    /** Issue a new registration secret (invalidates the old one). */
    data object RegenerateRegisterSecret : ClientAction

    /** The path segment the server switches on. */
    val wireName: String
        get() = when (this) {
            is Renew -> "renew"
            is Toggle -> "toggle"
            is AddIp -> "add_ip"
            is RemoveIp -> "remove_ip"
            ResetTraffic -> "reset-traffic"
            RegenerateUuid -> "regenerate-uuid"
            RegenerateRegisterSecret -> "regenerate-register-secret"
        }
}
