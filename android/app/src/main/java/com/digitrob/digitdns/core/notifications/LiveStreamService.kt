package com.digitrob.digitdns.core.notifications

import android.annotation.SuppressLint
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import com.digitrob.digitdns.MainActivity
import com.digitrob.digitdns.R
import com.digitrob.digitdns.data.ConnectionStore
import com.digitrob.digitdns.data.stream.LiveQueryStream
import dagger.hilt.android.AndroidEntryPoint
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.firstOrNull
import kotlinx.coroutines.launch
import timber.log.Timber
import javax.inject.Inject

/**
 * Keeps the live query log streaming while the app is backgrounded and raises a local
 * notification for the events worth surfacing (blocked queries).
 *
 * This is the "local notifications over SSE" transport chosen for v1: the server has
 * no FCM today, so there is nothing to register a device token with, and a foreground
 * service is the honest way for an Android app to stay connected without one — a
 * background socket is killed by Doze within minutes. A later phase can add FCM on
 * top; the notification surface here is transport-agnostic.
 *
 * It does **not** own a socket: [LiveQueryStream] is the single connection, shared
 * with the in-app Live Query Log, and this service is one of its consumers. It holds a
 * stream reference for as long as it runs (that reference is what keeps the socket
 * open when the UI is gone) and registers a listener that filters events into
 * notifications.
 */
@AndroidEntryPoint
class LiveStreamService : Service() {

    @Inject lateinit var liveStream: LiveQueryStream
    @Inject lateinit var mapper: NotificationMapper
    @Inject lateinit var connectionStore: ConnectionStore

    private var removeListener: (() -> Unit)? = null

    /**
     * The service outlives any single Activity but does not outlive itself, so its
     * own flow collectors get a scope with [onCreate]'s lifetime.
     */
    private val serviceScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)

    /**
     * The active install's display name, supplied to the alert mapper only when more
     * than one server is saved — with a single install, a tag has nothing to
     * disambiguate and every notification would read the same for no reason. A
     * volatile so the listener callback (which may run on any thread) always reads the
     * latest tag instead of a pre-switch one.
     */
    @Volatile
    private var serverTag: String? = null

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onCreate() {
        super.onCreate()
        NotificationChannels.ensure(this)
        startForegroundCompat()
        // Holding a stream reference is what keeps the socket alive without the UI;
        // releasing it on destroy lets the last remaining consumer (if any) keep it.
        liveStream.start()
        removeListener = liveStream.addQueryListener { event -> notify(mapper.map(event, serverTag)) }
        observeServerIdentity()
    }

    /**
     * Follow the saved-server list and the active install: a second install appearing
     * is what turns server tagging on (and names the ongoing notification), a first
     * install turning it back off, and a switch or rename rewrites both immediately —
     * before the next blocked query can carry a stale tag.
     *
     * The collector must not run on the main looper's dispatch queue beyond the state
     * update: it only touches [notifyPersistent], which posts a notification.
     */
    private fun observeServerIdentity() {
        serviceScope.launch {
            connectionStore.connection.collect { connection ->
                val count = connectionStore.servers.firstOrNull().orEmpty().size
                serverTag = if (count > 1) connection?.displayName else null
                notifyPersistent()
            }
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP) {
            stopSelf()
            return START_NOT_STICKY
        }
        // START_STICKY: if the OS kills the process under memory pressure, the stream
        // is worth re-establishing without the operator reopening the app.
        return START_STICKY
    }

    override fun onDestroy() {
        removeListener?.invoke()
        removeListener = null
        serviceScope.cancel()
        liveStream.stop()
        super.onDestroy()
    }

    // POST_NOTIFICATIONS is checked at the opt-in (the Alerts screen), and a denied
    // permission makes `notify` a silent no-op rather than a crash — but lint cannot
    // see across the service boundary, so the check it wants is asserted here instead.
    @SuppressLint("MissingPermission")
    private fun notify(alert: NotificationMapper.Alert?) {
        if (alert == null) return
        val notification = NotificationCompat.Builder(this, NotificationChannels.ALERTS)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(alert.title)
            .setContentText(alert.text)
            .setStyle(NotificationCompat.BigTextStyle().bigText(alert.text))
            .setPriority(NotificationCompat.PRIORITY_DEFAULT)
            .setContentIntent(openAppIntent())
            .setAutoCancel(true)
            .build()
        // POST_NOTIFICATIONS may be denied; posting is then a no-op, which is the
        // correct behaviour — the operator declined this channel.
        runCatching { NotificationManagerCompat.from(this).notify(alert.id, notification) }
            .onFailure { Timber.w(it, "could not post alert") }
    }

    private fun openAppIntent(): PendingIntent {
        val intent = Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
        }
        return PendingIntent.getActivity(
            this,
            0,
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
    }

    /**
     * The service's own persistent notification, required for a foreground service.
     * Its detail line names the install it is connected to once tagging is on, so a
     * multi-install operator can tell from the shade which server they are watching
     * without opening the app.
     */
    private fun startForegroundCompat() {
        val detail = serverTag?.let { getString(R.string.live_alerts_running_detail_named, it) }
            ?: getString(R.string.live_alerts_running_detail)
        val notification = NotificationCompat.Builder(this, NotificationChannels.STATUS)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(getString(R.string.live_alerts_running))
            .setContentText(detail)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .setOngoing(true)
            .setContentIntent(openAppIntent())
            .build()

        val type = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC
        } else {
            0
        }
        startForeground(FOREGROUND_NOTIFICATION_ID, notification, type)
    }

    /** Refresh the persistent notification in place, same id and channel. */
    @SuppressLint("MissingPermission")
    private fun notifyPersistent() {
        val detail = serverTag?.let { getString(R.string.live_alerts_running_detail_named, it) }
            ?: getString(R.string.live_alerts_running_detail)
        val notification = NotificationCompat.Builder(this, NotificationChannels.STATUS)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(getString(R.string.live_alerts_running))
            .setContentText(detail)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .setOngoing(true)
            .setContentIntent(openAppIntent())
            .build()
        runCatching {
            NotificationManagerCompat.from(this).notify(FOREGROUND_NOTIFICATION_ID, notification)
        }.onFailure { Timber.w(it, "could not refresh the persistent notification") }
    }

    companion object {
        const val ACTION_STOP = "com.digitrob.digitdns.action.STOP_LIVE_STREAM"

        private const val FOREGROUND_NOTIFICATION_ID = 1001

        /** Start the stream service. Safe to call when it is already running. */
        fun start(context: Context) {
            val intent = Intent(context, LiveStreamService::class.java)
            context.startForegroundService(intent)
        }

        /** Stop the stream service. */
        fun stop(context: Context) {
            val intent = Intent(context, LiveStreamService::class.java).apply {
                action = ACTION_STOP
            }
            context.startService(intent)
        }
    }
}
