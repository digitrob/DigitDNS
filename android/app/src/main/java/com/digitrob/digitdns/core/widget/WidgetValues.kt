package com.digitrob.digitdns.core.widget

import android.util.Base64
import com.digitrob.digitdns.R
import com.digitrob.digitdns.core.network.FilteringSettingsDto
import com.digitrob.digitdns.core.network.FilteringSettingsRequest
import com.digitrob.digitdns.data.ThemeMode
import java.util.Locale

internal data class WidgetThemeColors(
    val title: Int,
    val value: Int,
    val muted: Int,
    val toggleOn: Int,
    val toggleOff: Int,
    val filteringOn: Int,
    val filteringOff: Int,
    val toggleUnavailable: Int,
    val open: Int,
    val cardBorder: Int,
    val bgColor: Int,
    val surface: Int,
    val transportPlain: Int,
    val transportDot: Int,
    val transportDoh: Int,
    val transportDoh3: Int,
    val transportDoq: Int,
    val transportDnscrypt: Int,
    val currentWash: Int,
    val fastestText: Int,
    val badgeBg: Int,
    val latencyGood: Int,
    val latencyWarn: Int,
    val latencyBad: Int,
    val dotHealthy: Int,
    val dotWarn: Int,
    val dotBad: Int,
    val dotUnknown: Int,
)

internal object WidgetValues {

    private const val RGB_MASK = 0x00FFFFFF
    private const val ALPHA_SHIFT = 24
    private const val CHANNEL_MAX = 255
    private const val PERCENT_MAX = 100
    private const val BASE64_GROUP = 4
    private const val MAX_HOST_LENGTH = 28
    private const val TRUNCATED_HOST_LENGTH = 26
    private const val HEALTHY_MS = 40L
    private const val WARN_MS = 80L

    fun formatQps(qps: Float): String {
        if (!qps.isFinite()) return "—"
        return String.format(Locale.US, "%.1f", qps)
    }

    fun filteringToggleRequest(settings: FilteringSettingsDto, enabled: Boolean): FilteringSettingsRequest =
        FilteringSettingsRequest(
            enabled = enabled,
            blockMode = settings.blockMode,
            blockTtl = settings.blockTtl,
            updateIntervalHours = settings.updateIntervalHours,
            allowlist = settings.allowlist.orEmpty(),
            customRules = settings.customRules,
        )

    fun resolveWidgetTheme(override: ThemeMode?, appMode: ThemeMode, systemDark: Boolean): ThemeMode =
        when (override ?: appMode) {
            ThemeMode.SYSTEM -> if (systemDark) ThemeMode.DARK else ThemeMode.LIGHT
            ThemeMode.GREY -> ThemeMode.GREY
            ThemeMode.LIGHT -> ThemeMode.LIGHT
            ThemeMode.DARK -> ThemeMode.DARK
        }

    fun resolverWidgetColors(mode: ThemeMode): WidgetThemeColors = when (mode) {
        ThemeMode.SYSTEM, ThemeMode.DARK -> darkWidgetColors()
        ThemeMode.GREY -> greyWidgetColors()
        ThemeMode.LIGHT -> lightWidgetColors()
    }

    private fun darkWidgetColors() = WidgetThemeColors(
        title = 0xFFFFFFFF.toInt(),
        value = 0xFF00F0FF.toInt(),
        muted = 0xFF94A3B8.toInt(),
        toggleOn = 0xFF10B981.toInt(),
        toggleOff = 0xFFFF4655.toInt(),
        filteringOn = 0xFF10B981.toInt(),
        filteringOff = 0xFFFF4655.toInt(),
        toggleUnavailable = 0xFFF59E0B.toInt(),
        open = 0xFF00F0FF.toInt(),
        cardBorder = 0x2938BDF8.toInt(),
        bgColor = 0xFF0D1426.toInt(),
        surface = 0x1AFFFFFF.toInt(),
        transportPlain = 0xFF00F0FF.toInt(),
        transportDot = 0xFF10B981.toInt(),
        transportDoh = 0xFFA855F7.toInt(),
        transportDoh3 = 0xFFFB923C.toInt(),
        transportDoq = 0xFFF59E0B.toInt(),
        transportDnscrypt = 0xFFE879F9.toInt(),
        currentWash = 0x2410B981.toInt(),
        fastestText = 0xFF10B981.toInt(),
        badgeBg = 0x1F10B981.toInt(),
        latencyGood = 0xFF10B981.toInt(),
        latencyWarn = 0xFFF59E0B.toInt(),
        latencyBad = 0xFFFF4655.toInt(),
        dotHealthy = 0xFF10B981.toInt(),
        dotWarn = 0xFFF59E0B.toInt(),
        dotBad = 0xFFFF4655.toInt(),
        dotUnknown = 0xFF94A3B8.toInt(),
    )

    private fun greyWidgetColors() = WidgetThemeColors(
        title = 0xFFF4F4F5.toInt(),
        value = 0xFFA1A1AA.toInt(),
        muted = 0xFF52525B.toInt(),
        toggleOn = 0xFF10B981.toInt(),
        toggleOff = 0xFFFF4655.toInt(),
        filteringOn = 0xFF10B981.toInt(),
        filteringOff = 0xFFFF4655.toInt(),
        toggleUnavailable = 0xFFF59E0B.toInt(),
        open = 0xFFA1A1AA.toInt(),
        cardBorder = 0x2EA1A1AA.toInt(),
        bgColor = 0xFF27272A.toInt(),
        surface = 0x1FFFFFFF.toInt(),
        transportPlain = 0xFFA1A1AA.toInt(),
        transportDot = 0xFFD4D4D8.toInt(),
        transportDoh = 0xFFC4B5FD.toInt(),
        transportDoh3 = 0xFFD4D4D8.toInt(),
        transportDoq = 0xFFF59E0B.toInt(),
        transportDnscrypt = 0xFFC4B5FD.toInt(),
        currentWash = 0x1AFFFFFF.toInt(),
        fastestText = 0xFFD4D4D8.toInt(),
        badgeBg = 0x33FFFFFF.toInt(),
        latencyGood = 0xFF10B981.toInt(),
        latencyWarn = 0xFFF59E0B.toInt(),
        latencyBad = 0xFFFF4655.toInt(),
        dotHealthy = 0xFF10B981.toInt(),
        dotWarn = 0xFFF59E0B.toInt(),
        dotBad = 0xFFFF4655.toInt(),
        dotUnknown = 0xFF94A3B8.toInt(),
    )

    private fun lightWidgetColors() = WidgetThemeColors(
        title = 0xFF111823.toInt(),
        value = 0xFF0E7490.toInt(),
        muted = 0xFF4A5464.toInt(),
        toggleOn = 0xFF065840.toInt(),
        toggleOff = 0xFF8C1C1C.toInt(),
        filteringOn = 0xFF065840.toInt(),
        filteringOff = 0xFF8C1C1C.toInt(),
        toggleUnavailable = 0xFF92400E.toInt(),
        open = 0xFF0E7490.toInt(),
        cardBorder = 0x1A0F172A.toInt(),
        bgColor = 0xFFFFFFFF.toInt(),
        surface = 0x0F0F172A.toInt(),
        transportPlain = 0xFF0E7490.toInt(),
        transportDot = 0xFF065840.toInt(),
        transportDoh = 0xFF7E22CE.toInt(),
        transportDoh3 = 0xFFC2410C.toInt(),
        transportDoq = 0xFF92400E.toInt(),
        transportDnscrypt = 0xFF86198F.toInt(),
        currentWash = 0x14065840.toInt(),
        fastestText = 0xFF065840.toInt(),
        badgeBg = 0x1A065840.toInt(),
        latencyGood = 0xFF065840.toInt(),
        latencyWarn = 0xFF92400E.toInt(),
        latencyBad = 0xFF8C1C1C.toInt(),
        dotHealthy = 0xFF065840.toInt(),
        dotWarn = 0xFF92400E.toInt(),
        dotBad = 0xFF8C1C1C.toInt(),
        dotUnknown = 0xFF4A5464.toInt(),
    )

    fun resolverLatencyText(ms: Double): String =
        if (ms > 0.0) String.format(Locale.US, "%.1f ms", ms) else "—"

    fun bgColorWithAlpha(baseColor: Int, alpha: Int): Int {
        val a = alpha.coerceIn(0, CHANNEL_MAX)
        val rgb = baseColor and RGB_MASK
        return (a shl ALPHA_SHIFT) or rgb
    }

    /** Same as [bgColorWithAlpha] but takes the widget's 0–100 opacity percentage. */
    fun bgColorWithAlphaPercent(baseColor: Int, percent: Int): Int =
        bgColorWithAlpha(baseColor, (percent.coerceIn(0, PERCENT_MAX) * CHANNEL_MAX) / PERCENT_MAX)

    private fun dnscryptStampText(address: String): String = runCatching {
        val encoded = address.trim().removePrefix("sdns://")
        val padding = "=".repeat((BASE64_GROUP - encoded.length % BASE64_GROUP) % BASE64_GROUP)
        String(Base64.decode(encoded + padding, Base64.URL_SAFE or Base64.NO_WRAP), Charsets.ISO_8859_1)
    }.getOrDefault("")

    /**
     * One row of the provider table: the names, icon and routing key a resolver
     * address is recognised by. Keeping the three lookups (label, icon, key) on one
     * table is what stops them drifting apart.
     */
    private data class ProviderMatch(
        val name: String,
        val key: String,
        val icon: Int,
        val needles: List<String>,
    )

    private val providerMatches = listOf(
        ProviderMatch(
            "Cloudflare", "cloudflare", R.drawable.provider_cloudflare,
            listOf("cloudflare", "1.1.1.1", "1.0.0.1", "one.one.one.one"),
        ),
        ProviderMatch(
            "Google", "google", R.drawable.provider_google,
            listOf("google", "8.8.8.8", "8.8.4.4"),
        ),
        ProviderMatch(
            "Quad9", "quad9", R.drawable.provider_quad9,
            listOf("quad9", "9.9.9.9", "149.112.112.112"),
        ),
        ProviderMatch(
            "AdGuard", "adguard", R.drawable.provider_adguard,
            listOf("adguard", "94.140.14.14", "94.140.15.15"),
        ),
        ProviderMatch(
            "Control D", "controld", R.drawable.provider_controld,
            listOf("controld", "freedns", "76.76.2.0", "76.76.10.0"),
        ),
        ProviderMatch(
            "DNS.SB", "dns.sb", R.drawable.provider_dnssb,
            listOf("dns.sb", "dot.sb", "185.222.222.222", "45.11.45.11"),
        ),
        ProviderMatch(
            "OpenDNS", "opendns", R.drawable.provider_opendns,
            listOf("opendns", "208.67.222.222", "208.67.220.220"),
        ),
        ProviderMatch(
            "Yandex", "yandex", R.drawable.provider_yandex,
            listOf("yandex", "77.88.8.8", "77.88.8.1"),
        ),
        ProviderMatch(
            "NextDNS", "nextdns", R.drawable.provider_nextdns,
            listOf("nextdns", "45.90.28.0", "45.90.30.0"),
        ),
        ProviderMatch(
            "CleanBrowsing", "cleanbrowsing", R.drawable.provider_cleanbrowsing,
            listOf("cleanbrowsing", "185.228.168.9", "185.228.169.9"),
        ),
    )

    /** The provider a resolver address names, or null for a generic/unknown host. */
    private fun matchProvider(address: String): ProviderMatch? {
        val raw = address.trim()
        val lower = raw.lowercase(Locale.US)
        // A DNSCrypt stamp is base64 (or an sdns:// URL with a provider name in
        // the decoded payload), so the readable providers live in the decoded text.
        val haystack = if (lower.startsWith("sdns://")) {
            dnscryptStampText(raw).lowercase(Locale.US)
        } else {
            lower
        }
        return providerMatches.firstOrNull { match ->
            haystack.isNotBlank() && match.needles.any { haystack.contains(it) }
        }
    }

    fun resolverDisplayName(address: String): String {
        val raw = address.trim()
        val match = matchProvider(raw)
        val hostPart = raw
            .removePrefix("tls://")
            .removePrefix("https://")
            .removePrefix("http://")
            .removePrefix("quic://")
            .removePrefix("h3://")
            .substringBefore("/")
        return when {
            raw.isBlank() -> "—"
            match != null -> match.name
            raw.lowercase(Locale.US).startsWith("sdns://") -> "DNSCrypt"
            hostPart.length > MAX_HOST_LENGTH -> hostPart.take(TRUNCATED_HOST_LENGTH) + "…"
            else -> hostPart
        }
    }

    fun resolverTransportLabel(address: String): String {
        val lower = address.trim().lowercase(Locale.US)
        return when {
            lower.startsWith("sdns://") -> "DNSCrypt"
            lower.startsWith("tls://") -> "DoT"
            lower.startsWith("https://") || lower.startsWith("h3://") -> "DoH"
            lower.startsWith("quic://") -> "DoQ"
            else -> "Plain"
        }
    }

    fun resolveProtocolKey(address: String): String {
        val lower = address.trim().lowercase(Locale.US)
        return when {
            lower.startsWith("sdns://") -> "dnscrypt"
            lower.startsWith("tls://") -> "dot"
            lower.startsWith("https://") -> "doh"
            lower.startsWith("h3://") -> "doh3"
            lower.startsWith("quic://") -> "doq"
            else -> "plain"
        }
    }

    fun transportAllowedForProtocol(protocolKey: String, allowed: Set<String>?, dnscryptEnabled: Boolean): Boolean {
        val protocol = protocolKey.lowercase(Locale.US)
        return when (protocol) {
            "plain" -> true
            "dnscrypt" -> dnscryptEnabled
            else -> allowed?.contains(protocol) ?: true
        }
    }

    fun formatLatencyLabel(ms: Long?, measured: Boolean): String {
        if (!measured || ms == null || ms <= 0L) return "—"
        return "$ms ms"
    }

    /** Latency bucket for the latency colour, matching the app's thresholds. */
    fun latencyBucket(ms: Long?): String = when {
        ms == null -> "unknown"
        ms <= HEALTHY_MS -> "good"
        ms <= WARN_MS -> "warn"
        else -> "bad"
    }

    /** Latency bucket for the row's status dot, matching the app's thresholds. */
    fun statusKeyFor(measured: Boolean, ms: Long?): String = when {
        !measured -> "unknown"
        ms != null && ms <= HEALTHY_MS -> "healthy"
        ms != null && ms <= WARN_MS -> "warn"
        else -> "bad"
    }

    fun providerIconRes(address: String): Int? = matchProvider(address)?.icon

    fun providerKeyFromAddress(address: String): String? = matchProvider(address)?.key
}
