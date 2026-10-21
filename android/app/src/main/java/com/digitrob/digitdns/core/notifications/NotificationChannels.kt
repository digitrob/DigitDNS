package com.digitrob.digitdns.core.notifications

import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import androidx.core.content.getSystemService
import com.digitrob.digitdns.R

/**
 * The notification channels the app uses.
 *
 * Channels are created before any notification is posted, because posting to a
 * channel that does not exist is a silent no-op on API 26+ — the notification simply
 * never appears and nothing reports why. Two channels, split by importance:
 *  - [ALERTS] — the blocked-query notices the operator opted into (default importance,
 *    which makes a sound).
 *  - [STATUS] — the foreground service's own persistent notification (low, silent),
 *    required for the service to run and deliberately unobtrusive.
 */
object NotificationChannels {

    const val ALERTS = "digitdns_alerts"
    const val STATUS = "digitdns_status"

    /** Idempotent: safe to call on every service start. */
    fun ensure(context: Context) {
        val manager = context.getSystemService<NotificationManager>() ?: return

        val alerts = NotificationChannel(
            ALERTS,
            context.getString(R.string.channel_live_stream_name),
            NotificationManager.IMPORTANCE_DEFAULT,
        ).apply { description = context.getString(R.string.channel_live_stream_desc) }

        // IMPORTANCE_LOW: the persistent "connected" notification is a status line,
        // not an event — it must not buzz on every reconnect.
        val status = NotificationChannel(
            STATUS,
            context.getString(R.string.channel_live_status_name),
            NotificationManager.IMPORTANCE_LOW,
        ).apply { description = context.getString(R.string.channel_live_status_desc) }

        manager.createNotificationChannels(listOf(alerts, status))
    }
}
