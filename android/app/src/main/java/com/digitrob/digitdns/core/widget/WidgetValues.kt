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
        ThemeMode.SYSTEM, ThemeMode.DARK -> WidgetThemeColors(
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
        ThemeMode.GREY -> WidgetThemeColors(
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
        ThemeMode.LIGHT -> WidgetThemeColors(
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
    }

    fun resolverLatencyText(ms: Double): String = if (ms > 0.0) String.format(Locale.US, "%.1f ms", ms) else "—"

    fun bgColorWithAlpha(baseColor: Int, alpha: Int): Int {
        val a = alpha.coerceIn(0, 255)
        val rgb = baseColor and 0x00FFFFFF
        return (a shl 24) or rgb
    }

    /** Same as [bgColorWithAlpha] but takes the widget's 0–100 opacity percentage. */
    fun bgColorWithAlphaPercent(baseColor: Int, percent: Int): Int =
        bgColorWithAlpha(baseColor, (percent.coerceIn(0, 100) * 255) / 100)

    private fun dnscryptStampText(address: String): String = runCatching {
        val encoded = address.trim().removePrefix("sdns://")
        val padding = "=".repeat((4 - encoded.length % 4) % 4)
        String(Base64.decode(encoded + padding, Base64.URL_SAFE or Base64.NO_WRAP), Charsets.ISO_8859_1)
    }.getOrDefault("")

    fun resolverDisplayName(address: String): String {
        val raw = address.trim()
        if (raw.isBlank()) return "—"
        val a = raw.lowercase()
        return when {
            a.contains("cloudflare") || a.contains("1.1.1.1") || a.contains("1.0.0.1") || a.contains("one.one.one.one") -> "Cloudflare"
            a.contains("google") || a.contains("8.8.8.8") || a.contains("8.8.4.4") -> "Google"
            a.contains("quad9") || a.contains("9.9.9.9") || a.contains("149.112.112.112") -> "Quad9"
            a.contains("adguard") || a.contains("94.140.14.14") || a.contains("94.140.15.15") -> "AdGuard"
            a.contains("controld") || a.contains("freedns") || a.contains("76.76.2.0") || a.contains("76.76.10.0") -> "Control D"
            a.contains("dns.sb") || a.contains("dot.sb") || a.contains("185.222.222.222") || a.contains("45.11.45.11") -> "DNS.SB"
            a.contains("opendns") || a.contains("208.67.222.222") || a.contains("208.67.220.220") -> "OpenDNS"
            a.contains("yandex") || a.contains("77.88.8.8") || a.contains("77.88.8.1") -> "Yandex"
            a.contains("nextdns") -> "NextDNS"
            a.contains("cleanbrowsing") -> "CleanBrowsing"
            a.startsWith("sdns://") -> {
                val stamp = dnscryptStampText(raw).lowercase()
                when {
                    stamp.contains("quad9") -> "Quad9"
                    stamp.contains("opendns") -> "OpenDNS"
                    stamp.contains("adguard") -> "AdGuard"
                    else -> "DNSCrypt"
                }
            }
            else -> {
                val hostPart = raw.removePrefix("tls://").removePrefix("https://").removePrefix("http://")
                    .removePrefix("quic://").removePrefix("h3://").substringBefore("/")
                if (hostPart.length > 28) hostPart.take(26) + "…" else hostPart
            }
        }
    }

    fun resolverTransportLabel(address: String): String {
        val a = address.trim().lowercase()
        return when {
            a.startsWith("sdns://") -> "DNSCrypt"
            a.startsWith("tls://") -> "DoT"
            a.startsWith("https://") || a.startsWith("h3://") -> "DoH"
            a.startsWith("quic://") -> "DoQ"
            else -> "Plain"
        }
    }

    fun resolveProtocolKey(address: String): String {
        val a = address.trim().lowercase()
        return when {
            a.startsWith("sdns://") -> "dnscrypt"
            a.startsWith("tls://") -> "dot"
            a.startsWith("https://") -> "doh"
            a.startsWith("h3://") -> "doh3"
            a.startsWith("quic://") -> "doq"
            else -> "plain"
        }
    }

    fun transportAllowedForProtocol(protocolKey: String, allowed: Set<String>?, dnscryptEnabled: Boolean): Boolean {
        val p = protocolKey.lowercase(Locale.US)
        if (p == "plain") return true
        if (p == "dnscrypt") return dnscryptEnabled
        val a = allowed ?: return true
        return a.contains(p)
    }

    fun formatLatencyLabel(ms: Long?, measured: Boolean): String {
        if (!measured || ms == null || ms <= 0L) return "—"
        return "${ms} ms"
    }

    fun providerIconRes(address: String): Int? {
        val raw = address.trim()
        if (raw.isBlank()) return null
        val a = raw.lowercase()
        val haystack = if (a.startsWith("sdns://")) dnscryptStampText(raw).lowercase() else a
        return when {
            haystack.contains("cloudflare") || haystack.contains("1.1.1.1") || haystack.contains("1.0.0.1") || haystack.contains("one.one.one.one") -> R.drawable.provider_cloudflare
            haystack.contains("google") || haystack.contains("8.8.8.8") || haystack.contains("8.8.4.4") -> R.drawable.provider_google
            haystack.contains("quad9") || haystack.contains("9.9.9.9") || haystack.contains("149.112.112.112") -> R.drawable.provider_quad9
            haystack.contains("adguard") || haystack.contains("94.140.14.14") || haystack.contains("94.140.15.15") -> R.drawable.provider_adguard
            haystack.contains("controld") || haystack.contains("freedns") || haystack.contains("76.76.2.0") || haystack.contains("76.76.10.0") -> R.drawable.provider_controld
            haystack.contains("dns.sb") || haystack.contains("dot.sb") || haystack.contains("185.222.222.222") || haystack.contains("45.11.45.11") -> R.drawable.provider_dnssb
            haystack.contains("opendns") || haystack.contains("208.67.222.222") || haystack.contains("208.67.220.220") -> R.drawable.provider_opendns
            haystack.contains("yandex") || haystack.contains("77.88.8.8") || haystack.contains("77.88.8.1") -> R.drawable.provider_yandex
            haystack.contains("nextdns") || haystack.contains("45.90.28.0") || haystack.contains("45.90.30.0") -> R.drawable.provider_nextdns
            haystack.contains("cleanbrowsing") || haystack.contains("185.228.168.9") || haystack.contains("185.228.169.9") -> R.drawable.provider_cleanbrowsing
            else -> null
        }
    }

    fun providerKeyFromAddress(address: String): String? {
        val raw = address.trim()
        if (raw.isBlank()) return null
        val a = raw.lowercase()
        val haystack = if (a.startsWith("sdns://")) dnscryptStampText(raw).lowercase() else a
        return when {
            haystack.contains("cloudflare") || haystack.contains("1.1.1.1") || haystack.contains("1.0.0.1") || haystack.contains("one.one.one.one") -> "cloudflare"
            haystack.contains("google") || haystack.contains("8.8.8.8") || haystack.contains("8.8.4.4") -> "google"
            haystack.contains("quad9") || haystack.contains("9.9.9.9") || haystack.contains("149.112.112.112") -> "quad9"
            haystack.contains("adguard") || haystack.contains("94.140.14.14") || haystack.contains("94.140.15.15") -> "adguard"
            haystack.contains("controld") || haystack.contains("freedns") || haystack.contains("76.76.2.0") || haystack.contains("76.76.10.0") -> "controld"
            haystack.contains("dns.sb") || haystack.contains("dot.sb") || haystack.contains("185.222.222.222") || haystack.contains("45.11.45.11") -> "dns.sb"
            haystack.contains("opendns") || haystack.contains("208.67.222.222") || haystack.contains("208.67.220.220") -> "opendns"
            haystack.contains("yandex") || haystack.contains("77.88.8.8") || haystack.contains("77.88.8.1") -> "yandex"
            haystack.contains("nextdns") || haystack.contains("45.90.28.0") || haystack.contains("45.90.30.0") -> "nextdns"
            haystack.contains("cleanbrowsing") || haystack.contains("185.228.168.9") || haystack.contains("185.228.169.9") -> "cleanbrowsing"
            else -> null
        }
    }

    fun grayscaleVariantOf(resId: Int): Int? = null
}
