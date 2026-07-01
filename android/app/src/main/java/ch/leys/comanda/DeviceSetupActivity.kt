package ch.leys.comanda

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import ch.leys.comanda.ui.theme.ComandaTheme
import com.journeyapps.barcodescanner.ScanContract
import com.journeyapps.barcodescanner.ScanOptions
import com.sumup.merchant.reader.api.SumUpAPI
import com.sumup.merchant.reader.api.SumUpLogin
import com.sumup.merchant.reader.api.SumUpLoginContract

/**
 * PIN-gated operator surface for pointing this neutral APK at a tenant host at runtime.
 * Returns RESULT_OK with a `hostChanged` extra so WebPosActivity knows to wipe the old
 * tenant's session and reload.
 */
class DeviceSetupActivity : ComponentActivity() {

    companion object {
        const val EXTRA_HOST_CHANGED = "hostChanged"
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val startHost = DeviceConfig.effectiveAllowedHost(this)

        setContent {
            ComandaTheme {
                Surface(Modifier.fillMaxSize()) {
                    var unlocked by remember { mutableStateOf(false) }
                    if (unlocked) {
                        SetupForm(startHost) { changed -> finishWith(changed) }
                    } else {
                        PinGate { unlocked = true }
                    }
                }
            }
        }
    }

    private fun finishWith(hostChanged: Boolean) {
        setResult(RESULT_OK, Intent().putExtra(EXTRA_HOST_CHANGED, hostChanged))
        finish()
    }
}

@Composable
private fun PinGate(onUnlock: () -> Unit) {
    var pin by remember { mutableStateOf("") }
    var error by remember { mutableStateOf<String?>(null) }

    Column(
        Modifier
            .fillMaxSize()
            .imePadding()
            .padding(24.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text("Geräte-Einrichtung", style = MaterialTheme.typography.headlineSmall)
        Spacer(Modifier.height(16.dp))
        OutlinedTextField(
            value = pin,
            onValueChange = { pin = it.filter(Char::isDigit); error = null },
            label = { Text("PIN") },
            singleLine = true,
            visualTransformation = PasswordVisualTransformation(),
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.NumberPassword),
            isError = error != null,
            modifier = Modifier.fillMaxWidth(),
        )
        if (error != null) {
            Spacer(Modifier.height(8.dp))
            Text(error!!, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
        }
        Spacer(Modifier.height(16.dp))
        Button(
            onClick = { if (pin == BuildConfig.SETUP_PIN) onUnlock() else { error = "Falsche PIN."; pin = "" } },
            modifier = Modifier.fillMaxWidth(),
        ) { Text("Weiter") }
    }
}

@Composable
private fun SetupForm(startHost: String?, onDone: (Boolean) -> Unit) {
    val ctx = LocalContext.current
    var input by remember { mutableStateOf("") }
    var error by remember { mutableStateOf<String?>(null) }
    var pendingLogout by remember { mutableStateOf(false) }
    val recents = remember { DeviceConfig.recentHosts(ctx) }
    val hasOverride = remember { DeviceConfig.hostOverride(ctx) != null }

    fun finishAfterHostChange(changed: Boolean) {
        if (changed && SumUpAPI.isLoggedIn()) pendingLogout = true else onDone(changed)
    }

    val scanLauncher = rememberLauncherForActivityResult(ScanContract()) { result ->
        result.contents?.let { input = it; error = null }
    }
    fun launchScan() {
        scanLauncher.launch(
            ScanOptions().apply {
                setPrompt("Tenant-QR scannen")
                setBeepEnabled(true)
                setOrientationLocked(false)
                setDesiredBarcodeFormats(ScanOptions.QR_CODE)
                captureActivity = CustomCaptureActivity::class.java
            }
        )
    }
    val cameraPerm = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        if (granted) launchScan()
    }

    Column(
        Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .imePadding()
            .padding(24.dp),
    ) {
        Text("Geräte-Einrichtung", style = MaterialTheme.typography.headlineSmall)
        Spacer(Modifier.height(4.dp))
        Text(
            "Aktiver Host: ${startHost ?: "—"}${if (!hasOverride) " (Standard)" else ""}",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.height(20.dp))

        OutlinedTextField(
            value = input,
            onValueChange = { input = it; error = null },
            label = { Text("Tenant-Host") },
            placeholder = { Text("kasse.comanda.leys.ch") },
            singleLine = true,
            isError = error != null,
            modifier = Modifier.fillMaxWidth(),
        )
        if (error != null) {
            Spacer(Modifier.height(6.dp))
            Text(error!!, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
        }

        Spacer(Modifier.height(12.dp))
        OutlinedButton(
            onClick = {
                if (ContextCompat.checkSelfPermission(ctx, Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED)
                    launchScan()
                else
                    cameraPerm.launch(Manifest.permission.CAMERA)
            },
            modifier = Modifier.fillMaxWidth(),
        ) { Text("QR-Code scannen") }

        if (recents.isNotEmpty()) {
            Spacer(Modifier.height(20.dp))
            Text("Zuletzt verwendet", style = MaterialTheme.typography.labelMedium)
            recents.forEach { h ->
                TextButton(onClick = { input = h; error = null }, modifier = Modifier.fillMaxWidth()) {
                    Text(h, modifier = Modifier.fillMaxWidth())
                }
            }
        }

        Spacer(Modifier.height(24.dp))
        Button(
            onClick = {
                when (val r = TenantHostValidator.validate(input)) {
                    is TenantHostValidator.Result.Valid -> {
                        DeviceConfig.setHostOverride(ctx, r.host)
                        finishAfterHostChange(r.host != startHost)
                    }
                    is TenantHostValidator.Result.Invalid -> error = r.reason
                }
            },
            modifier = Modifier.fillMaxWidth(),
        ) { Text("Speichern & neu laden") }

        if (hasOverride) {
            Spacer(Modifier.height(8.dp))
            OutlinedButton(
                onClick = {
                    DeviceConfig.clearHostOverride(ctx)
                    finishAfterHostChange(TenantHostValidator.effectiveAllowedHost(null, BuildConfig.POS_URL) != startHost)
                },
                modifier = Modifier.fillMaxWidth(),
            ) { Text("Auf Standard zurücksetzen") }
        }

        Spacer(Modifier.height(28.dp))
        HorizontalDivider()
        Spacer(Modifier.height(20.dp))
        SumUpAccountSection()

        Spacer(Modifier.height(24.dp))
        TextButton(onClick = { onDone(false) }, modifier = Modifier.fillMaxWidth()) { Text("Abbrechen") }
    }

    if (pendingLogout) {
        AlertDialog(
            onDismissRequest = { pendingLogout = false; onDone(true) },
            title = { Text("Tenant gewechselt") },
            text = { Text("Beim vorherigen SumUp-Konto abmelden? Für den neuen Tenant anschliessend am richtigen Konto anmelden.") },
            confirmButton = {
                TextButton(onClick = {
                    try { SumUpAPI.logout() } catch (_: Throwable) {}
                    pendingLogout = false
                    onDone(true)
                }) { Text("Abmelden") }
            },
            dismissButton = {
                TextButton(onClick = { pendingLogout = false; onDone(true) }) { Text("Angemeldet bleiben") }
            },
        )
    }
}

@Composable
private fun SumUpAccountSection() {
    val ctx = LocalContext.current
    val expected = remember { DeviceConfig.expectedSumUpMerchant(ctx) }
    val tenantName = remember { DeviceConfig.tenantDisplayName(ctx) }
    val keyConfigured = BuildConfig.SUMUP_AFFILIATE_KEY.isNotBlank()

    var loggedIn by remember { mutableStateOf(SumUpAPI.isLoggedIn()) }
    var current by remember { mutableStateOf(readCurrentMerchant()) }
    fun refresh() {
        loggedIn = SumUpAPI.isLoggedIn()
        current = readCurrentMerchant()
    }

    val loginLauncher = rememberLauncherForActivityResult(SumUpLoginContract()) { refresh() }

    val status = SumUpGuardrail.evaluate(expected, loggedIn, current)

    Text("SumUp-Konto", style = MaterialTheme.typography.titleMedium)
    Spacer(Modifier.height(4.dp))
    Text(
        "Erwartet: ${expected ?: "—"}${if (tenantName != null) " ($tenantName)" else ""}",
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    Spacer(Modifier.height(12.dp))

    val (message, isError) = statusCopy(status, tenantName)
    Text(
        message,
        style = MaterialTheme.typography.bodyMedium,
        color = if (isError) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurface,
    )

    Spacer(Modifier.height(16.dp))
    Button(
        onClick = { loginLauncher.launch(SumUpLogin.builder(BuildConfig.SUMUP_AFFILIATE_KEY).build()) },
        enabled = keyConfigured,
        modifier = Modifier.fillMaxWidth(),
    ) { Text(if (loggedIn) "SumUp-Konto wechseln" else "SumUp-Konto anmelden") }

    if (loggedIn) {
        Spacer(Modifier.height(8.dp))
        OutlinedButton(
            onClick = { try { SumUpAPI.logout() } catch (_: Throwable) {}; refresh() },
            modifier = Modifier.fillMaxWidth(),
        ) { Text("Abmelden") }
    }

    if (!keyConfigured) {
        Spacer(Modifier.height(8.dp))
        Text(
            "SumUp ist in diesem Build nicht konfiguriert.",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.error,
        )
    }
}

private fun readCurrentMerchant(): String? =
    try { SumUpAPI.getCurrentMerchant()?.merchantCode } catch (_: Throwable) { null }

private fun statusCopy(status: SumUpGuardrail.Status, tenantName: String?): Pair<String, Boolean> = when (status) {
    is SumUpGuardrail.Status.Match ->
        "Angemeldet als ${status.merchant} — stimmt mit ${tenantName ?: "dem Tenant"} überein." to false
    is SumUpGuardrail.Status.Mismatch ->
        "Falsches Konto: angemeldet als ${status.current}, erwartet ${status.expected}. Kartenzahlung ist gesperrt, bis das richtige Konto angemeldet ist." to true
    is SumUpGuardrail.Status.LoggedOut ->
        "Nicht bei SumUp angemeldet." to false
    is SumUpGuardrail.Status.Unverifiable ->
        "Angemeldet, aber das aktive Konto konnte nicht gelesen werden. Erwartet: ${status.expected}. Bitte manuell prüfen." to true
    is SumUpGuardrail.Status.NotConfigured ->
        (if (status.loggedIn) "Angemeldet als ${status.currentMerchant ?: "unbekannt"}. " else "Nicht angemeldet. ") +
            "Kein erwartetes SumUp-Konto für diesen Tenant hinterlegt." to false
}
