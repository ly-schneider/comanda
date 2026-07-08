package ch.leys.comanda

import org.junit.Assert.assertEquals
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
}
