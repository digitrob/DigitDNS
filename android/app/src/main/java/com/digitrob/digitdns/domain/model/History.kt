package com.digitrob.digitdns.domain.model

/**
 * The four windows the server's aggregate store exposes. The wire values are the
 * `range` query parameter, not an enum ordinal or a display label.
 */
enum class HistoryRange(val wire: String) {
    LAST_24H("24h"),
    LAST_7D("7d"),
    LAST_30D("30d"),
    LAST_90D("90d"),
    ;

    companion object {
        fun fromWire(value: String?): HistoryRange =
            entries.firstOrNull { it.wire == value } ?: LAST_24H
    }
}

/** The ranked tables the history endpoint can return. */
enum class TopDimension(val wire: String) {
    DOMAIN("domain"),
    BLOCKED_DOMAIN("blocked_domain"),
    CLIENT("client"),
    UPSTREAM("upstream"),
    PROTOCOL("protocol"),
    RELAY_COUNTRY("relay_country"),
}
