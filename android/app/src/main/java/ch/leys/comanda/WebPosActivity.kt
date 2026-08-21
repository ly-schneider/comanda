package ch.leys.comanda

import android.Manifest
import android.annotation.SuppressLint
import android.content.Intent
import android.content.BroadcastReceiver
import android.content.IntentFilter
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.view.GestureDetector
import android.view.MotionEvent
import android.webkit.JavascriptInterface
import android.webkit.PermissionRequest
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebSettings
import android.webkit.WebStorage
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.activity.ComponentActivity
import androidx.activity.result.contract.ActivityResultContracts
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import androidx.core.content.IntentCompat
import androidx.webkit.WebSettingsCompat
import androidx.webkit.WebViewFeature
import android.webkit.CookieManager
import android.bluetooth.BluetoothManager
import android.bluetooth.BluetoothDevice
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import com.google.zxing.BarcodeFormat
import com.google.zxing.EncodeHintType
import com.google.zxing.qrcode.QRCodeWriter
import com.google.zxing.qrcode.decoder.ErrorCorrectionLevel
import com.sumup.merchant.reader.api.SumUpAPI
import com.sumup.merchant.reader.api.SumUpLogin
import com.sumup.merchant.reader.api.SumUpPayment
import com.sumup.checkout.core.models.TransactionInfo
import java.math.BigDecimal
import org.json.JSONObject
import android.widget.Toast
import android.util.Log
import java.util.Base64
import java.util.UUID
import org.json.JSONArray
import android.bluetooth.BluetoothAdapter
import android.content.Context

/** Branding supplied by the web POS in the print payload. All fields optional/nullable. */
private data class ReceiptBranding(
    val displayName: String?,
    val logoUrl: String?,
    val address: String?,
    val currency: String?
)

private fun parseBranding(obj: JSONObject): ReceiptBranding {
    val b = obj.optJSONObject("branding")
    return if (b != null) ReceiptBranding(
        displayName = b.optString("displayName").ifBlank { null },
        logoUrl     = b.optString("logoUrl").ifBlank { null },
        address     = b.optString("address").ifBlank { null },
        currency    = b.optString("currency").ifBlank { null }
    ) else ReceiptBranding(null, null, null, null)
}

/** One refunded/redeemable line on a slip. */
private data class ReceiptLine(
    val title: String,
    val quantity: Int,
    val unitPriceCents: Long,
    val depositCents: Long,
    val configuration: List<Pair<String, String>>
)

/** Receipt kind sent by the web POS; absent or unknown ⇒ "sale". */
// A v3 pickup token is base64url of raw binary whose first byte is 0x03; decode
// it so the QR can be rendered byte-mode. Non-v3 (e.g. the pickup URL) stays text.
internal fun decodePickupTokenBytes(token: String): ByteArray? = try {
    val bytes = Base64.getUrlDecoder().decode(token.trim())
    if (bytes.size > 64 && bytes[0] == 0x03.toByte()) bytes else null
} catch (_: IllegalArgumentException) {
    null
}

private fun receiptKind(obj: JSONObject): String =
    when (obj.optString("kind", "sale").lowercase()) {
        "refund" -> "refund"
        "pickup" -> "pickup"
        else -> "sale"
    }

private fun prettyRefundMethod(method: String): String = when (method.lowercase()) {
    "twint" -> "TWINT"
    "cash-out", "cashout" -> "Cash-out"
    "cash" -> "Bar"
    "card" -> "Karte"
    else -> method
}

internal fun prettyPayMethod(method: String, cardLast4: String?): String {
    val base = when (method.lowercase()) {
        "twint" -> "TWINT"
        "cash" -> "Bar"
        "card" -> "Karte"
        "gratis_guest" -> "Gratis (Gast)"
        "gratis_vip" -> "Gratis (VIP)"
        "gratis_staff" -> "Gratis (Mitarbeiter)"
        "gratis_plugin" -> "Gratis"
        else -> if (method.isNotEmpty()) method.substring(0, 1).uppercase() + method.substring(1) else method
    }
    return if (method.equals("card", ignoreCase = true) && !cardLast4.isNullOrBlank()) "$base •••• $cardLast4" else base
}

internal fun formatReceiptDateTime(orderTs: Long, nowMs: Long): String = try {
    val whenMs = if (orderTs > 0) orderTs else nowMs
    java.text.SimpleDateFormat("dd.MM.yyyy HH:mm", java.util.Locale.GERMANY).format(java.util.Date(whenMs))
} catch (_: Throwable) { "" }

private fun receiptDateTime(obj: JSONObject): String =
    formatReceiptDateTime(obj.optLong("orderTimestamp", -1L), System.currentTimeMillis())

private fun moneyStr(currency: String, cents: Long): String =
    String.format("%s %.2f", currency, cents.toDouble() / 100.0)

private fun parseReceiptLines(obj: JSONObject): List<ReceiptLine> {
    val arr = obj.optJSONArray("items") ?: return emptyList()
    val out = ArrayList<ReceiptLine>(arr.length())
    for (i in 0 until arr.length()) {
        val it = arr.optJSONObject(i) ?: continue
        val cfg = ArrayList<Pair<String, String>>()
        it.optJSONArray("configuration")?.let { ca ->
            for (j in 0 until ca.length()) {
                val c = ca.optJSONObject(j) ?: continue
                cfg.add(c.optString("slot") to c.optString("choice"))
            }
        }
        out.add(
            ReceiptLine(
                title = it.optString("title").ifBlank { "Artikel" },
                quantity = it.optInt("quantity", 1),
                unitPriceCents = it.optLong("unitPriceCents", -1L),
                depositCents = it.optLong("depositCents", 0L),
                configuration = cfg
            )
        )
    }
    return out
}

class WebPosActivity : ComponentActivity() {

    companion object {
        private const val REQ_LOGIN = 1001
        private const val REQ_CHECKOUT = 1002
        private const val REQ_CAMERA = 3001
    }

    private lateinit var webView: WebView
    private var allowedHost: String? = null
    private var pendingPayment: PendingPayment? = null

    // Hidden operator gesture: long-press the top-left corner to open Device Setup.
    private val gestureDetector by lazy {
        GestureDetector(this, object : GestureDetector.SimpleOnGestureListener() {
            override fun onLongPress(e: MotionEvent) {
                val corner = resources.displayMetrics.density * 72f
                if (e.x <= corner && e.y <= corner) openDeviceSetup()
            }
        })
    }

    private val setupLauncher =
        registerForActivityResult(ActivityResultContracts.StartActivityForResult()) { res ->
            if (res.resultCode == RESULT_OK &&
                res.data?.getBooleanExtra(DeviceSetupActivity.EXTRA_HOST_CHANGED, false) == true
            ) {
                onTenantHostChanged()
            }
        }
    // WebView camera permission request held while the OS CAMERA grant is pending
    private var pendingCameraRequest: PermissionRequest? = null
    // Store last successful SumUp transaction details by correlationId/reference
    private val txByCorrelation: MutableMap<String, JSONObject> = mutableMapOf()
    // Last successful transaction (fallback when correlationId is not provided)
    private var lastTx: JSONObject? = null

    // Simple prefs for storing selected printer
    private val prefs by lazy { getSharedPreferences("pos_prefs", Context.MODE_PRIVATE) }
    private fun saveSelectedPrinter(mac: String?) {
        prefs.edit().putString("printer_mac", mac).apply()
    }
    private fun loadSelectedPrinter(): String? = prefs.getString("printer_mac", null)

    private fun clearWebCacheOnUpgrade(wv: WebView) {
        val last = prefs.getInt("last_version_code", -1)
        if (BuildConfig.VERSION_CODE > last) {
            wv.clearCache(true)
            prefs.edit().putInt("last_version_code", BuildConfig.VERSION_CODE).apply()
        }
    }

    // Bluetooth discovery receiver lifecycle
    private var btReceiver: BroadcastReceiver? = null
    private var discoveryActive: Boolean = false
    private fun registerBtReceiver() {
        if (btReceiver != null) return
        btReceiver = object : BroadcastReceiver() {
            override fun onReceive(context: Context?, intent: Intent?) {
                when (intent?.action) {
                    BluetoothDevice.ACTION_FOUND -> {
                        try {
                            val dev: BluetoothDevice? = intent.getParcelableExtra(BluetoothDevice.EXTRA_DEVICE)
                            if (dev != null) {
                                val name = try { dev.name ?: "Bluetooth Device" } catch (_: SecurityException) { "Bluetooth Device" } catch (_: Throwable) { "Bluetooth Device" }
                                val addr = try { dev.address ?: "" } catch (_: SecurityException) { "" } catch (_: Throwable) { "" }
                                val bonded = try { dev.bondState == BluetoothDevice.BOND_BONDED } catch (_: Throwable) { false }
                                val proto = if (name.contains("T02", true) || name.contains("Phomemo", true)) "phomemo" else "escpos"
                                if (addr.isNotBlank()) {
                                    sendEvent(
                                        "comanda:printer:found",
                                        JSONObject().put("name", name).put("address", addr).put("bonded", bonded).put("protocol", proto)
                                    )
                                }
                            }
                        } catch (_: Throwable) {}
                    }
                    BluetoothDevice.ACTION_BOND_STATE_CHANGED -> {
                        val dev: BluetoothDevice? = intent.getParcelableExtra(BluetoothDevice.EXTRA_DEVICE)
                        if (dev != null) {
                            val addr = try { dev.address ?: "" } catch (_: Throwable) { "" }
                            val state = when (dev.bondState) {
                                BluetoothDevice.BOND_BONDING -> "bonding"
                                BluetoothDevice.BOND_BONDED -> "bonded"
                                else -> "none"
                            }
                            if (addr.isNotBlank()) {
                                sendEvent(
                                    "comanda:printer:bond:state",
                                    JSONObject().put("address", addr).put("state", state)
                                )
                            }
                        }
                    }
                    BluetoothAdapter.ACTION_DISCOVERY_FINISHED -> {
                        discoveryActive = false
                        sendEvent("comanda:printer:discovery:finished", JSONObject())
                    }
                }
            }
        }
        val f = IntentFilter().apply {
            addAction(BluetoothDevice.ACTION_FOUND)
            addAction(BluetoothDevice.ACTION_BOND_STATE_CHANGED)
            addAction(BluetoothAdapter.ACTION_DISCOVERY_FINISHED)
        }
        try { registerReceiver(btReceiver, f) } catch (_: Throwable) {}
    }

    private fun unregisterBtReceiver() {
        val r = btReceiver ?: return
        btReceiver = null
        try { unregisterReceiver(r) } catch (_: Throwable) {}
    }

    data class PendingPayment(
        val amountCents: Long,
        val currency: String,
        val reference: String,
        val title: String?,
        val items: String?
    )

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        allowedHost = DeviceConfig.effectiveAllowedHost(this)
        webView = WebView(this)
        configureWebView(webView)
        setContentView(webView)

        // Bridge for PosBridge.* calls from /pos page.
        //
        // The bridge hands the loaded page card payments, printing and the
        // scanner, so a tenant build refuses to expose it anywhere but the host
        // it was built for. Nothing can move that host today — Device Setup
        // hides the field in tenant builds — so this is an invariant check that
        // fails closed rather than a live gate.
        if (bridgeHostAllowed()) {
            webView.addJavascriptInterface(PosBridge(), "PosBridge")
        } else {
            Log.e("WebPosActivity", "refusing to expose PosBridge: host $allowedHost is not this build's tenant host")
        }

        webView.loadUrl(DeviceConfig.effectivePosUrl(this))

        AppUpdateChecker(this).checkForUpdate()
    }

    override fun onDestroy() {
        super.onDestroy()
        try {
            val mgr = getSystemService(BLUETOOTH_SERVICE) as BluetoothManager
            val adapter = mgr.adapter
            if (adapter != null && adapter.isDiscovering) {
                try { adapter.cancelDiscovery() } catch (_: Throwable) {}
            }
        } catch (_: Throwable) {}
        unregisterBtReceiver()
    }

    override fun dispatchTouchEvent(ev: MotionEvent): Boolean {
        // Watch for the setup gesture without consuming events — the WebView still handles them.
        try { gestureDetector.onTouchEvent(ev) } catch (_: Throwable) {}
        return super.dispatchTouchEvent(ev)
    }

    private fun openDeviceSetup() {
        setupLauncher.launch(Intent(this, DeviceSetupActivity::class.java))
    }

    private fun bridgeHostAllowed(): Boolean {
        if (!BuildConfig.TENANT_BUILD) return true
        val baked = TenantHostValidator.effectiveAllowedHost(null, BuildConfig.POS_URL)
        return baked != null && allowedHost == baked
    }

    // Switching tenants: wipe the old session (device token in localStorage + auth cookies) so the app re-pairs.
    private fun onTenantHostChanged() {
        try { WebStorage.getInstance().deleteAllData() } catch (_: Throwable) {}
        DeviceConfig.clearTenantConfig(this)
        val cm = CookieManager.getInstance()
        cm.removeAllCookies(null)
        cm.flush()
        webView.clearHistory()
        allowedHost = DeviceConfig.effectiveAllowedHost(this)
        webView.loadUrl(DeviceConfig.effectivePosUrl(this))
        try { Toast.makeText(this, "Host aktualisiert", Toast.LENGTH_SHORT).show() } catch (_: Throwable) {}
    }

    @SuppressLint("SetJavaScriptEnabled")
    private fun configureWebView(wv: WebView) {
        val s: WebSettings = wv.settings
        s.javaScriptEnabled = true
        s.domStorageEnabled = true
        s.databaseEnabled = false
        s.allowFileAccess = false
        s.allowContentAccess = false
        s.javaScriptCanOpenWindowsAutomatically = false
        s.mediaPlaybackRequiresUserGesture = true
        s.cacheMode = WebSettings.LOAD_DEFAULT
        clearWebCacheOnUpgrade(wv)

        if (WebViewFeature.isFeatureSupported(WebViewFeature.SAFE_BROWSING_ENABLE)) {
            WebSettingsCompat.setSafeBrowsingEnabled(s, true)
        }

        // Allow mixed content only in dev builds to support http://10.0.2.2
        if (BuildConfig.DEV_BUILD) {
            s.mixedContentMode = WebSettings.MIXED_CONTENT_COMPATIBILITY_MODE
        } else {
            s.mixedContentMode = WebSettings.MIXED_CONTENT_NEVER_ALLOW
        }

        if (WebViewFeature.isFeatureSupported(WebViewFeature.FORCE_DARK)) {
            WebSettingsCompat.setForceDark(s, WebSettingsCompat.FORCE_DARK_AUTO)
        }

        // Allow cookies for cross-domain auth flows; third-party cookies only in dev
        val cm = CookieManager.getInstance()
        cm.setAcceptCookie(true)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.LOLLIPOP) {
            cm.setAcceptThirdPartyCookies(wv, BuildConfig.DEV_BUILD)
        }

        wv.webViewClient = object : WebViewClient() {
            override fun shouldOverrideUrlLoading(
                view: WebView,
                request: WebResourceRequest
            ): Boolean {
                val url = request.url
                val host = url.host
                // Allow in-app only the configured host; open others externally.
                // Cloudflare Access team domains stay in-app so the transparent
                // WARP login redirect on gated hosts can complete in the WebView.
                val inApp = host != null &&
                    (host == allowedHost || host.endsWith(".cloudflareaccess.com"))
                return if (inApp) {
                    false
                } else {
                    startActivity(Intent(Intent.ACTION_VIEW, url))
                    true
                }
            }
        }
        wv.webChromeClient = object : WebChromeClient() {
            // The web station scanner (zxing-wasm) calls getUserMedia; grant the camera so it runs in-app.
            override fun onPermissionRequest(request: PermissionRequest) {
                val wantsCamera = request.resources.any { it == PermissionRequest.RESOURCE_VIDEO_CAPTURE }
                if (!wantsCamera) {
                    request.deny()
                    return
                }
                if (ContextCompat.checkSelfPermission(this@WebPosActivity, Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED) {
                    request.grant(arrayOf(PermissionRequest.RESOURCE_VIDEO_CAPTURE))
                } else {
                    pendingCameraRequest = request
                    ActivityCompat.requestPermissions(this@WebPosActivity, arrayOf(Manifest.permission.CAMERA), REQ_CAMERA)
                }
            }

            override fun onPermissionRequestCanceled(request: PermissionRequest) {
                if (pendingCameraRequest == request) pendingCameraRequest = null
            }
        }
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode == REQ_CAMERA) {
            val req = pendingCameraRequest
            pendingCameraRequest = null
            if (req != null) {
                val granted = grantResults.isNotEmpty() && grantResults[0] == PackageManager.PERMISSION_GRANTED
                if (granted) req.grant(arrayOf(PermissionRequest.RESOURCE_VIDEO_CAPTURE)) else req.deny()
            }
        }
    }

    inner class PosBridge {
        // Called from JS: PosBridge.payWithCard({ amountCents, currency, reference })
        @JavascriptInterface
        fun payWithCard(json: String) {
            var ref: String = ""
            try {
                val obj = try { JSONObject(json) } catch (_: Throwable) { null }
                if (obj == null) {
                    sendEvent(
                        "comanda:sumup:result",
                        JSONObject().put("success", false).put("error", "invalid_request").put("correlationId", "")
                    )
                    return
                }
                val amountCents = obj.optLong("amountCents", -1L)
                val currency = obj.optString("currency", "CHF")
                ref = obj.optString("reference", "pos_${System.currentTimeMillis()}")
                if (amountCents <= 0) {
                    sendEvent(
                        "comanda:sumup:result",
                        JSONObject().put("success", false).put("error", "invalid_amount").put("correlationId", ref)
                    )
                    return
                }
                val title = obj.optString("title").ifBlank { null }
                val items = obj.optString("items").ifBlank { null }
                val pending = PendingPayment(amountCents, currency, ref, title, items)
                runOnUiThread { startSumUpCheckout(pending) }
            } catch (e: Exception) {
                sendEvent(
                    "comanda:sumup:result",
                    JSONObject().put("success", false).put("error", e.message ?: "invalid_request").put("correlationId", ref)
                )
            }
        }

        // Called from JS after /pos/me loads: PosBridge.setTenantConfig({ sumupMerchantCode, tenantName })
        @JavascriptInterface
        fun setTenantConfig(json: String) {
            try {
                val obj = JSONObject(json)
                val merchant = if (obj.isNull("sumupMerchantCode")) null
                    else obj.optString("sumupMerchantCode").ifBlank { null }
                val name = if (obj.isNull("tenantName")) null
                    else obj.optString("tenantName").ifBlank { null }
                DeviceConfig.setTenantConfig(this@WebPosActivity, merchant, name)
            } catch (_: Throwable) {}
        }

        // Called from JS: PosBridge.print(jsonStringOrObject)
        @JavascriptInterface
        fun print(payload: String) {
            val correlationId = try {
                val obj = JSONObject(payload)
                obj.optString("correlationId", "print_${System.currentTimeMillis()}")
            } catch (_: Exception) {
                "print_${System.currentTimeMillis()}"
            }

            // For MVP: use system print for HTML, ESC/POS for plain receipt
            try {
                val obj = try { JSONObject(payload) } catch (_: Exception) { JSONObject() }

                // Skip printing silently if explicitly requested (e.g., Jeton mode)
                val skipPrint = obj.optBoolean("skipPrint", false)
                if (skipPrint) {
                    sendEvent(
                        "comanda:print:result",
                        JSONObject().put("success", true).put("skipped", true).put("correlationId", correlationId)
                    )
                    return
                }

                val mode = obj.optString("mode", "escpos")
                if (mode == "system") {
                    val html = obj.optString("content", payload)
                    printWithSystem(html) { success, error ->
                        sendEvent(
                            "comanda:print:result",
                            JSONObject().put("success", success).put("error", error ?: JSONObject.NULL).put("correlationId", correlationId)
                        )
                    }
                } else {
                    // ESC/POS (and Phomemo fallback) path
                    // Try to augment receipt content with SumUp tx details if available
                    val rawContent = obj.optString("content", payload)
                    // Optional explicit printer selection in payload
                    val explicitPrinterAddr = obj.optString("printerAddress", obj.optString("printer", "")).ifBlank { null }
                    val augmentedContent = try {
                        val rc = JSONObject(rawContent)
                        val method = rc.optString("method", "").lowercase()
                        if (method == "card" && !rc.has("cardLast4")) {
                            val tx = txByCorrelation[correlationId] ?: lastTx
                            if (tx != null) {
                                // Copy over known non-sensitive card + tx fields
                                tx.optString("cardLast4").takeIf { it.isNotEmpty() }?.let { rc.put("cardLast4", it) }
                                tx.optString("cardType").takeIf { it.isNotEmpty() }?.let { rc.put("cardType", it) }
                                tx.optString("entryMode").takeIf { it.isNotEmpty() }?.let { rc.put("entryMode", it) }
                                tx.optString("txId").takeIf { it.isNotEmpty() }?.let { rc.put("txId", it) }
                                tx.optString("status").takeIf { it.isNotEmpty() }?.let { rc.put("txStatus", it) }
                                // Prefer currency from tx; keep existing if already set
                                if (!rc.has("currency")) tx.optString("currency").takeIf { it.isNotEmpty() }?.let { rc.put("currency", it) }
                                if (!rc.has("totalCents")) tx.optLong("amountCents", -1L).takeIf { it >= 0 }?.let { rc.put("totalCents", it) }
                            }
                        }
                        rc.toString()
                    } catch (_: Throwable) {
                        rawContent
                    }
                    val ok = printWithEscPos(augmentedContent, explicitPrinterAddr)
                    sendEvent(
                        "comanda:print:result",
                        JSONObject().put("success", ok).put("correlationId", correlationId)
                    )
                }
            } catch (e: Exception) {
                sendEvent(
                    "comanda:print:result",
                    JSONObject().put("success", false).put("error", e.message ?: "print failed").put("correlationId", correlationId)
                )
            }
        }

        // Return a JSON array string of paired Bluetooth printers the app can see
        // [{ name, address, protocol }]
        @JavascriptInterface
        fun listPrinters(): String {
            if (!ensureBtPermissions()) return "[]"
            val arr = JSONArray()
            try {
                // Prefer DantSu helper to enumerate known printer connections
                val dsuList = try {
                    com.dantsu.escposprinter.connection.bluetooth.BluetoothPrintersConnections().list
                } catch (_: Throwable) { null }
                if (dsuList != null && dsuList.isNotEmpty()) {
                    for (conn in dsuList) {
                        try {
                            val dev = try { conn.device } catch (_: Throwable) { null }
                            val name = try { dev?.name ?: "Bluetooth Printer" } catch (_: Throwable) { "Bluetooth Printer" }
                            val addr = try { dev?.address ?: "" } catch (_: Throwable) { "" }
                            val proto = if (name.contains("T02", true) || name.contains("Phomemo", true)) "phomemo" else "escpos"
                            if (addr.isNotBlank()) {
                                arr.put(
                                    JSONObject().put("name", name).put("address", addr).put("protocol", proto)
                                )
                            }
                        } catch (_: Throwable) { /* ignore entry */ }
                    }
                } else {
                    // Fallback: list bonded devices from adapter
                    val mgr = getSystemService(BLUETOOTH_SERVICE) as BluetoothManager
                    val adapter = mgr.adapter
                    val bonded = try { adapter?.bondedDevices } catch (_: SecurityException) { null }
                    bonded?.forEach { dev ->
                        try {
                            val name = try { dev.name } catch (_: Throwable) { "Bluetooth Device" }
                            val addr = try { dev.address } catch (_: Throwable) { "" }
                            if (addr.isNotBlank()) {
                                val proto = if (name.contains("T02", true) || name.contains("Phomemo", true)) "phomemo" else "escpos"
                                arr.put(JSONObject().put("name", name).put("address", addr).put("protocol", proto))
                            }
                        } catch (_: Throwable) { }
                    }
                }
            } catch (_: Throwable) { }
            return arr.toString()
        }

        // Persist selected printer MAC address (null or empty clears selection)
        @JavascriptInterface
        fun selectPrinter(address: String?) {
            val mac = address?.trim().orEmpty()
            if (mac.isBlank()) saveSelectedPrinter(null) else saveSelectedPrinter(mac)
        }

        // Return current selected printer MAC (or empty)
        @JavascriptInterface
        fun getSelectedPrinter(): String {
            return loadSelectedPrinter() ?: ""
        }

        // Start Bluetooth discovery; emits comanda:printer:found events and comanda:printer:discovery:finished
        @JavascriptInterface
        fun startDiscovery(): Boolean {
            if (!ensureBtPermissions()) return false
            val mgr = getSystemService(BLUETOOTH_SERVICE) as BluetoothManager
            val adapter = mgr.adapter
            if (adapter == null || !adapter.isEnabled) return false
            if (!hasBtScanPermission()) return false
            try {
                if (adapter.isDiscovering) {
                    try { adapter.cancelDiscovery() } catch (_: Throwable) {}
                }
                registerBtReceiver()
                discoveryActive = adapter.startDiscovery()
                return discoveryActive
            } catch (_: SecurityException) {
                return false
            } catch (_: Throwable) {
                return false
            }
        }

        // Stop Bluetooth discovery
        @JavascriptInterface
        fun stopDiscovery() {
            try {
                val mgr = getSystemService(BLUETOOTH_SERVICE) as BluetoothManager
                val adapter = mgr.adapter
                if (adapter != null && adapter.isDiscovering) {
                    try { adapter.cancelDiscovery() } catch (_: Throwable) {}
                }
            } catch (_: Throwable) {}
            discoveryActive = false
        }

        // Attempt to pair (bond) with a device by MAC address
        @JavascriptInterface
        fun pair(address: String?): Boolean {
            if (!ensureBtPermissions()) return false
            val mac = address?.trim().orEmpty()
            if (mac.isBlank()) return false
            return try {
                val mgr = getSystemService(BLUETOOTH_SERVICE) as BluetoothManager
                val adapter = mgr.adapter
                if (adapter == null || !adapter.isEnabled) return false
                if (!hasBtConnectPermission()) return false
                val dev = try { adapter.getRemoteDevice(mac) } catch (_: SecurityException) { return false }
                // Kick off bond; user may see a pairing dialog
                try { dev.createBond() } catch (_: SecurityException) { return false }
                true
            } catch (_: Throwable) {
                false
            }
        }

        @JavascriptInterface
        fun getAppVersion(): String =
            JSONObject()
                .put("versionName", BuildConfig.VERSION_NAME)
                .put("versionCode", BuildConfig.VERSION_CODE)
                .toString()

        @JavascriptInterface
        fun checkForUpdate() {
            runOnUiThread { AppUpdateChecker(this@WebPosActivity).checkForUpdate(manual = true) }
        }
    }

    private fun startSumUpCheckout(p: PendingPayment) {
        pendingPayment = p
        val payment = try {
            val builder = SumUpPayment.builder()
                .total(BigDecimal(p.amountCents).divide(BigDecimal(100)))
                .currency(SumUpPayment.Currency.valueOf(p.currency))
                .foreignTransactionId(p.reference)
                .skipSuccessScreen()
            // Only set a title when the POS supplies one; otherwise SumUp labels the
            // charge with its own unique transaction code, which stays searchable.
            p.title?.takeIf { it.isNotBlank() }?.let { t ->
                builder.title(if (t.length > 120) t.substring(0, 119) + "…" else t)
            }
            p.items?.takeIf { it.isNotBlank() }?.let { info ->
                builder.addAdditionalInfo("Artikel", if (info.length > 500) info.substring(0, 500) else info)
            }
            builder.build()
        } catch (t: Throwable) {
            sendEvent(
                "comanda:sumup:result",
                JSONObject().put("success", false).put("error", "invalid_currency").put("correlationId", p.reference)
            )
            return
        }

        if (BuildConfig.SUMUP_AFFILIATE_KEY.isBlank()) {
            sendEvent(
                "comanda:sumup:result",
                JSONObject().put("success", false).put("error", "missing_affiliate_key").put("correlationId", p.reference)
            )
            return
        }

        if (SumUpAPI.isLoggedIn()) {
            val status = SumUpGuardrail.evaluate(
                DeviceConfig.expectedSumUpMerchant(this),
                loggedIn = true,
                currentMerchant = try { SumUpAPI.getCurrentMerchant()?.merchantCode } catch (_: Throwable) { null }
            )
            if (status is SumUpGuardrail.Status.Mismatch) {
                sendEvent(
                    "comanda:sumup:result",
                    JSONObject()
                        .put("success", false)
                        .put(
                            "error",
                            "Falsches SumUp-Konto: angemeldet als ${status.current}, erwartet ${status.expected}. " +
                                "Bitte in der Geräte-Einrichtung das richtige Konto anmelden."
                        )
                        .put("correlationId", p.reference)
                )
                pendingPayment = null
                return
            }
            SumUpAPI.checkout(this, payment, REQ_CHECKOUT)
        } else {
            val login = SumUpLogin.builder(BuildConfig.SUMUP_AFFILIATE_KEY).build()
            SumUpAPI.openLoginActivity(this, login, REQ_LOGIN)
        }
    }

    // SumUpPayment.Builder now uses .skipSuccessScreen() explicitly per SDK docs

    @Deprecated("SumUp SDK uses legacy result callback")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        val bundle = data?.extras ?: return
        when (requestCode) {
            REQ_LOGIN -> {
                val rc = bundle.getInt(SumUpAPI.Response.RESULT_CODE)
                if (rc == SumUpAPI.Response.ResultCode.SUCCESSFUL) {
                    pendingPayment?.let { startSumUpCheckout(it) }
                } else {
                    val ref = pendingPayment?.reference ?: ""
                    sendEvent(
                        "comanda:sumup:result",
                        JSONObject().put("success", false).put("error", "login_failed").put("correlationId", ref)
                    )
                    pendingPayment = null
                }
            }
            REQ_CHECKOUT -> {
                val ref = pendingPayment?.reference ?: ""
                val rc = bundle.getInt(SumUpAPI.Response.RESULT_CODE)
                if (rc == SumUpAPI.Response.ResultCode.SUCCESSFUL) {
                    val txCode = bundle.getString(SumUpAPI.Response.TX_CODE)

                    val txJson = JSONObject()
                    val txInfo = try {
                        IntentCompat.getParcelableExtra(data, SumUpAPI.Response.TX_INFO, TransactionInfo::class.java)
                    } catch (_: Throwable) { null }
                    txInfo?.let { info ->
                        // Card brand + last4 live on the nested Card object, not flat on TransactionInfo
                        info.card?.type?.let { txJson.put("cardType", it) }
                        info.card?.last4Digits?.let { txJson.put("cardLast4", it) }
                        info.entryMode?.let { txJson.put("entryMode", it) }
                        info.currency?.let { txJson.put("currency", it) }
                        info.transactionCode?.let { txJson.put("txId", it) }
                        info.status?.let { txJson.put("status", it) }
                    }

                    // Always include txId (from bundle) and correlation id
                    if (!txJson.has("txId")) txJson.put("txId", txCode ?: "")
                    txJson.put("correlationId", ref)
                    // Fill amount/currency from the pending payment if still missing
                    pendingPayment?.let {
                        if (!txJson.has("amountCents")) txJson.put("amountCents", it.amountCents)
                        if (!txJson.has("currency")) txJson.put("currency", it.currency)
                    }
                    // Mark overall status
                    txJson.put("success", true)

                    // Keep for later augmentation during print
                    if (ref.isNotBlank()) txByCorrelation[ref] = txJson
                    lastTx = txJson

                    sendEvent(
                        "comanda:sumup:result",
                        txJson
                    )
                } else {
                    val msg = bundle.getString(SumUpAPI.Response.MESSAGE) ?: "payment_failed"
                    sendEvent(
                        "comanda:sumup:result",
                        JSONObject().put("success", false).put("error", msg).put("correlationId", ref)
                    )
                }
                pendingPayment = null
            }
        }
    }

    // Note: We only surface non-sensitive card details (brand, last4, entry mode)

    private fun sendEvent(name: String, detail: JSONObject) {
        val js = (
            "(function(){try{var d=" + JSONObject.quote(detail.toString()) +
                ";var o=JSON.parse(d);window.dispatchEvent(new CustomEvent('" + name + "',{detail:o}));}catch(e){console.error(e);}})();"
            )
        runOnUiThread { webView.evaluateJavascript(js, null) }
    }

    private fun ensureBtPermissions(): Boolean {
        val needs = mutableListOf<String>()
        if (Build.VERSION.SDK_INT >= 31) {
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.BLUETOOTH_CONNECT) != PackageManager.PERMISSION_GRANTED) {
                needs.add(Manifest.permission.BLUETOOTH_CONNECT)
            }
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.BLUETOOTH_SCAN) != PackageManager.PERMISSION_GRANTED) {
                needs.add(Manifest.permission.BLUETOOTH_SCAN)
            }
        }
        return if (needs.isNotEmpty()) {
            ActivityCompat.requestPermissions(this, needs.toTypedArray(), 2001)
            false
        } else true
    }

    private fun hasBtConnectPermission(): Boolean {
        return if (Build.VERSION.SDK_INT >= 31) {
            ContextCompat.checkSelfPermission(this, Manifest.permission.BLUETOOTH_CONNECT) == PackageManager.PERMISSION_GRANTED
        } else true
    }

    private fun hasBtScanPermission(): Boolean {
        return if (Build.VERSION.SDK_INT >= 31) {
            ContextCompat.checkSelfPermission(this, Manifest.permission.BLUETOOTH_SCAN) == PackageManager.PERMISSION_GRANTED
        } else true
    }

    // Very small HTML print helper via hidden WebView
    private fun printWithSystem(htmlContent: String, onResult: (success: Boolean, error: String?) -> Unit) {
        val pv = WebView(this)
        pv.settings.javaScriptEnabled = false
        pv.loadDataWithBaseURL(null, htmlContent, "text/html", "UTF-8", null)
        pv.post {
            try {
                val printMgr = getSystemService(android.content.Context.PRINT_SERVICE) as android.print.PrintManager
                val adapter = pv.createPrintDocumentAdapter("POS_Receipt")
                val job = printMgr.print("POS_Receipt", adapter, android.print.PrintAttributes.Builder().build())

                // Poll job state until it finishes one way or another
                val handler = android.os.Handler(android.os.Looper.getMainLooper())
                fun check() {
                    when {
                        job.isCompleted -> onResult(true, null)
                        job.isFailed -> onResult(false, "print_failed")
                        job.isCancelled -> onResult(false, "print_cancelled")
                        else -> handler.postDelayed({ check() }, 400)
                    }
                }
                check()
            } catch (t: Throwable) {
                onResult(false, t.message ?: "print_failed")
            }
        }
    }

    // Minimal ESC/POS text printing using DantSu library; returns success
    private fun printWithEscPos(content: String, preferredAddress: String? = null): Boolean {
        if (!ensureBtPermissions()) return false
        return try {
            val connection = run {
                val addr = preferredAddress ?: loadSelectedPrinter()
                if (!addr.isNullOrBlank()) {
                    val mgr = getSystemService(BLUETOOTH_SERVICE) as BluetoothManager
                    val adapter = mgr.adapter
                    if (adapter == null || !adapter.isEnabled) throw Exception("bt_disabled")
                    if (!hasBtConnectPermission()) throw Exception("missing_bt_permission")
                    val dev = try { adapter.getRemoteDevice(addr) } catch (_: SecurityException) { throw Exception("bt_connect_permission_denied") }
                    com.dantsu.escposprinter.connection.bluetooth.BluetoothConnection(dev)
                } else {
                    com.dantsu.escposprinter.connection.bluetooth.BluetoothPrintersConnections.selectFirstPaired()
                        ?: throw Exception("no_paired_printer")
                }
            }
            // If the paired device looks like a Phomemo T02 (non-ESC/POS), use raw raster path
            val devName = if (hasBtConnectPermission()) {
                try { connection.device?.name } catch (_: SecurityException) { null } catch (_: Throwable) { null }
            } else null
            val devAddr = if (hasBtConnectPermission()) {
                try { connection.device?.address } catch (_: SecurityException) { null } catch (_: Throwable) { null }
            } else null
            if ((devName?.contains("T02", ignoreCase = true) == true || devName?.contains("Phomemo", ignoreCase = true) == true) && !devAddr.isNullOrBlank()) {
                Log.d("POS_PRINT", "Detected Phomemo-like printer: $devName ($devAddr), using raw raster protocol")
                try { connection.disconnect() } catch (_: Throwable) {}
                return printWithPhomemo(devAddr, content)
            }

            // Try explicit connect to surface errors early for ESC/POS printers
            try {
                connection.connect()
                try {
                    val name = if (hasBtConnectPermission()) {
                        try { connection.device.name } catch (_: SecurityException) { "Bluetooth Printer" } catch (_: Throwable) { "Bluetooth Printer" }
                    } else "Bluetooth Printer"
                    Log.d("POS_PRINT", "Connected to printer: $name")
                } catch (_: Throwable) {}
            } catch (t: Throwable) {
                Log.e("POS_PRINT", "Bluetooth connect failed", t)
                throw Exception(t.message ?: "bt_connect_failed")
            }
            val printer = com.dantsu.escposprinter.EscPosPrinter(connection, 203, 48f, 32)
            // Print the receipt as a raster image; the printer's internal font inks unevenly.
            val obj = try { JSONObject(content) } catch (_: Exception) { JSONObject() }
            val branding = parseBranding(obj)
            val bmp = renderReceiptBitmap(obj, branding)
            val imgHex = com.dantsu.escposprinter.textparser.PrinterTextParserImg.bitmapToHexadecimalString(printer, bmp)
            printer.printFormattedText("[C]<img>$imgHex</img>\n")
            try { connection.disconnect() } catch (_: Throwable) {}
            true
        } catch (e: Exception) {
            e.printStackTrace()
            // Only show toast for unexpected errors, not for "no_paired_printer"
            if (e.message != "no_paired_printer") {
                try { Toast.makeText(this, "Drucken fehlgeschlagen: ${e.message}", Toast.LENGTH_SHORT).show() } catch (_: Throwable) {}
            }
            false
        }
    }

    // Raw raster printing for Phomemo T02-like printers using SPP socket
    private fun printWithPhomemo(macAddress: String, content: String): Boolean {
        if (!ensureBtPermissions()) return false
        return try {
            val mgr = getSystemService(BLUETOOTH_SERVICE) as BluetoothManager
            val adapter = mgr.adapter
            if (adapter == null || !adapter.isEnabled) throw Exception("bt_disabled")
            if (!hasBtConnectPermission()) throw Exception("missing_bt_permission")
            val dev = try {
                adapter.getRemoteDevice(macAddress)
            } catch (_: SecurityException) {
                throw Exception("bt_connect_permission_denied")
            }
            if (hasBtScanPermission()) {
                try { adapter.cancelDiscovery() } catch (_: SecurityException) {}
            }

            val sock = try {
                dev.createInsecureRfcommSocketToServiceRecord(UUID.fromString("00001101-0000-1000-8000-00805F9B34FB"))
            } catch (_: SecurityException) {
                throw Exception("bt_connect_permission_denied")
            }
            try {
                if (!hasBtConnectPermission()) throw Exception("missing_bt_permission")
                try {
                    sock.connect()
                } catch (_: SecurityException) {
                    throw Exception("bt_connect_permission_denied")
                }
                val name = if (hasBtConnectPermission()) {
                    try { dev.name } catch (_: SecurityException) { macAddress } catch (_: Throwable) { macAddress }
                } else macAddress
                Log.d("POS_PRINT", "Connected (raw) to printer: $name")

                val obj = try { JSONObject(content) } catch (_: Exception) { JSONObject() }
                val bmp = renderReceiptBitmap(obj, parseBranding(obj))

                val out = sock.outputStream
                val wBytes = bmp.width / 8
                val hDots = bmp.height
                val lineBuf = ByteArray(wBytes)

                // Header/init + vendor-specific raster start
                out.write(byteArrayOf(
                    0x1B, 0x40,             // ESC @ init
                    0x1B, 0x61, 0x00,       // ESC a 0 left
                    0x1F, 0x11, 0x02, 0x04  // vendor: start raster
                ))

                var row = 0
                while (row < hDots) {
                    val blockLines = kotlin.math.min(255, hDots - row)
                    // GS v 0 m=0 xL xH yL yH
                    out.write(byteArrayOf(
                        0x1D, 0x76, 0x30, 0x00,
                        (wBytes and 0xFF).toByte(), 0x00,
                        (blockLines and 0xFF).toByte(), (blockLines shr 8).toByte()
                    ))

                    repeat(blockLines) {
                        java.util.Arrays.fill(lineBuf, 0.toByte())
                        var byte = 0
                        var bit = 7
                        var col = 0
                        while (col < bmp.width) {
                            val px = bmp.getPixel(col, row)
                            val lum = Color.red(px) // grayscale
                            if (lum < 128) byte = byte or (1 shl bit)
                            if (bit == 0) {
                                lineBuf[col / 8] = byte.toByte()
                                byte = 0
                                bit = 7
                            } else {
                                bit--
                            }
                            col++
                        }
                        out.write(lineBuf)
                        row++
                    }
                }

                // Footer/commit — feed just enough for the last line to clear the head
                out.write(byteArrayOf(
                    0x1B, 0x64, 0x02,  // ESC d 2  feed 2 lines
                    0x1B, 0x64, 0x02,  // feed 2 more (total ~4)
                    0x1F, 0x11, 0x08,  // commit/print sequence
                    0x1F, 0x11, 0x0E,
                    0x1F, 0x11, 0x07,
                    0x1F, 0x11, 0x09
                ))
                out.flush()

                try { Thread.sleep(600) } catch (_: Throwable) {}
                try { out.close() } catch (_: Throwable) {}
                true
            } finally {
                try { sock.close() } catch (_: Throwable) {}
            }
        } catch (t: Throwable) {
            Log.e("POS_PRINT", "Phomemo print failed", t)
            try { Toast.makeText(this, "Drucken fehlgeschlagen: ${t.message}", Toast.LENGTH_SHORT).show() } catch (_: Throwable) {}
            false
        }
    }

    private class ReceiptCanvasBuilder(
        val width: Int,
        val padding: Int,
        val lineSpacing: Int,
        val qrSize: Int,
    ) {
        class Row(val height: Int, val draw: (Canvas, Float) -> Unit)

        val rows = ArrayList<Row>()

        fun gap(h: Int) { rows.add(Row(h) { _, _ -> }) }

        fun textRow(text: String, p: Paint, center: Boolean, indent: Float = 0f, ellipsizeFn: (String, Paint, Float) -> String) {
            val fm = Paint.FontMetrics(); p.getFontMetrics(fm)
            val maxW = (width - padding * 2).toFloat() - indent
            val display = ellipsizeFn(text, p, maxW)
            rows.add(Row((fm.bottom - fm.top).toInt() + lineSpacing) { c, y ->
                val x = if (center) ((width - p.measureText(display)) / 2f).coerceAtLeast(padding.toFloat())
                else padding.toFloat() + indent
                c.drawText(display, x, y - fm.top, p)
            })
        }

        fun twoColRow(left: String, right: String, p: Paint, ellipsizeFn: (String, Paint, Float) -> String, indent: Float = 0f) {
            val fm = Paint.FontMetrics(); p.getFontMetrics(fm)
            rows.add(Row((fm.bottom - fm.top).toInt() + lineSpacing) { c, y ->
                val rw = p.measureText(right)
                val l = ellipsizeFn(left, p, ((width - padding * 2).toFloat() - rw - 8f - indent).coerceAtLeast(0f))
                c.drawText(l, padding.toFloat() + indent, y - fm.top, p)
                c.drawText(right, width - padding - rw, y - fm.top, p)
            })
        }

        fun imageRow(bm: Bitmap) {
            rows.add(Row(bm.height + lineSpacing) { c, y ->
                c.drawBitmap(bm, ((width - bm.width) / 2f).coerceAtLeast(0f), y, null)
            })
        }

        fun qrRow(data: String) {
            val qrBmp = try {
                val hints = mutableMapOf<EncodeHintType, Any>(
                    EncodeHintType.ERROR_CORRECTION to ErrorCorrectionLevel.M,
                    EncodeHintType.MARGIN to 2,
                )
                // Byte-mode QR of the raw bytes: ISO-8859-1 maps each byte 1:1 and, being
                // the default QR charset, adds no ECI header, so the station reader gets
                // them back exactly. Everything else stays a text QR.
                val content = decodePickupTokenBytes(data)?.let {
                    hints[EncodeHintType.CHARACTER_SET] = "ISO-8859-1"
                    String(it, Charsets.ISO_8859_1)
                } ?: data
                val matrix = QRCodeWriter().encode(content, BarcodeFormat.QR_CODE, qrSize, qrSize, hints)
                Bitmap.createBitmap(qrSize, qrSize, Bitmap.Config.RGB_565).also { bm ->
                    for (x in 0 until qrSize) for (yy in 0 until qrSize) bm.setPixel(x, yy, if (matrix.get(x, yy)) Color.BLACK else Color.WHITE)
                }
            } catch (_: Throwable) { null }
            if (qrBmp != null) imageRow(qrBmp)
        }

        fun build(): Bitmap {
            val totalH = rows.sumOf { it.height }.coerceAtLeast(200)
            val bmp = Bitmap.createBitmap(width, totalH, Bitmap.Config.ARGB_8888)
            val canvas = Canvas(bmp)
            canvas.drawColor(Color.WHITE)
            var y = 0f
            for (r in rows) { r.draw(canvas, y); y += r.height }
            return bmp
        }
    }

    private fun renderReceiptBitmap(obj: JSONObject, branding: ReceiptBranding = ReceiptBranding(null, null, null, null)): Bitmap {
        if (receiptKind(obj) != "sale") return renderSlipBitmap(obj, branding)

        val width = 384
        val padding = 10
        val lineSpacing = 8
        val topPad = 12
        val bottomPad = 16
        val qrSize = 288
        val logoMaxHeight = 120

        // No anti-aliasing: thermal printers are 1-bit, and AA grey edges get thresholded
        // away, breaking up thin strokes. Aliased text prints solid like the QR.
        val titlePaint = Paint().apply { color = Color.BLACK; textSize = 34f; isFakeBoldText = true }
        val itemPaint = Paint().apply { color = Color.BLACK; textSize = 22f }
        val totalPaint = Paint().apply { color = Color.BLACK; textSize = 26f; isFakeBoldText = true }
        val small = Paint().apply { color = Color.BLACK; textSize = 18f }

        val currency = branding.currency ?: obj.optString("currency", "CHF")
        val totalCents = obj.optLong("totalCents", -1L)
        val method = obj.optString("method", "card")
        val cardLast4 = obj.optString("cardLast4", "").ifBlank { null }
        val orderId = obj.optString("orderId", "-")
        val dateStr = receiptDateTime(obj)
        val pickupQr = obj.optString("pickupQr", "")

        val logoBitmap: Bitmap? = if (!branding.logoUrl.isNullOrBlank())
            fetchLogoForReceipt(branding.logoUrl, cacheDir, maxWidth = width, maxHeight = logoMaxHeight) else null

        val b = ReceiptCanvasBuilder(width, padding, lineSpacing, qrSize)
        val ell = { t: String, p: Paint, w: Float -> ellipsize(t, p, w) }

        b.gap(topPad)
        b.textRow("Abhol-Quittung", titlePaint, center = true, ellipsizeFn = ell)
        b.gap(lineSpacing)

        for (it in parseReceiptLines(obj)) {
            val qtyTitle = "${it.quantity}x ${it.title}"
            if (it.unitPriceCents >= 0) b.twoColRow(qtyTitle, moneyStr(currency, it.unitPriceCents * it.quantity), itemPaint, ell)
            else b.textRow(qtyTitle, itemPaint, center = false, ellipsizeFn = ell)
            for ((slot, choice) in it.configuration) b.textRow("$slot: $choice", small, center = false, indent = 16f, ellipsizeFn = ell)
            if (it.depositCents > 0) b.twoColRow("Depot", moneyStr(currency, it.depositCents * it.quantity), small, ell, indent = 16f)
        }
        b.gap(lineSpacing)

        if (totalCents >= 0) b.twoColRow("Total", moneyStr(currency, totalCents), totalPaint, ell)
        if (method.equals("cash", ignoreCase = true)) {
            val rec = obj.optLong("amountReceivedCents", -1L)
            val chg = obj.optLong("changeCents", -1L)
            if (rec >= 0) b.textRow("Erhalten: ${moneyStr(currency, rec)}", small, center = false, ellipsizeFn = ell)
            if (chg >= 0) b.textRow("Rückgeld: ${moneyStr(currency, chg)}", small, center = false, ellipsizeFn = ell)
        }

        if (pickupQr.isNotEmpty()) {
            b.gap(lineSpacing * 2)
            b.qrRow(pickupQr)
            b.gap(lineSpacing)
        }

        b.gap(lineSpacing)
        b.textRow("Bestellung: $orderId", small, center = false, ellipsizeFn = ell)
        b.textRow("Zahlart: ${prettyPayMethod(method, cardLast4)}", small, center = false, ellipsizeFn = ell)
        if (dateStr.isNotEmpty()) b.textRow(dateStr, small, center = false, ellipsizeFn = ell)
        branding.displayName?.let { b.textRow(it, small, center = false, ellipsizeFn = ell) }
        (branding.address?.split("\n")?.map { it.trim() }?.filter { it.isNotBlank() } ?: emptyList())
            .forEach { b.textRow(it, small, center = false, ellipsizeFn = ell) }
        logoBitmap?.let { b.gap(lineSpacing); b.imageRow(it) }
        b.gap(bottomPad)

        return b.build()
    }

    private fun renderSlipBitmap(obj: JSONObject, branding: ReceiptBranding): Bitmap {
        val width = 384
        val padding = 10
        val lineSpacing = 8
        val topPad = 12
        val bottomPad = 4
        val qrSize = 288
        val logoMaxHeight = 120

        val titlePaint = Paint().apply { color = Color.BLACK; textSize = 34f; isFakeBoldText = true }
        val itemPaint = Paint().apply { color = Color.BLACK; textSize = 22f }
        val totalPaint = Paint().apply { color = Color.BLACK; textSize = 26f; isFakeBoldText = true }
        val small = Paint().apply { color = Color.BLACK; textSize = 18f }

        val kind = receiptKind(obj)
        val currency = branding.currency ?: obj.optString("currency", "CHF")
        val orderId = obj.optString("orderId", "-")
        val method = obj.optString("method", "")
        val cardLast4 = obj.optString("cardLast4", "").ifBlank { null }
        val totalCents = obj.optLong("totalCents", -1L)
        val refundCents = obj.optLong("refundCents", -1L)
        val refundAmount = if (refundCents >= 0) refundCents else totalCents
        val pickupQr = obj.optString("pickupQr", "")
        val dateStr = receiptDateTime(obj)

        val logoBitmap: Bitmap? = if (!branding.logoUrl.isNullOrBlank())
            fetchLogoForReceipt(branding.logoUrl, cacheDir, maxWidth = width, maxHeight = logoMaxHeight) else null

        val b = ReceiptCanvasBuilder(width, padding, lineSpacing, qrSize)
        val ell = { t: String, p: Paint, w: Float -> ellipsize(t, p, w) }

        b.gap(topPad)
        val titleText = when (kind) { "pickup" -> "Abhol-Quittung"; else -> "Rückerstattung" }
        b.textRow(titleText, titlePaint, center = true, ellipsizeFn = ell)
        b.gap(lineSpacing)

        for (it in parseReceiptLines(obj)) {
            val qtyTitle = "${it.quantity}x ${it.title}"
            if (it.unitPriceCents >= 0) b.twoColRow(qtyTitle, moneyStr(currency, it.unitPriceCents * it.quantity), itemPaint, ell)
            else b.textRow(qtyTitle, itemPaint, center = false, ellipsizeFn = ell)
            for ((slot, choice) in it.configuration) b.textRow("$slot: $choice", small, center = false, indent = 16f, ellipsizeFn = ell)
            if (it.depositCents > 0) b.twoColRow("Depot", moneyStr(currency, it.depositCents * it.quantity), small, ell, indent = 16f)
        }
        b.gap(lineSpacing)

        if (kind == "refund" && refundAmount >= 0) {
            b.twoColRow("Rückerstattung", "-${moneyStr(currency, refundAmount)}", totalPaint, ell)
        }

        if (kind == "pickup" && pickupQr.isNotEmpty()) {
            b.gap(lineSpacing * 2)
            b.qrRow(pickupQr)
            b.gap(lineSpacing)
        }

        b.gap(lineSpacing)
        b.textRow("Bestellung: $orderId", small, center = false, ellipsizeFn = ell)
        if (method.isNotBlank()) {
            val methodLine = if (kind == "refund") "Methode: ${prettyRefundMethod(method)}" else "Zahlart: ${prettyPayMethod(method, cardLast4)}"
            b.textRow(methodLine, small, center = false, ellipsizeFn = ell)
        }
        if (dateStr.isNotEmpty()) b.textRow(dateStr, small, center = false, ellipsizeFn = ell)
        branding.displayName?.let { b.textRow(it, small, center = false, ellipsizeFn = ell) }
        (branding.address?.split("\n")?.map { it.trim() }?.filter { it.isNotBlank() } ?: emptyList())
            .forEach { b.textRow(it, small, center = false, ellipsizeFn = ell) }
        logoBitmap?.let { b.gap(lineSpacing); b.imageRow(it) }
        b.gap(bottomPad)

        return b.build()
    }

    private fun ellipsize(text: String, paint: Paint, maxWidth: Float): String {
        if (paint.measureText(text) <= maxWidth) return text
        var s = text
        val ell = "…"
        var w = paint.measureText(s + ell)
        while (s.isNotEmpty() && w > maxWidth) {
            s = s.substring(0, s.length - 1)
            w = paint.measureText(s + ell)
        }
        return s + ell
    }
}
