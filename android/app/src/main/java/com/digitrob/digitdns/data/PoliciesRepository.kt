package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.PoliciesApi
import com.digitrob.digitdns.core.network.PolicyDto
import com.digitrob.digitdns.domain.model.Policy
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The raw policy records. Read-mostly: the panel's rule editing happens on the rules
 * screen, and this view exists to show what is stored and to store a named custom
 * entry. POST is a strict upsert keyed by `key`, so [save] writes the whole record.
 */
@Singleton
class PoliciesRepository @Inject constructor(
    private val api: PoliciesApi,
) {
    suspend fun list(): List<Policy> = api.list().items.map(Policy::from)

    suspend fun save(policy: Policy): Policy = Policy.from(
        api.create(
            PolicyDto(
                key = policy.key,
                name = policy.name,
                category = policy.category,
                enabled = policy.enabled,
                customDomains = policy.customDomains,
            ),
        ),
    )
}
