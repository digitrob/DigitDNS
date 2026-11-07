package com.digitrob.digitdns.core.network

import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import okhttp3.OkHttpClient
import java.util.concurrent.TimeUnit
import javax.inject.Singleton

/**
 * The live-log client. It is the singleton client with only the read timeout
 * removed: an idle queue between events is normal for SSE, while every ordinary
 * request keeps a finite timeout and can fail instead of spinning forever.
 *
 * Kept in its own module so [NetworkModule] stays under the function-count
 * threshold it is held to.
 */
@Module
@InstallIn(SingletonComponent::class)
object StreamClientModule {
    @Provides
    @Singleton
    @StreamClient
    fun provideStreamClient(base: OkHttpClient): OkHttpClient =
        base.newBuilder()
            .readTimeout(0, TimeUnit.MILLISECONDS)
            .build()
}
