package ch.leys.comanda

/** Tenant/account match for SumUp card payments — pure so it stays plain-JUnit testable. */
object SumUpGuardrail {

    sealed interface Status {
        data class NotConfigured(val loggedIn: Boolean, val currentMerchant: String?) : Status
        object LoggedOut : Status
        data class Match(val merchant: String) : Status
        data class Mismatch(val current: String, val expected: String) : Status
        data class Unverifiable(val expected: String) : Status
    }

    fun evaluate(expected: String?, loggedIn: Boolean, currentMerchant: String?): Status {
        val exp = expected?.trim().orEmpty()
        val cur = currentMerchant?.trim().orEmpty()
        if (exp.isEmpty()) return Status.NotConfigured(loggedIn, cur.ifEmpty { null })
        if (!loggedIn) return Status.LoggedOut
        if (cur.isEmpty()) return Status.Unverifiable(exp)
        return if (cur.equals(exp, ignoreCase = true)) Status.Match(cur) else Status.Mismatch(cur, exp)
    }

    fun blocksCardCheckout(status: Status): Boolean = status is Status.Mismatch
}
