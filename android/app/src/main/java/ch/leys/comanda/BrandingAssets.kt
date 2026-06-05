package ch.leys.comanda

import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.ColorMatrix
import android.graphics.ColorMatrixColorFilter
import android.graphics.Paint
import android.util.Log
import java.io.File
import java.net.HttpURLConnection
import java.net.URL

private const val TAG = "BrandingAssets"

// In-memory cache keyed by URL. Cleared when the process dies; reprints within the same session
// are free. The on-disk cache in cacheDir survives process restarts.
private val memCache = mutableMapOf<String, Bitmap>()

/**
 * Fetch and decode a logo bitmap from [url], caching in memory and on disk.
 *
 * The call is blocking — it is intended to run on a background/print thread.
 * Returns null on any failure (bad URL, network error, decode error, timeout).
 *
 * [maxWidth] and [maxHeight] control the output dimensions; the image is scaled
 * to fit within these bounds while preserving aspect ratio. The output is always
 * ARGB_8888 with a white background (alpha flattened), ready for a thermal renderer.
 */
fun fetchLogoForReceipt(
    url: String,
    cacheDir: File,
    maxWidth: Int = 384,
    maxHeight: Int = 160
): Bitmap? {
    if (url.isBlank()) return null

    // Memory cache hit
    memCache[url]?.let { return it }

    // On-disk cache: use a sanitised filename derived from the URL hash
    val cacheFile = File(cacheDir, "logo_${url.hashCode().toUInt()}.png")
    if (cacheFile.exists()) {
        val cached = BitmapFactory.decodeFile(cacheFile.absolutePath)
        if (cached != null) {
            val ready = prepareForThermal(cached, maxWidth, maxHeight)
            memCache[url] = ready
            return ready
        }
    }

    return try {
        val conn = URL(url).openConnection() as HttpURLConnection
        conn.connectTimeout = 5_000
        conn.readTimeout = 8_000
        conn.instanceFollowRedirects = true
        conn.setRequestProperty("Accept", "image/*")

        if (conn.responseCode != 200) {
            Log.w(TAG, "Logo fetch HTTP ${conn.responseCode} for $url")
            conn.disconnect()
            return null
        }

        val raw = conn.inputStream.use { BitmapFactory.decodeStream(it) }
        conn.disconnect()

        if (raw == null) {
            Log.w(TAG, "Failed to decode logo from $url")
            return null
        }

        val ready = prepareForThermal(raw, maxWidth, maxHeight)

        // Persist to disk cache (best-effort; don't fail the print if this errors)
        try {
            cacheDir.mkdirs()
            cacheFile.outputStream().use { ready.compress(Bitmap.CompressFormat.PNG, 90, it) }
        } catch (t: Throwable) {
            Log.w(TAG, "Logo disk-cache write failed", t)
        }

        memCache[url] = ready
        ready
    } catch (t: Throwable) {
        Log.w(TAG, "Logo fetch failed for $url", t)
        null
    }
}

/**
 * Scale [src] to fit within [maxWidth]x[maxHeight], flatten alpha onto white,
 * and convert to grayscale so thermal dithering works correctly.
 */
private fun prepareForThermal(src: Bitmap, maxWidth: Int, maxHeight: Int): Bitmap {
    // 1. Scale to fit
    val scale = minOf(maxWidth.toFloat() / src.width, maxHeight.toFloat() / src.height, 1f)
    val dstW = (src.width * scale).toInt().coerceAtLeast(1)
    val dstH = (src.height * scale).toInt().coerceAtLeast(1)

    // 2. Composite onto a white ARGB_8888 canvas (handles transparency)
    val flat = Bitmap.createBitmap(dstW, dstH, Bitmap.Config.ARGB_8888)
    val canvas = Canvas(flat)
    canvas.drawColor(Color.WHITE)
    val scaledSrc = Bitmap.createScaledBitmap(src, dstW, dstH, true)
    canvas.drawBitmap(scaledSrc, 0f, 0f, null)
    if (scaledSrc !== src) scaledSrc.recycle()

    // 3. Desaturate to grayscale
    val gray = Bitmap.createBitmap(dstW, dstH, Bitmap.Config.ARGB_8888)
    val grayCanvas = Canvas(gray)
    val grayPaint = Paint().apply {
        colorFilter = ColorMatrixColorFilter(ColorMatrix().also { it.setSaturation(0f) })
    }
    grayCanvas.drawBitmap(flat, 0f, 0f, grayPaint)
    flat.recycle()

    return gray
}
