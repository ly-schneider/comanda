package ch.leys.comanda

import android.os.Handler
import android.os.Looper
import android.util.Log
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL

private const val TAG = "SetupPinVerifier"

/**
 * Checks the Device Setup PIN against the platform API.
 *
 * No offline fallback: Device Setup already requires connectivity because SumUp
 * login does, so there is no local copy of the PIN to leak or keep in sync.
 */
object SetupPinVerifier {

    sealed interface Result {
        data object Ok : Result
        data class Rejected(val message: String) : Result
        data class Unreachable(val message: String) : Result
    }

    fun verify(pin: String, onResult: (Result) -> Unit) {
        val base = BuildConfig.API_BASE_URL.trimEnd('/')
        if (base.isEmpty()) {
            post(onResult, Result.Unreachable("Dieser Build hat keine API-Adresse konfiguriert."))
            return
        }

        Thread {
            val result = try {
                request(base, pin)
            } catch (t: Throwable) {
                Log.w(TAG, "Setup PIN check failed", t)
                Result.Unreachable("Keine Verbindung zum Server. Die Geräte-Einrichtung braucht Internet.")
            }
            post(onResult, result)
        }.start()
    }

    private fun request(base: String, pin: String): Result {
        val conn = URL("$base/v1/android/setup/verify-pin").openConnection() as HttpURLConnection
        conn.connectTimeout = 10_000
        conn.readTimeout = 10_000
        conn.requestMethod = "POST"
        conn.doOutput = true
        conn.setRequestProperty("Content-Type", "application/json")
        conn.setRequestProperty("Accept", "application/json")

        try {
            conn.outputStream.use { it.write(JSONObject().put("pin", pin).toString().toByteArray()) }

            return when (conn.responseCode) {
                200 -> Result.Ok
                401 -> Result.Rejected("Falsche PIN.")
                429 -> Result.Rejected("Zu viele Versuche. Bitte später erneut versuchen.")
                501 -> Result.Unreachable("Auf dem Server ist keine Setup-PIN hinterlegt.")
                else -> Result.Unreachable("Server-Fehler (${conn.responseCode}).")
            }
        } finally {
            conn.disconnect()
        }
    }

    private fun post(onResult: (Result) -> Unit, result: Result) {
        Handler(Looper.getMainLooper()).post { onResult(result) }
    }
}
