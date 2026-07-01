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
import androidx.compose.material3.Button
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
    val recents = remember { DeviceConfig.recentHosts(ctx) }
    val hasOverride = remember { DeviceConfig.hostOverride(ctx) != null }

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
                        onDone(r.host != startHost)
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
                    onDone(TenantHostValidator.effectiveAllowedHost(null, BuildConfig.POS_URL) != startHost)
                },
                modifier = Modifier.fillMaxWidth(),
            ) { Text("Auf Standard zurücksetzen") }
        }

        Spacer(Modifier.height(24.dp))
        TextButton(onClick = { onDone(false) }, modifier = Modifier.fillMaxWidth()) { Text("Abbrechen") }
    }
}
