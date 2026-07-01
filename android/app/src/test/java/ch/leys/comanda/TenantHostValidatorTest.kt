package ch.leys.comanda

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class TenantHostValidatorTest {

    private fun validHost(input: String): String {
        val r = TenantHostValidator.validate(input)
        assertTrue("expected $input to be valid but was $r", r is TenantHostValidator.Result.Valid)
        return (r as TenantHostValidator.Result.Valid).host
    }

    private fun assertInvalid(input: String) {
        assertTrue("expected $input to be invalid", TenantHostValidator.validate(input) is TenantHostValidator.Result.Invalid)
    }

    @Test fun acceptsTenantSubdomain() = assertEquals("kasse.comanda.leys.ch", validHost("kasse.comanda.leys.ch"))

    @Test fun acceptsStagingTenantSubdomain() =
        assertEquals("acme.staging.comanda.leys.ch", validHost("acme.staging.comanda.leys.ch"))

    @Test fun acceptsCustomDomain() = assertEquals("shop.acme.ch", validHost("shop.acme.ch"))

    @Test fun stripsSchemeAndPathFromHttpsUrl() =
        assertEquals("acme.comanda.leys.ch", validHost("https://acme.comanda.leys.ch/pos"))

    @Test fun stripsPort() = assertEquals("acme.comanda.leys.ch", validHost("acme.comanda.leys.ch:443"))

    @Test fun lowercasesHost() = assertEquals("acme.comanda.leys.ch", validHost("ACME.Comanda.Leys.CH"))

    @Test fun rejectsNonHttpsScheme() = assertInvalid("http://acme.comanda.leys.ch")

    @Test fun rejectsPlatformApex() = assertInvalid("comanda.leys.ch")

    @Test fun rejectsStagingApex() = assertInvalid("staging.comanda.leys.ch")

    @Test fun rejectsReservedSlug() = assertInvalid("www.comanda.leys.ch")

    @Test fun rejectsMultiLevelSubdomain() = assertInvalid("a.b.comanda.leys.ch")

    @Test fun rejectsLocalhost() = assertInvalid("localhost")

    @Test fun rejectsBareWordWithoutDot() = assertInvalid("acme")

    @Test fun rejectsEmpty() = assertInvalid("   ")

    @Test fun effectivePosUrlFallsBackToBuildConfig() =
        assertEquals("http://localhost:3000/pos", TenantHostValidator.effectivePosUrl(null, "http://localhost:3000/pos"))

    @Test fun effectivePosUrlUsesHttpsOverride() =
        assertEquals("https://acme.comanda.leys.ch/pos", TenantHostValidator.effectivePosUrl("acme.comanda.leys.ch", "http://localhost:3000/pos"))

    @Test fun effectiveAllowedHostUsesOverride() =
        assertEquals("acme.comanda.leys.ch", TenantHostValidator.effectiveAllowedHost("acme.comanda.leys.ch", "http://localhost:3000/pos"))

    @Test fun effectiveAllowedHostParsesBuildConfigHost() =
        assertEquals("localhost", TenantHostValidator.effectiveAllowedHost(null, "http://localhost:3000/pos"))
}
