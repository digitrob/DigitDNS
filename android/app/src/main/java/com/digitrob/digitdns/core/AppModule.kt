package com.digitrob.digitdns.core

import android.content.Context
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

/**
 * The one binding the rest of the graph needs from the platform: an application
 * [Context]. It is provided at the singleton scope and as `@ApplicationContext`, so
 * the stores that persist to disk ([com.digitrob.digitdns.data.ConnectionStore],
 * [com.digitrob.digitdns.core.security.SecretStore]) never hold an Activity context
 * and cannot leak one.
 */
@Module
@InstallIn(SingletonComponent::class)
object AppModule {

    @Provides
    @Singleton
    fun provideApplicationContext(@ApplicationContext context: Context): Context = context
}
