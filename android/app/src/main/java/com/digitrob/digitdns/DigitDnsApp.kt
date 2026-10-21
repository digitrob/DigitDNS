package com.digitrob.digitdns

import android.app.Application
import dagger.hilt.android.HiltAndroidApp
import timber.log.Timber

/**
 * The application entry point. Timber is planted here (debug-only tree) so every
 * later component — the API client, the SSE service, the repositories — can log
 * through one facade, and Hilt builds the graph from this class.
 */
@HiltAndroidApp
class DigitDnsApp : Application() {
    override fun onCreate() {
        super.onCreate()
        if (BuildConfig.DEBUG) {
            Timber.plant(Timber.DebugTree())
        }
    }
}
