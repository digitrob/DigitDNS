import java.util.Properties

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
    alias(libs.plugins.ksp)
    alias(libs.plugins.hilt)
    alias(libs.plugins.detekt)
}

// The app carries its own version, independent of the server's v3.2.x line: a
// panel release and an app release are different products on different cadences,
// and tying them would make every server patch look like an app release.
val appVersionName: String = "3.4.5"
val appVersionCode: Int = 345

android {
    namespace = "com.digitrob.digitdns"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.digitrob.digitdns"
        minSdk = 29
        targetSdk = 36
        versionCode = appVersionCode
        versionName = appVersionName

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
        vectorDrawables { useSupportLibrary = true }

        // The id of the foreground service that holds the live SSE stream. Declared
        // as a buildConfig field so the notification channel and the manifest stay in
        // step from one source.
        buildConfigField("String", "SSE_SERVICE_CLASS", "\"com.digitrob.digitdns.core.notifications.LiveStreamService\"")
    }

    // Release signing is driven entirely by CI secrets. Nothing here hardcodes a
    // keystore; a local `assembleRelease` without them produces an unsigned APK,
    // which is the honest result for a machine that holds no signing identity.
    val keystorePropsFile = rootProject.file("keystore.properties")
    val keystoreProps = Properties().apply {
        if (keystorePropsFile.exists()) keystorePropsFile.inputStream().use { load(it) }
    }
    val storeFilePath = keystoreProps.getProperty("storeFile")
        ?: System.getenv("ANDROID_KEYSTORE_PATH")
    val hasReleaseSigning = !storeFilePath.isNullOrBlank()

    signingConfigs {
        if (hasReleaseSigning) {
            create("release") {
                storeFile = file(storeFilePath!!)
                storePassword = keystoreProps.getProperty("storePassword")
                    ?: System.getenv("ANDROID_KEYSTORE_PASSWORD")
                keyAlias = keystoreProps.getProperty("keyAlias")
                    ?: System.getenv("ANDROID_KEY_ALIAS")
                keyPassword = keystoreProps.getProperty("keyPassword")
                    ?: System.getenv("ANDROID_KEY_PASSWORD")
            }
        }
    }

    buildTypes {
        debug {
            applicationIdSuffix = ".debug"
            versionNameSuffix = "-debug"
            isMinifyEnabled = false
        }
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
            if (hasReleaseSigning) {
                signingConfig = signingConfigs.getByName("release")
            }
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    packaging {
        resources {
            excludes += "/META-INF/{AL2.0,LGPL2.1}"
        }
    }

    // The release ships per-architecture APKs so an operator downloads only the slice
    // their device runs — a universal APK carries every ABI's native libraries, and
    // this app pulls in enough native code (Coil, DataStore, ZXing) that the dead
    // weight is real. The universal APK is still emitted (`isUniversalApk`) as the
    // "I don't know / it's a weird device" fallback, and it is the one local debug
    // installs resolve to.
    splits {
        abi {
            isEnable = true
            reset()
            include("arm64-v8a", "armeabi-v7a", "x86_64")
            isUniversalApk = true
        }
    }

    testOptions {
        unitTests.isReturnDefaultValues = true
    }
}

// detekt reads the module's production Kotlin sources. Test sources are deliberately
// excluded: a test asserts against literal numbers ("1024" is the whole point of the
// formatting test) and the magic-number/long-method rules that keep production code
// honest would only be noise there. The tests are checked by the test runner and by
// review, not by a style rule.
detekt {
    buildUponDefaultConfig = true
    allRules = false
    config.setFrom(rootProject.files("config/detekt/detekt.yml"))
    source.setFrom(files("src/main/java"))
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.navigation.compose)

    // Compose. The BOM pins every androidx.compose.* artifact to one tested set.
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.graphics)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.compose.material3)
    implementation(libs.compose.material.icons.extended)
    debugImplementation(libs.compose.ui.tooling)
    debugImplementation(libs.compose.ui.test.manifest)

    // Hilt. The compiler runs through KSP, not kapt — the Hilt 2.53 line is fully
    // KSP-ready and kapt is slower on a plugin-heavy module like this one.
    implementation(libs.hilt.android)
    ksp(libs.hilt.compiler)
    implementation(libs.hilt.navigation.compose)

    // Networking. kotlinx.serialization is the single converter: the v3 API is JSON
    // and the models are @Serializable data classes, so there is no second JSON stack.
    implementation(libs.retrofit)
    implementation(libs.retrofit.kotlinx.serialization)
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.okhttp)
    implementation(libs.okhttp.logging)
    implementation(libs.okhttp.sse)
    implementation(libs.kotlinx.coroutines.android)

    // Persistence and secrets.
    implementation(libs.androidx.datastore.preferences)
    implementation(libs.androidx.security.crypto)

    // UI extras the panel has an equivalent of: QR generation for the connect tab,
    // charts for the dashboard, and Coil for the icon set.
    implementation(libs.vico.compose.m3)
    implementation(libs.accompanist.permissions)
    implementation(libs.zxing.core)
    implementation(libs.coil.compose)

    implementation(libs.timber)

    testImplementation(libs.junit)
    testImplementation(libs.turbine)
    testImplementation(libs.mockk)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.okhttp.mockwebserver)

    androidTestImplementation(libs.androidx.junit)
    androidTestImplementation(libs.androidx.espresso.core)
    androidTestImplementation(platform(libs.compose.bom))
    androidTestImplementation(libs.compose.ui.test.junit4)
}
