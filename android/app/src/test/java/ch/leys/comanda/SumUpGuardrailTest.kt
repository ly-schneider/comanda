package ch.leys.comanda

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SumUpGuardrailTest {

    @Test fun matchingAccountIsAllowed() {
        val s = SumUpGuardrail.evaluate(expected = "MC123", loggedIn = true, currentMerchant = "MC123")
        assertTrue(s is SumUpGuardrail.Status.Match)
        assertFalse(SumUpGuardrail.blocksCardCheckout(s))
    }

    @Test fun mismatchHardBlocks() {
        val s = SumUpGuardrail.evaluate(expected = "MC123", loggedIn = true, currentMerchant = "MC999")
        assertTrue(s is SumUpGuardrail.Status.Mismatch)
        assertTrue(SumUpGuardrail.blocksCardCheckout(s))
    }

    @Test fun noExpectedIsAdvisoryOnly() {
        for (exp in listOf(null, "", "  ")) {
            val s = SumUpGuardrail.evaluate(expected = exp, loggedIn = true, currentMerchant = "MC999")
            assertTrue("expected NotConfigured for $exp", s is SumUpGuardrail.Status.NotConfigured)
            assertFalse(SumUpGuardrail.blocksCardCheckout(s))
        }
    }

    @Test fun loggedOutWithExpectedDoesNotBlock() {
        val s = SumUpGuardrail.evaluate(expected = "MC123", loggedIn = false, currentMerchant = null)
        assertTrue(s is SumUpGuardrail.Status.LoggedOut)
        assertFalse(SumUpGuardrail.blocksCardCheckout(s))
    }

    @Test fun unreadableMerchantFallsBackToAdvisory() {
        val s = SumUpGuardrail.evaluate(expected = "MC123", loggedIn = true, currentMerchant = null)
        assertTrue(s is SumUpGuardrail.Status.Unverifiable)
        assertFalse(SumUpGuardrail.blocksCardCheckout(s))
    }

    @Test fun comparisonIgnoresCaseAndWhitespace() {
        val s = SumUpGuardrail.evaluate(expected = "mc123", loggedIn = true, currentMerchant = " MC123 ")
        assertTrue(s is SumUpGuardrail.Status.Match)
    }
}
