package ch.leys.comanda

import android.content.Context

/** Runtime tenant-host override in SharedPreferences; falls back to BuildConfig.POS_URL. */
object DeviceConfig {
    private const val PREFS = "pos_prefs"
    private const val KEY_HOST = "pos_host_override"
    private const val KEY_RECENT = "pos_recent_hosts"
    private const val MAX_RECENT = 5

    private fun prefs(ctx: Context) = ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE)

    fun hostOverride(ctx: Context): String? = prefs(ctx).getString(KEY_HOST, null)?.ifBlank { null }

    fun effectivePosUrl(ctx: Context): String =
        TenantHostValidator.effectivePosUrl(hostOverride(ctx), BuildConfig.POS_URL)

    fun effectiveAllowedHost(ctx: Context): String? =
        TenantHostValidator.effectiveAllowedHost(hostOverride(ctx), BuildConfig.POS_URL)

    fun setHostOverride(ctx: Context, host: String) {
        prefs(ctx).edit().putString(KEY_HOST, host).apply()
        addRecent(ctx, host)
    }

    fun clearHostOverride(ctx: Context) {
        prefs(ctx).edit().remove(KEY_HOST).apply()
    }

    fun recentHosts(ctx: Context): List<String> =
        (prefs(ctx).getString(KEY_RECENT, "") ?: "").split("\n").map { it.trim() }.filter { it.isNotEmpty() }

    private fun addRecent(ctx: Context, host: String) {
        val list = ArrayList(recentHosts(ctx))
        list.remove(host)
        list.add(0, host)
        while (list.size > MAX_RECENT) list.removeAt(list.size - 1)
        prefs(ctx).edit().putString(KEY_RECENT, list.joinToString("\n")).apply()
    }
}
