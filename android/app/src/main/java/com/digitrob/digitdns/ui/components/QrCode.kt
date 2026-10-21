package com.digitrob.digitdns.ui.components

import android.graphics.Bitmap
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.google.zxing.BarcodeFormat
import com.google.zxing.EncodeHintType
import com.google.zxing.MultiFormatWriter
import com.google.zxing.common.BitMatrix

/**
 * Renders [content] as a QR code, using zxing to build the matrix and Compose to
 * draw it.
 *
 * The QR is drawn in black on white regardless of theme — a dark-mode QR (light
 * modules on dark) is frequently unscannable by phone cameras, and this code exists
 * to be scanned by exactly such a camera. The surrounding card may be dark; the code
 * itself is not.
 *
 * Generation is remembered on [content] so a recomposition (every poll tick on a
 * neighbouring screen) does not rebuild the matrix.
 */
@Composable
fun QrCode(
    content: String,
    modifier: Modifier = Modifier,
    size: Dp = 220.dp,
) {
    val bitmap = remember(content) { generateQrBitmap(content) }
    if (bitmap == null) return

    Image(
        bitmap = bitmap.asImageBitmap(),
        contentDescription = "QR code",
        modifier = modifier
            .size(size)
            .clip(RoundedCornerShape(8.dp))
            .background(androidx.compose.ui.graphics.Color.White)
            .padding(8.dp),
    )
}

/**
 * Build the QR bitmap. Returns null on content zxing cannot encode (an over-long or
 * empty string), so the caller can simply not draw anything rather than crash.
 */
private fun generateQrBitmap(content: String, dimension: Int = 512): Bitmap? {
    if (content.isBlank()) return null
    return runCatching {
        val hints = mapOf(EncodeHintType.MARGIN to 1)
        val matrix: BitMatrix = MultiFormatWriter().encode(
            content,
            BarcodeFormat.QR_CODE,
            dimension,
            dimension,
            hints,
        )
        val pixels = IntArray(dimension * dimension)
        for (y in 0 until dimension) {
            val row = y * dimension
            for (x in 0 until dimension) {
                pixels[row + x] = if (matrix.get(x, y)) BLACK else WHITE
            }
        }
        Bitmap.createBitmap(dimension, dimension, Bitmap.Config.ARGB_8888).apply {
            setPixels(pixels, 0, dimension, 0, 0, dimension, dimension)
        }
    }.getOrNull()
}

private const val BLACK = 0xFF000000.toInt()
private const val WHITE = 0xFFFFFFFF.toInt()
