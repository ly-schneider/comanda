package tenantbranding

import java.awt.Color
import java.awt.GradientPaint
import java.awt.image.BufferedImage
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class InitialsForTest {

    @Test
    fun `takes two letters from a single word`() {
        assertEquals("BL", initialsFor("BlessThun"))
    }

    @Test
    fun `takes one letter from each of the first two words`() {
        assertEquals("AF", initialsFor("Acme Food"))
        assertEquals("AF", initialsFor("Acme Food Company"))
    }

    @Test
    fun `splits on hyphens and underscores too`() {
        assertEquals("BF", initialsFor("bless-food"))
        assertEquals("BF", initialsFor("bless_food"))
    }

    @Test
    fun `survives names the icon still has to render`() {
        assertEquals("?", initialsFor(""))
        assertEquals("?", initialsFor("   "))
        assertEquals("X", initialsFor("x"))
        assertEquals("ÉT", initialsFor("Étoile"))
    }
}

class ContrastingColorTest {

    @Test
    fun `dark brand colour gets white text`() {
        assertEquals(Color.WHITE, contrastingColor(Color(0x0B, 0x5D, 0x3B)))
        assertEquals(Color.WHITE, contrastingColor(Color.BLACK))
    }

    @Test
    fun `light brand colour gets dark text`() {
        assertEquals(Color(0x1F, 0x1F, 0x1D), contrastingColor(Color.WHITE))
        assertEquals(Color(0x1F, 0x1F, 0x1D), contrastingColor(Color(0xF5, 0xF0, 0xE6)))
    }

    @Test
    fun `mid greens are judged on luminance not brightness`() {
        // Pure green is light by luminance; white text on it is the classic
        // unreadable icon.
        assertEquals(Color(0x1F, 0x1F, 0x1D), contrastingColor(Color(0x00, 0xFF, 0x00)))
    }
}

class ParseColorTest {

    @Test
    fun `accepts both hex forms`() {
        assertEquals(Color(0xFF, 0x00, 0x00, 0xFF), parseColor("#FF0000"))
        assertEquals(Color(0xFF, 0x00, 0x00, 0xFF), parseColor("FF0000"))
        assertEquals(Color(0x00, 0x00, 0xFF, 0x80), parseColor("#800000FF"))
    }

    @Test
    fun `rejects anything else rather than guessing`() {
        for (bad in listOf("", "#FFF", "red", "#GGGGGG")) {
            try {
                parseColor(bad)
                throw AssertionError("parseColor(\"$bad\") should have thrown")
            } catch (_: IllegalArgumentException) {
            } catch (_: NumberFormatException) {
            }
        }
    }
}

class MatteColorTest {

    private fun image(border: Color?, centre: Color): BufferedImage {
        val img = BufferedImage(64, 64, BufferedImage.TYPE_INT_ARGB)
        val g = img.createGraphics()
        if (border != null) {
            g.color = border
            g.fillRect(0, 0, 64, 64)
        }
        g.color = centre
        g.fillRect(16, 16, 32, 32)
        g.dispose()
        return img
    }

    @Test
    fun `flat opaque border is the matte`() {
        assertEquals(Color.WHITE, matteColor(image(Color.WHITE, Color.RED)))
        val teal = Color(0x29, 0xB4, 0xA7)
        assertEquals(teal, matteColor(image(teal, Color.WHITE)))
    }

    @Test
    fun `transparent border is not a matte`() {
        assertNull(matteColor(image(null, Color.RED)))
    }

    @Test
    fun `a partly transparent border is not a matte`() {
        assertNull(matteColor(image(Color(255, 255, 255, 128), Color.RED)))
    }

    @Test
    fun `a border that is not one flat colour is not a matte`() {
        val img = BufferedImage(64, 64, BufferedImage.TYPE_INT_ARGB)
        val g = img.createGraphics()
        g.paint = GradientPaint(0f, 0f, Color.RED, 64f, 64f, Color.BLUE)
        g.fillRect(0, 0, 64, 64)
        g.dispose()
        assertNull(matteColor(img))
    }

    @Test
    fun `near-identical border survives antialiasing noise`() {
        val img = image(Color.WHITE, Color.RED)
        // One slightly-off pixel, as a rescaled logo's edge would produce.
        img.setRGB(0, 0, Color(252, 253, 255).rgb)
        assertEquals(Color.WHITE, matteColor(img))
    }
}
