package com.digitrob.digitdns.core.network

import com.digitrob.digitdns.data.TwoFactorRepository
import com.jakewharton.retrofit2.converter.kotlinx.serialization.asConverterFactory
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import retrofit2.Retrofit

/**
 * TOTP two-factor. The secret and `otpauth://` URI exist only in the setup response —
 * status never carries them — so the tests pin that setup returns them and that
 * enable/disable send the identify-and-prove triplet.
 */
class TwoFactorApiTest {

    private lateinit var server: MockWebServer
    private lateinit var repo: TwoFactorRepository

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        val json = Json { ignoreUnknownKeys = true; explicitNulls = false }
        val client = OkHttpClient.Builder().addInterceptor(ApiExceptionInterceptor(Reachability())).build()
        val api = Retrofit.Builder()
            .baseUrl(server.url("/"))
            .client(client)
            .addConverterFactory(json.asConverterFactory("application/json".toMediaType()))
            .build()
            .create(TwoFactorApi::class.java)
        repo = TwoFactorRepository(api)
    }

    @After
    fun tearDown() = server.shutdown()

    @Test
    fun statusReadsTheThreeFlags() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody("""{"totp_enabled":true,"totp_configured":true,"totp_enrolling":false}"""),
        )

        val status = repo.status()

        assertTrue(status.totpEnabled)
        assertTrue(status.totpConfigured)
    }

    @Test
    fun setupReturnsSecretAndUriAndPostsCredentials() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(200).setBody(
                """{"success":true,"secret":"JBSWY3DPEHPK3PXP","otpauth_uri":"otpauth://totp/digitdns?secret=JBSWY3DPEHPK3PXP"}""",
            ),
        )

        val enrolment = repo.startEnrolment("admin", "pw")

        assertEquals("JBSWY3DPEHPK3PXP", enrolment.secret)
        assertTrue(enrolment.otpauthUri.startsWith("otpauth://"))
        val recorded = server.takeRequest()
        assertEquals("/auth/2fa/setup", recorded.path)
        assertEquals("""{"username":"admin","current_password":"pw"}""", recorded.body.readUtf8())
    }

    @Test
    fun enableSendsTheCodeAndReportsSuccess() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"success":true}"""))

        val ok = repo.enable("admin", "pw", "123456")

        assertTrue(ok)
        val recorded = server.takeRequest()
        assertEquals("/auth/2fa/enable", recorded.path)
        assertEquals("""{"username":"admin","current_password":"pw","code":"123456"}""", recorded.body.readUtf8())
    }

    @Test
    fun disableSendsTheCode() = runTest {
        server.enqueue(MockResponse().setResponseCode(200).setBody("""{"success":true}"""))

        repo.disable("admin", "pw", "654321")

        val recorded = server.takeRequest()
        assertEquals("/auth/2fa/disable", recorded.path)
        assertEquals("""{"username":"admin","current_password":"pw","code":"654321"}""", recorded.body.readUtf8())
    }
}
