package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.RulesApi
import com.digitrob.digitdns.core.network.RulesUpdateRequest
import com.digitrob.digitdns.domain.model.RuleList
import com.digitrob.digitdns.domain.model.RuleLists
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Reads and writes the custom domain rules.
 *
 * The read returns the whole set; the write is partial by design (see [RulesApi]), so
 * a save of one edited list does not carry the box's other three. [saveOne] is the
 * common case — the screen edits a single list and hands back only that one.
 */
@Singleton
class RulesRepository @Inject constructor(
    private val api: RulesApi,
) {
    suspend fun load(): RuleLists = RuleLists.from(api.get())

    /** Save every list. Used when the screen has all five in hand. */
    suspend fun save(lists: RuleLists): Boolean = api.update(
        RulesUpdateRequest(
            customProxied = lists.proxied,
            customProxiedDisabled = lists.proxiedDisabled,
            customBlocked = lists.blocked,
            customDirect = lists.direct,
            customRecords = lists.records,
        ),
    ).success

    /**
     * Save just one list. The other four are absent from the body and the server
     * leaves them as they were; sending all five would risk clobbering a concurrent
     * edit to a list this screen never touched.
     */
    suspend fun saveList(key: RuleList, domains: List<String>): Boolean {
        val body = when (key) {
            RuleList.Proxied -> RulesUpdateRequest(customProxied = domains)
            RuleList.ProxiedDisabled -> RulesUpdateRequest(customProxiedDisabled = domains)
            RuleList.Blocked -> RulesUpdateRequest(customBlocked = domains)
            RuleList.Direct -> RulesUpdateRequest(customDirect = domains)
        }
        return api.update(body).success
    }

    /** Replace the pinned-record map wholesale, matching the server's replace semantics. */
    suspend fun saveRecords(records: Map<String, String>): Boolean =
        api.update(RulesUpdateRequest(customRecords = records)).success

    /**
     * Add [domain] to one list, read-modify-write.
     *
     * The live log's quick action knows only the domain and the target list, not the
     * list's current contents, so the current contents are read here and the domain
     * appended. The write is still a single-key partial merge, so the other three lists
     * are not touched. Returns the resulting list, or null when the save failed.
     */
    suspend fun addDomain(key: RuleList, domain: String): List<String>? =
        mutateList(key) { current -> if (domain in current) current else current + domain }

    /** Remove [domain] from one list, read-modify-write. Used by the "Remove rule" action. */
    suspend fun removeDomain(key: RuleList, domain: String): List<String>? =
        mutateList(key) { current -> current - domain }

    private suspend fun mutateList(
        key: RuleList,
        transform: (List<String>) -> List<String>,
    ): List<String>? {
        val lists = load()
        val updated = transform(current(key, lists))
        return if (saveList(key, updated)) updated else null
    }

    private fun current(key: RuleList, lists: RuleLists): List<String> = when (key) {
        RuleList.Proxied -> lists.proxied
        RuleList.ProxiedDisabled -> lists.proxiedDisabled
        RuleList.Blocked -> lists.blocked
        RuleList.Direct -> lists.direct
    }
}
