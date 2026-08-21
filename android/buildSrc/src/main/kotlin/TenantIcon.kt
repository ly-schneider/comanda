package tenantbranding

import org.gradle.api.DefaultTask
import org.gradle.api.file.DirectoryProperty
import org.gradle.api.file.RegularFileProperty
import org.gradle.api.provider.Property
import org.gradle.api.tasks.CacheableTask
import org.gradle.api.tasks.Input
import org.gradle.api.tasks.InputFile
import org.gradle.api.tasks.Optional
import org.gradle.api.tasks.OutputDirectory
import org.gradle.api.tasks.PathSensitive
import org.gradle.api.tasks.PathSensitivity
import org.gradle.api.tasks.TaskAction
import java.awt.Color
import java.awt.Font
import java.awt.RenderingHints
import java.awt.geom.Ellipse2D
import java.awt.image.BufferedImage
import java.io.File
import javax.imageio.ImageIO

/**
 * Generates a tenant's launcher icon from their logo, or from their initials
 * when no usable logo is supplied.
 *
 * Resources are named `ic_launcher_tenant*` so they never collide with the stock
 * icons in `main/res`; the manifest picks between them via the `appIcon` placeholder.
 */
@CacheableTask
abstract class GenerateTenantIconTask : DefaultTask() {

    /** Absent falls back to [initials]. */
    @get:InputFile
    @get:Optional
    @get:PathSensitive(PathSensitivity.NONE)
    abstract val sourceImage: RegularFileProperty

    @get:Input
    abstract val initials: Property<String>

    /** `#RRGGBB` or `#AARRGGBB`. */
    @get:Input
    abstract val backgroundColor: Property<String>

    @get:Input
    abstract val generateIcon: Property<Boolean>

    @get:OutputDirectory
    abstract val outputDir: DirectoryProperty

    @TaskAction
    fun generate() {
        val out = outputDir.get().asFile
        out.deleteRecursively()
        out.mkdirs()
        if (!generateIcon.get()) return

        // CI runners have no display.
        System.setProperty("java.awt.headless", "true")

        val background = parseColor(backgroundColor.get())
        val foreground = contrastingColor(background)
        val logo = sourceImage.orNull?.asFile?.takeIf { it.isFile }?.let { readImage(it) }

        ADAPTIVE_DENSITIES.forEach { (density, size) ->
            val canvas = blankCanvas(size)
            canvas.createGraphics().use { g ->
                // Fitting the art's diagonal inside the safe circle survives
                // any mask shape the launcher applies.
                val safeDiameter = size * SAFE_ZONE_FRACTION
                drawContent(g, logo, initials.get(), foreground, size, safeDiameter)
            }
            write(canvas, File(out, "mipmap-$density/ic_launcher_tenant_foreground.png"))
        }

        LEGACY_DENSITIES.forEach { (density, size) ->
            val icon = legacyIcon(size, logo, initials.get(), background, foreground)
            write(icon, File(out, "mipmap-$density/ic_launcher_tenant.png"))
            write(icon, File(out, "mipmap-$density/ic_launcher_tenant_round.png"))
        }

        File(out, "values").mkdirs()
        File(out, "values/tenant_icon_colors.xml").writeText(
            """
            <?xml version="1.0" encoding="utf-8"?>
            <resources>
                <color name="ic_launcher_tenant_background">${toHex(background)}</color>
            </resources>
            """.trimIndent() + "\n"
        )

        File(out, "mipmap-anydpi-v26").mkdirs()
        listOf("ic_launcher_tenant", "ic_launcher_tenant_round").forEach { name ->
            File(out, "mipmap-anydpi-v26/$name.xml").writeText(
                """
                <?xml version="1.0" encoding="utf-8"?>
                <adaptive-icon xmlns:android="http://schemas.android.com/apk/res/android">
                    <background android:drawable="@color/ic_launcher_tenant_background" />
                    <foreground android:drawable="@mipmap/ic_launcher_tenant_foreground" />
                </adaptive-icon>
                """.trimIndent() + "\n"
            )
        }
    }

    private fun legacyIcon(
        size: Int,
        logo: BufferedImage?,
        initials: String,
        background: Color,
        foreground: Color,
    ): BufferedImage {
        val canvas = blankCanvas(size)
        canvas.createGraphics().use { g ->
            // Pre-26 launchers show the bitmap unmasked, so draw our own circle.
            g.color = background
            g.fill(Ellipse2D.Float(0f, 0f, size.toFloat(), size.toFloat()))
            drawContent(g, logo, initials, foreground, size, size * LEGACY_CONTENT_FRACTION)
        }
        return canvas
    }

    private fun drawContent(
        g: java.awt.Graphics2D,
        logo: BufferedImage?,
        initials: String,
        foreground: Color,
        size: Int,
        contentDiameter: Float,
    ) {
        if (logo != null) {
            val scale = contentDiameter / Math.hypot(logo.width.toDouble(), logo.height.toDouble()).toFloat()
            val w = (logo.width * scale).toInt().coerceAtLeast(1)
            val h = (logo.height * scale).toInt().coerceAtLeast(1)
            g.drawImage(logo, (size - w) / 2, (size - h) / 2, w, h, null)
            return
        }

        val text = initials.ifBlank { "?" }
        g.color = foreground
        // Scale linearly from a reference size, twice: font metrics aren't linear.
        var font = Font(Font.SANS_SERIF, Font.BOLD, (size / 2f).toInt().coerceAtLeast(1))
        repeat(2) {
            val bounds = g.getFontMetrics(font).getStringBounds(text, g)
            val diagonal = Math.hypot(bounds.width, bounds.height).toFloat()
            if (diagonal <= 0f) return@repeat
            val next = (font.size * (contentDiameter / diagonal)).toInt().coerceAtLeast(1)
            font = font.deriveFont(next.toFloat())
        }
        g.font = font
        val metrics = g.fontMetrics
        val bounds = metrics.getStringBounds(text, g)
        val x = ((size - bounds.width) / 2).toFloat()
        val y = ((size - bounds.height) / 2 - bounds.y).toFloat()
        g.drawString(text, x, y)
    }

    private fun blankCanvas(size: Int) = BufferedImage(size, size, BufferedImage.TYPE_INT_ARGB)

    private fun readImage(file: File): BufferedImage? =
        try {
            ImageIO.read(file)
        } catch (e: Exception) {
            logger.warn("Tenant icon: could not read ${file.name} (${e.message}); falling back to initials")
            null
        }

    private fun write(image: BufferedImage, target: File) {
        target.parentFile.mkdirs()
        ImageIO.write(image, "png", target)
    }

    private companion object {
        /** Adaptive layers are 108dp; the guaranteed-visible circle is 72dp. */
        const val SAFE_ZONE_FRACTION = 72f / 108f
        const val LEGACY_CONTENT_FRACTION = 0.72f

        val ADAPTIVE_DENSITIES = listOf(
            "mdpi" to 108, "hdpi" to 162, "xhdpi" to 216, "xxhdpi" to 324, "xxxhdpi" to 432,
        )
        val LEGACY_DENSITIES = listOf(
            "mdpi" to 48, "hdpi" to 72, "xhdpi" to 96, "xxhdpi" to 144, "xxxhdpi" to 192,
        )
    }
}

private inline fun java.awt.Graphics2D.use(block: (java.awt.Graphics2D) -> Unit) {
    setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON)
    setRenderingHint(RenderingHints.KEY_RENDERING, RenderingHints.VALUE_RENDER_QUALITY)
    setRenderingHint(RenderingHints.KEY_INTERPOLATION, RenderingHints.VALUE_INTERPOLATION_BICUBIC)
    setRenderingHint(RenderingHints.KEY_TEXT_ANTIALIASING, RenderingHints.VALUE_TEXT_ANTIALIAS_ON)
    try {
        block(this)
    } finally {
        dispose()
    }
}

internal fun parseColor(raw: String): Color {
    val hex = raw.trim().removePrefix("#")
    return when (hex.length) {
        6 -> Color(hex.toInt(16) or (0xFF shl 24), true)
        8 -> Color(hex.toLong(16).toInt(), true)
        else -> throw IllegalArgumentException("Icon colour must be #RRGGBB or #AARRGGBB, got \"$raw\"")
    }
}

internal fun toHex(c: Color): String = String.format("#%02X%02X%02X%02X", c.alpha, c.red, c.green, c.blue)

/** WCAG relative luminance, so initials stay legible on any brand colour. */
internal fun contrastingColor(background: Color): Color {
    fun channel(v: Int): Double {
        val s = v / 255.0
        return if (s <= 0.03928) s / 12.92 else Math.pow((s + 0.055) / 1.055, 2.4)
    }
    val luminance = 0.2126 * channel(background.red) +
        0.7152 * channel(background.green) +
        0.0722 * channel(background.blue)
    return if (luminance > 0.179) Color(0x1F, 0x1F, 0x1D) else Color.WHITE
}

fun initialsFor(name: String): String {
    val words = name.trim().split(Regex("[\\s\\-_]+")).filter { it.isNotBlank() }
    return when {
        words.isEmpty() -> "?"
        words.size == 1 -> words[0].take(2).uppercase()
        else -> (words[0].take(1) + words[1].take(1)).uppercase()
    }
}
