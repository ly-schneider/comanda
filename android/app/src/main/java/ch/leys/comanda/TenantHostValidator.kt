package ch.leys.comanda

/**
 * Classifies an operator host + derives the effective POS URL. Mirrors web/lib/tenant.ts
 * (rejects the platform apex, which would fail-closed-brick the terminal). No android.net.Uri
 * so it stays plain-JUnit testable.
 */
object TenantHostValidator {

    private val RESERVED_SLUGS = setOf(
        "www", "api", "app", "staging", "static", "assets", "cdn", "mail", "smtp", "admin", "platform"
    )
    private val PLATFORM_BARE_HOSTS = setOf(
        "localhost", "127.0.0.1", "comanda.leys.ch", "staging.comanda.leys.ch"
    )
    private val LABEL = Regex("^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$")

    sealed interface Result {
        data class Valid(val host: String) : Result
        data class Invalid(val reason: String) : Result
    }

    fun validate(raw: String): Result {
        val input = raw.trim()
        if (input.isEmpty()) return Result.Invalid("Bitte einen Host eingeben.")

        val rawHost = if (input.contains("://")) {
            val scheme = input.substringBefore("://").lowercase()
            if (scheme != "https") return Result.Invalid("Nur HTTPS-Hosts sind erlaubt.")
            input.substringAfter("://")
        } else {
            input
        }.substringBefore("/").substringBefore("?").substringBefore("#")

        val host = rawHost.substringBefore(":").trim().lowercase()
        if (host.isEmpty() || !isValidHostname(host)) return Result.Invalid("Kein gültiger Host.")

        return when (classify(host)) {
            HostClass.PLATFORM -> Result.Invalid("Das ist ein Plattform-Host, kein Tenant.")
            HostClass.TENANT_SUBDOMAIN -> Result.Valid(host)
            HostClass.CUSTOM_DOMAIN ->
                if (host.contains(".")) Result.Valid(host) else Result.Invalid("Kein gültiger Host.")
        }
    }

    fun effectivePosUrl(overrideHost: String?, buildConfigUrl: String): String =
        if (overrideHost.isNullOrBlank()) buildConfigUrl else "https://$overrideHost/pos"

    fun effectiveAllowedHost(overrideHost: String?, buildConfigUrl: String): String? =
        if (!overrideHost.isNullOrBlank()) overrideHost else hostOf(buildConfigUrl)

    private enum class HostClass { PLATFORM, TENANT_SUBDOMAIN, CUSTOM_DOMAIN }

    private fun classify(host: String): HostClass {
        if (host in PLATFORM_BARE_HOSTS) return HostClass.PLATFORM
        // Staging apex must be checked before the prod apex — acme.staging.comanda.leys.ch
        // also ends with .comanda.leys.ch.
        slugClass(host, ".staging.comanda.leys.ch")?.let { return it }
        slugClass(host, ".comanda.leys.ch")?.let { return it }
        slugClass(host, ".localhost")?.let { return it }
        return HostClass.CUSTOM_DOMAIN
    }

    private fun slugClass(host: String, apex: String): HostClass? {
        if (!host.endsWith(apex)) return null
        val slug = host.removeSuffix(apex)
        return if (slug.isNotEmpty() && !slug.contains(".") && slug !in RESERVED_SLUGS)
            HostClass.TENANT_SUBDOMAIN
        else
            HostClass.PLATFORM
    }

    private fun hostOf(url: String): String? {
        val host = url.substringAfter("://", url).substringBefore("/").substringBefore("?")
            .substringBefore(":").trim().lowercase()
        return host.ifEmpty { null }
    }

    private fun isValidHostname(host: String): Boolean =
        host.length <= 253 && host.split(".").all { it.isNotEmpty() && LABEL.matches(it) }
}
