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
import com.google.zxing.common.BitMatrix
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
import java.util.UUID
import org.json.JSONArray
import android.bluetooth.BluetoothAdapter
import android.content.Context

/** Branding supplied by the web POS in the print payload. All fields optional/nullable. */
private data class ReceiptBranding(
    val displayName: String?,
    val logoUrl: String?,
    val address: String?,
    val currency: String?,
    val thankYou: String?
)

private fun parseBranding(obj: JSONObject): ReceiptBranding {
    val b = obj.optJSONObject("branding")
    return if (b != null) ReceiptBranding(
        displayName = b.optString("displayName").ifBlank { null },
        logoUrl     = b.optString("logoUrl").ifBlank { null },
        address     = b.optString("address").ifBlank { null },
        currency    = b.optString("currency").ifBlank { null },
        thankYou    = b.optString("thankYou").ifBlank { null }
    ) else ReceiptBranding(null, null, null, null, null)
}

/** One refunded/redeemable line on a slip. */
private data class ReceiptLine(
    val title: String,
    val quantity: Int,
    val unitPriceCents: Long,
    val configuration: List<Pair<String, String>>
)

/** Receipt kind sent by the web POS; absent or unknown ⇒ "sale". */
private fun receiptKind(obj: JSONObject): String =
    when (obj.optString("kind", "sale").lowercase()) {
        "refund" -> "refund"
        "pickup" -> "pickup"
        "void" -> "void"
        else -> "sale"
    }

private fun prettyRefundMethod(method: String): String = when (method.lowercase()) {
    "twint" -> "TWINT"
    "cash-out", "cashout" -> "Cash-out"
    "cash" -> "Bar"
    "card" -> "Karte"
    else -> method
}

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
        val reference: String
    )

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        allowedHost = DeviceConfig.effectiveAllowedHost(this)
        webView = WebView(this)
        configureWebView(webView)
        setContentView(webView)

        // Bridge for PosBridge.* calls from /pos page
        webView.addJavascriptInterface(PosBridge(), "PosBridge")

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
                // Allow in-app only the configured host; open others externally
                return if (host != null && host == allowedHost) {
                    false
                } else {
                    startActivity(Intent(Intent.ACTION_VIEW, url))
                    true
                }
            }
        }
        wv.webChromeClient = object : WebChromeClient() {
            // The web station scanner (html5-qrcode) calls getUserMedia; grant the camera so it runs in-app.
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
                val pending = PendingPayment(amountCents, currency, ref)
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
    }

    private fun startSumUpCheckout(p: PendingPayment) {
        pendingPayment = p
        val payment = try {
            SumUpPayment.builder()
                .total(BigDecimal(p.amountCents).divide(BigDecimal(100)))
                .currency(SumUpPayment.Currency.valueOf(p.currency))
                .title("POS ${p.reference}")
                .foreignTransactionId(p.reference)
                .skipSuccessScreen()
                .build()
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
            // Render a simple receipt from JSON content
            val obj = try { JSONObject(content) } catch (_: Exception) { JSONObject() }
            val branding = parseBranding(obj)
            val totalCents = obj.optLong("totalCents", -1)
            val orderId = obj.optString("orderId", "-")
            val method = obj.optString("method", "card")
            val pickupQr = obj.optString("pickupQr", "")
            val currency = branding.currency ?: obj.optString("currency", "CHF")
            val orderTs = obj.optLong("orderTimestamp", -1L)
            val dateStr = try {
                val whenMs = if (orderTs > 0) orderTs else System.currentTimeMillis()
                val fmt = java.text.SimpleDateFormat("dd.MM.yyyy HH:mm", java.util.Locale.GERMANY)
                fmt.format(java.util.Date(whenMs))
            } catch (_: Throwable) { "" }
            val kind = receiptKind(obj)
            val depositCents = obj.optLong("depositCents", -1L)
            val refundCents = obj.optLong("refundCents", -1L)
            val lines = StringBuilder()
            val headerName = branding.displayName ?: "Comanda"
            lines.append("[C]<b>$headerName</b>\n")
            // Omit address entirely when not provided — no fallback to a hardcoded address
            if (!branding.address.isNullOrBlank()) {
                branding.address.split("\n").forEach { line ->
                    if (line.isNotBlank()) lines.append("[C]${line.trim()}\n")
                }
            }
            if (dateStr.isNotEmpty()) lines.append("[C]$dateStr\n")

            val appendQr: (String) -> Unit = { data ->
                try {
                    val hints = mapOf(
                        EncodeHintType.ERROR_CORRECTION to ErrorCorrectionLevel.M,
                        EncodeHintType.MARGIN to 2
                    )
                    val qrBitmapSize = 320
                    val matrix = QRCodeWriter().encode(data, BarcodeFormat.QR_CODE, qrBitmapSize, qrBitmapSize, hints)
                    val qrBmp = Bitmap.createBitmap(qrBitmapSize, qrBitmapSize, Bitmap.Config.RGB_565)
                    for (x in 0 until qrBitmapSize) {
                        for (yy in 0 until qrBitmapSize) {
                            qrBmp.setPixel(x, yy, if (matrix.get(x, yy)) Color.BLACK else Color.WHITE)
                        }
                    }
                    val imgHex = com.dantsu.escposprinter.textparser.PrinterTextParserImg.bitmapToHexadecimalString(printer, qrBmp)
                    lines.append("[C]<img>$imgHex</img>\n")
                } catch (_: Throwable) {}
            }
            val appendItems: () -> Unit = {
                for (it in parseReceiptLines(obj)) {
                    val qtyTitle = "${it.quantity}x ${it.title}"
                    if (it.unitPriceCents >= 0) {
                        lines.append("[L]$qtyTitle[R]${moneyStr(currency, it.unitPriceCents * it.quantity)}\n")
                    } else {
                        lines.append("[L]$qtyTitle\n")
                    }
                    for ((slot, choice) in it.configuration) lines.append("[L]  - $slot: $choice\n")
                }
            }

            when (kind) {
                "refund" -> {
                    lines.append("[C]<b>Rückerstattung</b>\n")
                    appendItems()
                    val amt = if (refundCents >= 0) refundCents else totalCents
                    if (amt >= 0) lines.append("[L]Rückerstattung[R]-${moneyStr(currency, amt)}\n")
                    lines.append("Methode: ${prettyRefundMethod(method)}\n")
                    lines.append("Bestellung: ${orderId}\n")
                }
                "pickup" -> {
                    lines.append("[C]<b>Abhol-Quittung</b>\n")
                    appendItems()
                    lines.append("Bestellung: ${orderId}\n")
                    lines.append("\n")
                    if (pickupQr.isNotEmpty()) appendQr(pickupQr)
                    lines.append("[C]Alte Quittung ungültig\n")
                }
                "void" -> {
                    lines.append("[C]<b>Storniert</b>\n")
                    val amt = if (refundCents >= 0) refundCents else totalCents
                    if (amt >= 0) lines.append("[L]Rückerstattung[R]-${moneyStr(currency, amt)}\n")
                    if (method.isNotBlank()) lines.append("Methode: ${prettyRefundMethod(method)}\n")
                    lines.append("Bestellung: ${orderId}\n")
                }
                else -> {
                    if (totalCents >= 0) lines.append("Total: ${moneyStr(currency, totalCents)}\n")
                    if (depositCents > 0) lines.append("davon Depot: ${moneyStr(currency, depositCents)}\n")
                    val methodPretty = if (method.isNotEmpty()) method.substring(0, 1).uppercase() + method.substring(1) else method
                    lines.append("Zahlart: ${methodPretty}\n")
                    if (method.equals("cash", ignoreCase = true)) {
                        val rec = obj.optLong("amountReceivedCents", -1L)
                        val chg = obj.optLong("changeCents", -1L)
                        if (rec >= 0) lines.append("Erhalten: ${moneyStr(currency, rec)}\n")
                        if (chg >= 0) lines.append("Rückgeld: ${moneyStr(currency, chg)}\n")
                    }
                    lines.append("Bestellung: ${orderId}\n")
                    lines.append("\n")
                    if (pickupQr.isNotEmpty()) appendQr(pickupQr)
                }
            }
            val thankYou = branding.thankYou ?: "Danke!"
            lines.append("[C]$thankYou\n\n")
            printer.printFormattedText(lines.toString())
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

                // Footer/commit — add a bit more feed so the final line isn't clipped
                out.write(byteArrayOf(
                    0x1B, 0x64, 0x02,  // ESC d 2  feed 2 lines
                    0x1B, 0x64, 0x02,  // feed 2 more
                    0x1B, 0x64, 0x02,  // feed 2 more (total ~6)
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

    private fun renderReceiptBitmap(obj: JSONObject, branding: ReceiptBranding = ReceiptBranding(null, null, null, null, null)): Bitmap {
        if (receiptKind(obj) != "sale") return renderSlipBitmap(obj, branding)
        val width = 384
        val padding = 10
        val headerSize = 24f
        val fontSize = 20f
        val smallSize = 18f
        val lineSpacing = 8
        val headerTopPad = 12
        val headerBottomPad = 6
        val bottomPad = 56
        val qrSize = 320
        val logoMaxHeight = 160

        val paint = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = Color.BLACK; textSize = fontSize }
        val bold = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = Color.BLACK; textSize = headerSize; isFakeBoldText = true }
        val small = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = Color.BLACK; textSize = smallSize }

        // Attempt logo fetch (blocking; printing already runs off the main thread via Bluetooth I/O).
        // Falls back to null silently — the text header path below handles that.
        val logoBitmap: Bitmap? = if (!branding.logoUrl.isNullOrBlank()) {
            fetchLogoForReceipt(branding.logoUrl, cacheDir, maxWidth = width, maxHeight = logoMaxHeight)
        } else null

        val addressLines: List<String> = branding.address
            ?.split("\n")
            ?.map { it.trim() }
            ?.filter { it.isNotBlank() }
            ?: emptyList()

        fun measureHeight(): Int {
            var h = 0
            val fmHeader = Paint.FontMetrics(); bold.getFontMetrics(fmHeader)
            val fm = Paint.FontMetrics(); paint.getFontMetrics(fm)
            val fmSmall = Paint.FontMetrics(); small.getFontMetrics(fmSmall)
            val lineH = (fmSmall.bottom - fmSmall.top).toInt()
            // Header: logo or text
            h += headerTopPad
            if (logoBitmap != null) {
                h += logoBitmap.height + headerBottomPad
            } else {
                h += (fmHeader.bottom - fmHeader.top).toInt() + headerBottomPad
            }
            // Address lines (omitted when absent)
            h += addressLines.size * (lineH + lineSpacing)
            // Date/time
            h += lineH + lineSpacing
            // QR (optional)
            val hasQr = obj.optString("pickupQr", "").isNotEmpty()
            if (hasQr) h += qrSize + lineSpacing
            // Total
            h += (fm.bottom - fm.top).toInt() + lineSpacing
            // Deposit line (optional, small)
            if (obj.optLong("depositCents", -1L) > 0) h += lineH + lineSpacing
            // Payment line
            h += (fm.bottom - fm.top).toInt() + lineSpacing
            // Cash: received + change (up to two small lines)
            val methodForHeight = obj.optString("method", "card")
            if (methodForHeight.equals("cash", ignoreCase = true)) {
                val rec = obj.optLong("amountReceivedCents", -1L)
                val chg = obj.optLong("changeCents", -1L)
                if (rec >= 0) h += lineH + lineSpacing
                if (chg >= 0) h += lineH + lineSpacing
            }
            // Order ID (small)
            h += lineH + lineSpacing
            // Whitespace divider
            h += (fm.bottom - fm.top).toInt() / 2 + lineSpacing
            // Thank-you line + bottom pad
            h += (fm.bottom - fm.top).toInt() + bottomPad
            return h.coerceAtLeast(200)
        }

        val bmp = Bitmap.createBitmap(width, measureHeight(), Bitmap.Config.ARGB_8888)
        val canvas = Canvas(bmp)
        canvas.drawColor(Color.WHITE)

        var y: Float = headerTopPad.toFloat()

        // Header: logo (if fetched successfully) or displayName/fallback text
        if (logoBitmap != null) {
            val left = ((width - logoBitmap.width) / 2f).coerceAtLeast(0f)
            canvas.drawBitmap(logoBitmap, left, y, null)
            y += logoBitmap.height + headerBottomPad
        } else {
            val fm = Paint.FontMetrics(); bold.getFontMetrics(fm)
            y -= fm.top
            val title = branding.displayName ?: "Comanda"
            val tw = bold.measureText(title)
            canvas.drawText(title, (width - tw) / 2f, y, bold)
            y += fm.bottom + headerBottomPad
        }

        // Address lines (centered, small). Omitted entirely when not provided.
        if (addressLines.isNotEmpty()) {
            val fmSmall = Paint.FontMetrics(); small.getFontMetrics(fmSmall)
            for (line in addressLines) {
                val aw = small.measureText(line)
                canvas.drawText(line, (width - aw) / 2f, y - fmSmall.top, small)
                y += (fmSmall.bottom - fmSmall.top) + lineSpacing
            }
        }

        // Date/time of order (centered, small). Prefer orderTimestamp if provided.
        run {
            val fmSmall = Paint.FontMetrics(); small.getFontMetrics(fmSmall)
            val orderTs = obj.optLong("orderTimestamp", -1L)
            val dateStr = try {
                val whenMs = if (orderTs > 0) orderTs else System.currentTimeMillis()
                val fmt = java.text.SimpleDateFormat("dd.MM.yyyy HH:mm", java.util.Locale.GERMANY)
                fmt.format(java.util.Date(whenMs))
            } catch (_: Throwable) { "" }
            if (dateStr.isNotEmpty()) {
                val dw = small.measureText(dateStr)
                canvas.drawText(dateStr, (width - dw) / 2f, y - fmSmall.top, small)
                y += (fmSmall.bottom - fmSmall.top) + lineSpacing
            }
        }

        // QR code if provided
        val pickupQr = obj.optString("pickupQr", "")
        if (pickupQr.isNotEmpty()) {
            val size = qrSize
            try {
                val hints = mapOf(
                    EncodeHintType.ERROR_CORRECTION to ErrorCorrectionLevel.M,
                    EncodeHintType.MARGIN to 2
                )
                val matrix: BitMatrix = QRCodeWriter().encode(pickupQr, BarcodeFormat.QR_CODE, size, size, hints)
                val qrBmp = Bitmap.createBitmap(size, size, Bitmap.Config.RGB_565)
                for (x in 0 until size) {
                    for (yy in 0 until size) {
                        qrBmp.setPixel(x, yy, if (matrix.get(x, yy)) Color.BLACK else Color.WHITE)
                    }
                }
                val left = (width - size) / 2f
                canvas.drawBitmap(qrBmp, left, y, null)
                y += size + lineSpacing
            } catch (t: Throwable) {
                // ignore QR failures, continue text
            }
        }

        y += lineSpacing

        val totalCents = obj.optLong("totalCents", -1)
        val currency = branding.currency ?: obj.optString("currency", "CHF")
        val method = obj.optString("method", "card")
        val orderId = obj.optString("orderId", "-")

        // Total
        if (totalCents >= 0) {
            val fm = Paint.FontMetrics(); paint.getFontMetrics(fm)
            val t = String.format("Total: %s %.2f", currency, totalCents / 100.0)
            canvas.drawText(t, padding.toFloat(), y - fm.top, paint)
            y += (fm.bottom - fm.top) + lineSpacing
        }

        // Deposit line (optional)
        run {
            val depositCents = obj.optLong("depositCents", -1L)
            if (depositCents > 0) {
                val fmSmall = Paint.FontMetrics(); small.getFontMetrics(fmSmall)
                val d = "davon Depot: ${moneyStr(currency, depositCents)}"
                canvas.drawText(ellipsize(d, small, (width - padding * 2).toFloat()), padding.toFloat(), y - fmSmall.top, small)
                y += (fmSmall.bottom - fmSmall.top) + lineSpacing
            }
        }

        // Payment line with capitalized method
        run {
            val fm = Paint.FontMetrics(); paint.getFontMetrics(fm)
            val methodPretty = if (method.isNotEmpty()) method.substring(0, 1).uppercase() + method.substring(1) else method
            val payLine = "Zahlart: $methodPretty"
            val payText = ellipsize(payLine, paint, (width - padding * 2).toFloat())
            canvas.drawText(payText, padding.toFloat(), y - fm.top, paint)
            y += (fm.bottom - fm.top) + lineSpacing
        }

        // Cash details
        if (method.equals("cash", ignoreCase = true)) {
            val fmSmall = Paint.FontMetrics(); small.getFontMetrics(fmSmall)
            val rec = obj.optLong("amountReceivedCents", -1L)
            val chg = obj.optLong("changeCents", -1L)
            if (rec >= 0) {
                val recStr = String.format("Erhalten: %s %.2f", currency, rec / 100.0)
                canvas.drawText(ellipsize(recStr, small, (width - padding * 2).toFloat()), padding.toFloat(), y - fmSmall.top, small)
                y += (fmSmall.bottom - fmSmall.top) + lineSpacing
            }
            if (chg >= 0) {
                val chgStr = String.format("Rückgeld: %s %.2f", currency, chg / 100.0)
                canvas.drawText(ellipsize(chgStr, small, (width - padding * 2).toFloat()), padding.toFloat(), y - fmSmall.top, small)
                y += (fmSmall.bottom - fmSmall.top) + lineSpacing
            }
        }

        // Order ID under method
        run {
            val fmSmall = Paint.FontMetrics(); small.getFontMetrics(fmSmall)
            val ordLine = ellipsize("Bestellung: $orderId", small, (width - padding * 2).toFloat())
            canvas.drawText(ordLine, padding.toFloat(), y - fmSmall.top, small)
            y += (fmSmall.bottom - fmSmall.top) + lineSpacing
        }
        // Whitespace divider before thank you
        run {
            val fm = Paint.FontMetrics(); paint.getFontMetrics(fm)
            y += (fm.bottom - fm.top) / 2 + lineSpacing
        }

        // Thank-you line (with extra bottom padding to avoid clipping)
        run {
            val fm = Paint.FontMetrics(); paint.getFontMetrics(fm)
            val thankYou = branding.thankYou ?: "Danke!"
            canvas.drawText(thankYou, padding.toFloat(), y - fm.top, paint)
            y += bottomPad
        }

        return bmp
    }

    // Raster renderer for non-sale slips (refund / pickup re-issue / void) on Phomemo-class printers.
    private fun renderSlipBitmap(obj: JSONObject, branding: ReceiptBranding): Bitmap {
        val width = 384
        val padding = 10
        val headerSize = 24f
        val titleSize = 22f
        val fontSize = 20f
        val smallSize = 18f
        val lineSpacing = 8
        val headerTopPad = 12
        val headerBottomPad = 6
        val bottomPad = 56
        val qrSize = 320
        val logoMaxHeight = 160

        val paint = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = Color.BLACK; textSize = fontSize }
        val bold = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = Color.BLACK; textSize = headerSize; isFakeBoldText = true }
        val titlePaint = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = Color.BLACK; textSize = titleSize; isFakeBoldText = true }
        val small = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = Color.BLACK; textSize = smallSize }

        val kind = receiptKind(obj)
        val currency = branding.currency ?: obj.optString("currency", "CHF")
        val orderId = obj.optString("orderId", "-")
        val method = obj.optString("method", "")
        val totalCents = obj.optLong("totalCents", -1L)
        val refundCents = obj.optLong("refundCents", -1L)
        val refundAmount = if (refundCents >= 0) refundCents else totalCents
        val pickupQr = obj.optString("pickupQr", "")
        val orderTs = obj.optLong("orderTimestamp", -1L)
        val dateStr = try {
            val whenMs = if (orderTs > 0) orderTs else System.currentTimeMillis()
            java.text.SimpleDateFormat("dd.MM.yyyy HH:mm", java.util.Locale.GERMANY).format(java.util.Date(whenMs))
        } catch (_: Throwable) { "" }

        val logoBitmap: Bitmap? = if (!branding.logoUrl.isNullOrBlank())
            fetchLogoForReceipt(branding.logoUrl, cacheDir, maxWidth = width, maxHeight = logoMaxHeight) else null

        class Row(val height: Int, val draw: (Canvas, Float) -> Unit)
        val rows = ArrayList<Row>()
        fun gap(h: Int) { rows.add(Row(h) { _, _ -> }) }
        fun textRow(text: String, p: Paint, center: Boolean) {
            val fm = Paint.FontMetrics(); p.getFontMetrics(fm)
            val display = ellipsize(text, p, (width - padding * 2).toFloat())
            rows.add(Row((fm.bottom - fm.top).toInt() + lineSpacing) { c, y ->
                val x = if (center) ((width - p.measureText(display)) / 2f).coerceAtLeast(padding.toFloat()) else padding.toFloat()
                c.drawText(display, x, y - fm.top, p)
            })
        }
        fun twoColRow(left: String, right: String, p: Paint) {
            val fm = Paint.FontMetrics(); p.getFontMetrics(fm)
            rows.add(Row((fm.bottom - fm.top).toInt() + lineSpacing) { c, y ->
                val rw = p.measureText(right)
                val l = ellipsize(left, p, ((width - padding * 2).toFloat() - rw - 8f).coerceAtLeast(0f))
                c.drawText(l, padding.toFloat(), y - fm.top, p)
                c.drawText(right, width - padding - rw, y - fm.top, p)
            })
        }
        fun qrRow(data: String) {
            val qrBmp = try {
                val hints = mapOf(EncodeHintType.ERROR_CORRECTION to ErrorCorrectionLevel.M, EncodeHintType.MARGIN to 2)
                val matrix = QRCodeWriter().encode(data, BarcodeFormat.QR_CODE, qrSize, qrSize, hints)
                Bitmap.createBitmap(qrSize, qrSize, Bitmap.Config.RGB_565).also { bm ->
                    for (x in 0 until qrSize) for (yy in 0 until qrSize) bm.setPixel(x, yy, if (matrix.get(x, yy)) Color.BLACK else Color.WHITE)
                }
            } catch (_: Throwable) { null }
            if (qrBmp != null) rows.add(Row(qrSize + lineSpacing) { c, y -> c.drawBitmap(qrBmp, (width - qrSize) / 2f, y, null) })
        }

        gap(headerTopPad)
        if (logoBitmap != null) {
            rows.add(Row(logoBitmap.height + headerBottomPad) { c, y ->
                c.drawBitmap(logoBitmap, ((width - logoBitmap.width) / 2f).coerceAtLeast(0f), y, null)
            })
        } else {
            textRow(branding.displayName ?: "Comanda", bold, center = true)
            gap(headerBottomPad)
        }
        (branding.address?.split("\n")?.map { it.trim() }?.filter { it.isNotBlank() } ?: emptyList())
            .forEach { textRow(it, small, center = true) }
        if (dateStr.isNotEmpty()) textRow(dateStr, small, center = true)

        val titleText = when (kind) { "refund" -> "Rückerstattung"; "pickup" -> "Abhol-Quittung"; else -> "Storniert" }
        textRow(titleText, titlePaint, center = true)
        gap(lineSpacing)

        if (kind == "refund" || kind == "pickup") {
            for (it in parseReceiptLines(obj)) {
                val qtyTitle = "${it.quantity}x ${it.title}"
                if (it.unitPriceCents >= 0) twoColRow(qtyTitle, moneyStr(currency, it.unitPriceCents * it.quantity), paint)
                else textRow(qtyTitle, paint, center = false)
                for ((slot, choice) in it.configuration) textRow("  - $slot: $choice", small, center = false)
            }
        }

        if (kind == "refund" || kind == "void") {
            if (refundAmount >= 0) twoColRow("Rückerstattung", "-${moneyStr(currency, refundAmount)}", paint)
            if (method.isNotBlank()) textRow("Methode: ${prettyRefundMethod(method)}", small, center = false)
        }

        textRow("Bestellung: $orderId", small, center = false)

        if (kind == "pickup" && pickupQr.isNotEmpty()) {
            gap(lineSpacing)
            qrRow(pickupQr)
            textRow("Alte Quittung ungültig", small, center = true)
        }
        gap(bottomPad)

        val totalH = rows.sumOf { it.height }.coerceAtLeast(200)
        val bmp = Bitmap.createBitmap(width, totalH, Bitmap.Config.ARGB_8888)
        val canvas = Canvas(bmp)
        canvas.drawColor(Color.WHITE)
        var y = 0f
        for (r in rows) { r.draw(canvas, y); y += r.height }
        return bmp
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
