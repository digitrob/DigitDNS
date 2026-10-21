package com.digitrob.digitdns.core.network

import com.digitrob.digitdns.BuildConfig
import com.digitrob.digitdns.core.security.SecretStore
import com.digitrob.digitdns.core.security.TofuTrustManager
import com.jakewharton.retrofit2.converter.kotlinx.serialization.asConverterFactory
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.logging.HttpLoggingInterceptor
import retrofit2.Retrofit
import java.security.SecureRandom
import java.util.concurrent.TimeUnit
import javax.inject.Singleton
import javax.net.ssl.SSLContext
import javax.net.ssl.X509TrustManager

/**
 * The HTTP stack for the v3 API.
 *
 * Three details are deliberate and match the server:
 *  - The base URL is a placeholder; [AuthInterceptor] rewrites each request onto the
 *    stored connection, because the host, port and random admin path are only known
 *    after setup. See [ServerUrlProvider].
 *  - TLS uses [TofuTrustManager] so a self-signed install can be reached after the
 *    operator confirms its fingerprint, while a CA-issued certificate validates
 *    normally.
 *  - kotlinx.serialization is the only converter; the app has one JSON stack.
 */
/** Seconds to wait for a TCP/TLS handshake. */
private const val CONNECT_TIMEOUT_SECONDS = 15L

/** Seconds to wait to finish sending a request body. */
private const val WRITE_TIMEOUT_SECONDS = 15L

/** Zero disables the read timeout, which the idle-between-events SSE stream requires. */
private const val STREAM_READ_TIMEOUT_MILLIS = 0L

@Module
@InstallIn(SingletonComponent::class)
object NetworkModule {

    @Provides
    @Singleton
    fun provideJson(): Json = Json {
        ignoreUnknownKeys = true
        explicitNulls = false
        coerceInputValues = true
    }

    @Provides
    @Singleton
    fun provideOkHttp(
        authRetryInterceptor: AuthRetryInterceptor,
        authInterceptor: AuthInterceptor,
        apiExceptionInterceptor: ApiExceptionInterceptor,
        secretStore: SecretStore,
        urlProvider: ServerUrlProvider,
    ): OkHttpClient {
        val logging = HttpLoggingInterceptor().apply {
            // HEADERS, not BODY: the bodies carry tokens and client credentials, and a
            // debug log is exactly the place they should not surface.
            level = if (BuildConfig.DEBUG) {
                HttpLoggingInterceptor.Level.HEADERS
            } else {
                HttpLoggingInterceptor.Level.NONE
            }
        }

        val builder = OkHttpClient.Builder()
            // Outermost: it catches the 401 ApiException raised below, refreshes the
            // session, and retries once — before AuthInterceptor re-attaches the token.
            .addInterceptor(authRetryInterceptor)
            .addInterceptor(authInterceptor)
            // The exception mapper sits inside the loggers so the status and headers of
            // a failed call are still logged before the IOException is raised.
            .addInterceptor(apiExceptionInterceptor)
            .addInterceptor(logging)
            // The live stream holds a long connection that is idle between events, so
            // the read timeout must be disabled or SSE would be torn down mid-wait.
            .connectTimeout(CONNECT_TIMEOUT_SECONDS, TimeUnit.SECONDS)
            .readTimeout(STREAM_READ_TIMEOUT_MILLIS, TimeUnit.MILLISECONDS)
            .writeTimeout(WRITE_TIMEOUT_SECONDS, TimeUnit.SECONDS)

        // TOFU only applies to the current install's host; the trust manager is built
        // lazily so a re-configure (new host) swaps it on the next client build.
        urlProvider.current()?.let { conn ->
            val trustManager = TofuTrustManager(
                host = conn.host,
                pins = TofuTrustManager.PinSource { host -> secretStore.pinnedFingerprint(host) },
            )
            val sslContext = SSLContext.getInstance("TLS").apply {
                init(null, arrayOf<X509TrustManager>(trustManager), SecureRandom())
            }
            builder.sslSocketFactory(sslContext.socketFactory, trustManager)
        }

        return builder.build()
    }

    @Provides
    @Singleton
    fun provideRetrofit(client: OkHttpClient, json: Json): Retrofit = Retrofit.Builder()
        // A placeholder base; AuthInterceptor rewrites every request onto the stored
        // connection, so this host is never the one actually contacted.
        .baseUrl("https://127.0.0.1/")
        .client(client)
        .addConverterFactory(json.asConverterFactory("application/json".toMediaType()))
        .build()

    @Provides
    @Singleton
    fun provideDigitDnsApi(retrofit: Retrofit): DigitDnsApi = retrofit.create(DigitDnsApi::class.java)

    @Provides
    @Singleton
    fun provideAuthApi(retrofit: Retrofit): AuthApi = retrofit.create(AuthApi::class.java)

    @Provides
    @Singleton
    fun provideClientsApi(retrofit: Retrofit): ClientsApi = retrofit.create(ClientsApi::class.java)

    @Provides
    @Singleton
    fun provideStatsApi(retrofit: Retrofit): StatsApi = retrofit.create(StatsApi::class.java)

    @Provides
    @Singleton
    fun provideConfigApi(retrofit: Retrofit): ConfigApi = retrofit.create(ConfigApi::class.java)

    @Provides
    @Singleton
    fun provideRulesApi(retrofit: Retrofit): RulesApi = retrofit.create(RulesApi::class.java)

    @Provides
    @Singleton
    fun provideGroupsApi(retrofit: Retrofit): GroupsApi = retrofit.create(GroupsApi::class.java)

    @Provides
    @Singleton
    fun providePoliciesApi(retrofit: Retrofit): PoliciesApi = retrofit.create(PoliciesApi::class.java)

    @Provides
    @Singleton
    fun provideAccessApi(retrofit: Retrofit): AccessApi = retrofit.create(AccessApi::class.java)

    @Provides
    @Singleton
    fun provideUpstreamsApi(retrofit: Retrofit): UpstreamsApi = retrofit.create(UpstreamsApi::class.java)

    @Provides
    @Singleton
    fun provideSettingsApi(retrofit: Retrofit): SettingsApi = retrofit.create(SettingsApi::class.java)

    @Provides
    @Singleton
    fun provideSubscriptionApi(retrofit: Retrofit): SubscriptionApi =
        retrofit.create(SubscriptionApi::class.java)

    @Provides
    @Singleton
    fun provideTwoFactorApi(retrofit: Retrofit): TwoFactorApi = retrofit.create(TwoFactorApi::class.java)

    @Provides
    @Singleton
    fun provideTlsApi(retrofit: Retrofit): TlsApi = retrofit.create(TlsApi::class.java)

    @Provides
    @Singleton
    fun provideDiagnosticsApi(retrofit: Retrofit): DiagnosticsApi =
        retrofit.create(DiagnosticsApi::class.java)

    @Provides
    @Singleton
    fun provideSystemApi(retrofit: Retrofit): SystemApi = retrofit.create(SystemApi::class.java)
}
