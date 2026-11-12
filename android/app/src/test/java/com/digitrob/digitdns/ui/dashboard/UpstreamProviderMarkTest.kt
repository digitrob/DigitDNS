package com.digitrob.digitdns.ui.dashboard

import com.digitrob.digitdns.R
import org.junit.Assert.assertEquals
import org.junit.Test

/** The active resolver list and Single Server picker use these bundled marks. */
class UpstreamProviderMarkTest {
    @Test
    fun controlDFreednsUsesTheControlDMark() {
        assertEquals(R.drawable.provider_controld, providerDrawable("tls://p0.freedns.controld.com"))
        assertEquals(R.drawable.provider_controld, providerDrawable("https://freedns.controld.com/p0"))
        assertEquals(R.drawable.provider_controld, providerDrawable("76.76.2.0"))
    }

    @Test
    fun theRestOfTheCatalogueMapsToItsOwnMark() {
        assertEquals(R.drawable.provider_dnssb, providerDrawable("tls://dot.sb"))
        assertEquals(R.drawable.provider_opendns, providerDrawable("https://doh.opendns.com/dns-query"))
        assertEquals(R.drawable.provider_yandex, providerDrawable("77.88.8.8"))
        assertEquals(R.drawable.provider_nextdns, providerDrawable("45.90.28.0"))
        assertEquals(R.drawable.provider_cleanbrowsing, providerDrawable("185.228.168.9"))
    }
}
