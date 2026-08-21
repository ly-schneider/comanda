package tenantbranding

import java.awt.Color
import kotlin.test.Test
import kotlin.test.assertEquals

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
        // Pure green is bright to the eye and light by luminance, so it must not
        // get white text — that pairing is the classic unreadable icon.
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
                // expected
            } catch (_: NumberFormatException) {
                // expected
            }
        }
    }
}
