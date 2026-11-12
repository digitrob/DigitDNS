package com.digitrob.digitdns.data

import com.digitrob.digitdns.core.network.AlertTestResponse
import com.digitrob.digitdns.core.network.AlertsEnvelope
import com.digitrob.digitdns.core.network.AlertsApi
import com.digitrob.digitdns.core.network.AlertsDto
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class ServerAlertsRepository @Inject constructor(private val api: AlertsApi) {
    suspend fun load(): AlertsDto = api.get().alerts
    suspend fun loadEnvelope(): AlertsEnvelope = api.get()
    suspend fun save(body: AlertsDto): AlertsDto = api.save(body).alerts
    suspend fun test(): AlertTestResponse = api.test()
}
