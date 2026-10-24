package com.digitrob.digitdns.ui.components

import android.content.Context
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import coil.ImageLoader
import coil.compose.AsyncImage
import coil.decode.SvgDecoder
import java.util.Locale

/**
 * The flat 4:3 flag for an ISO 3166-1 alpha-2 country, drawn from the assets that
 * ship with the app.
 *
 * The flags are SVGs from flag-icons, deliberately not emoji: emoji flags are
 * drawn by the device's emoji font and every country ends up a different shape
 * (and some builds fall back to two letters). A bundled rectangle looks the same
 * on every phone, offline. An unknown or empty code renders nothing, which is
 * the right outcome for a server whose country the database has not resolved.
 */
@Composable
fun CountryFlag(
    country: String,
    modifier: Modifier = Modifier,
    width: Dp = 22.dp,
    height: Dp = 16.dp,
) {
    val code = country.trim().lowercase(Locale.ROOT)
    if (!FLAG_CODE.matches(code)) return

    val context = LocalContext.current
    val loader = remember(context) { FlagImageLoader.get(context) }
    AsyncImage(
        model = "file:///android_asset/flags/$code.svg",
        contentDescription = Locale("", code.uppercase(Locale.ROOT))
            .getDisplayCountry(Locale.getDefault()),
        imageLoader = loader,
        contentScale = ContentScale.Crop,
        modifier = modifier
            .size(width = width, height = height)
            .clip(RoundedCornerShape(2.dp)),
    )
}

private val FLAG_CODE = Regex("^[a-z]{2}$")

/**
 * One ImageLoader for every flag. Coil's default loader does not decode SVG
 * without the `coil-svg` component registered, and building a loader per row
 * would give each flag its own caches and thread pool.
 */
private object FlagImageLoader {
    @Volatile
    private var instance: ImageLoader? = null

    fun get(context: Context): ImageLoader =
        instance ?: synchronized(this) {
            instance ?: ImageLoader.Builder(context.applicationContext)
                .components { add(SvgDecoder.Factory()) }
                .build()
                .also { instance = it }
        }
}
