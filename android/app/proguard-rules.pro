# kotlinx.serialization generates a serializer per @Serializable class and reflects
# on it by name; without these keep rules R8 strips the generated serializers and the
# app fails at runtime only on a release build.
-keepattributes *Annotation*, InnerClasses
-dontnote kotlinx.serialization.**

-keepclassmembers class ** {
    @kotlinx.serialization.Serializable <fields>;
}
-keepclasseswithmembers class ** {
    kotlinx.serialization.KSerializer serializer(...);
}

# Retrofit reads the @GET/@POST annotations and the method signatures reflectively.
-keepattributes Signature, RuntimeVisibleAnnotations, AnnotationDefault
-keep,allowobfuscation,allowshrinking interface retrofit2.Call
-keep,allowobfuscation,allowshrinking class retrofit2.Response
-keep,allowobfuscation,allowshrinking class kotlin.coroutines.Continuation

# OkHttp/Okio carry platform-specific paths R8 would otherwise warn about.
-dontwarn okhttp3.internal.platform.**
-dontwarn org.conscrypt.**
-dontwarn org.bouncycastle.**
-dontwarn org.openjsse.**

# Tink (pulled in by androidx.security.crypto, which backs SecretStore) references
# Error Prone's annotations. They are compile-time only and never ship, so R8 cannot
# resolve them and the release build fails outright without these suppressions — the
# debug build is unaffected, which is exactly why this only ever surfaces here.
-dontwarn com.google.errorprone.annotations.**
