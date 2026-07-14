package ch.leys.comanda

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

class ReceiptFormattingTest {

    @Test fun cardMasksLast4() {
        assertEquals("Karte •••• 1234", prettyPayMethod("card", "1234"))
    }

    @Test fun cardWithoutLast4OmitsMask() {
        assertEquals("Karte", prettyPayMethod("card", null))
        assertEquals("Karte", prettyPayMethod("card", ""))
        assertEquals("Karte", prettyPayMethod("card", "   "))
    }

    @Test fun last4OnlyShownForCard() {
        assertEquals("Bar", prettyPayMethod("cash", "1234"))
        assertEquals("TWINT", prettyPayMethod("twint", "1234"))
    }

    @Test fun knownMethodsAreLabelled() {
        assertEquals("Bar", prettyPayMethod("cash", null))
        assertEquals("TWINT", prettyPayMethod("twint", null))
        assertEquals("Karte", prettyPayMethod("CARD", null))
    }

    @Test fun gratisMethodsAreLabelled() {
        assertEquals("Gratis (Gast)", prettyPayMethod("gratis_guest", null))
        assertEquals("Gratis (VIP)", prettyPayMethod("gratis_vip", null))
        assertEquals("Gratis (Mitarbeiter)", prettyPayMethod("gratis_staff", null))
        assertEquals("Gratis", prettyPayMethod("gratis_plugin", null))
    }

    @Test fun unknownMethodIsCapitalized() {
        assertEquals("Voucher", prettyPayMethod("voucher", null))
        assertEquals("", prettyPayMethod("", null))
    }

    @Test fun presentTimestampIsFormatted() {
        val ts = 1_700_000_000_000L
        val expected = SimpleDateFormat("dd.MM.yyyy HH:mm", Locale.GERMANY).format(Date(ts))
        assertEquals(expected, formatReceiptDateTime(ts, 0L))
    }

    @Test fun absentTimestampFallsBackToNow() {
        val now = 1_700_000_000_000L
        val expected = SimpleDateFormat("dd.MM.yyyy HH:mm", Locale.GERMANY).format(Date(now))
        assertEquals(expected, formatReceiptDateTime(-1L, now))
        assertEquals(expected, formatReceiptDateTime(0L, now))
    }

    // Byte-exact golden vector — identical to backend/internal/qrsign
    // (TestGoldenVector) and the web qr-verify test. Seed = 0x00..0x1f. The order
    // mixes a variant line (prod_pommes1 / vrnt_size_xl) and a no-variant line.
    private val goldenToken =
        "A3RlbmFudF9zZWVkMUhTNzlVMXlIN1pkM2VT8QACcHJvZF9wb21tZXMxdnJudF9zaXplX3hsAnByb2RfY29sYTAwMQAAAAAAAAAAAAAAAAH-io89pj7lJ4n0DW50ZTd5NU-q3E4FX1v8VUvSkn9XVq1-1uea_qSnTtS8W4XlnIHsbk0yK6ZT94-xQy6RX6QF"

    @Test fun decodesV3PickupTokenToRawBytes() {
        val bytes = decodePickupTokenBytes(goldenToken)
        assertNotNull(bytes)
        // 30-byte header + 2 lines × 25 + 64-byte signature = 144.
        assertEquals(144, bytes!!.size)
        assertEquals(0x03.toByte(), bytes[0])
        assertEquals("tenant_seed1", String(bytes, 1, 12, Charsets.ISO_8859_1))
        assertEquals("HS79U1yH7Zd3", String(bytes, 13, 12, Charsets.ISO_8859_1))
        // First line: product id then its variant id.
        assertEquals("prod_pommes1", String(bytes, 30, 12, Charsets.ISO_8859_1))
        assertEquals("vrnt_size_xl", String(bytes, 42, 12, Charsets.ISO_8859_1))
    }

    @Test fun nonV3TokensAreNotByteMode() {
        // Pickup-URL fallback: invalid base64url → text QR.
        assertNull(decodePickupTokenBytes("https://acme.comanda.leys.ch/o/HS79U1yH7Zd3"))
        // A non-v3 payload whose first byte isn't 0x03 (here base64url of "{...}").
        assertNull(decodePickupTokenBytes("eyJ2IjoxfQ"))
    }
}
