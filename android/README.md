# Comanda Android App

Android POS terminal application for Comanda — provides on-site payment processing via SumUp card terminals, QR code scanning, and thermal receipt printing.

> **Setting up tablets for an event, or cutting a release?** See **[PROVISIONING.md](./PROVISIONING.md)** — the single-app release flow and the per-event tablet provisioning runbook (Device Setup, SumUp login, pairing, teardown). This README covers development and build details.

## Architecture

The app uses **Kotlin + Jetpack Compose** with two distinct activities:

1. **MainActivity** (Compose) — native payment UI with QR scanner, receipt printing, and SumUp card terminal integration
2. **WebPosActivity** (WebView) — loads the Next.js web app's `/pos` interface for the cashier workflow

### Key Design Decisions

- **Dual-activity architecture** — native Compose UI for hardware-integrated features, WebView for the web-based POS interface
- **SumUp SDK** for card terminal payments — connects to physical SumUp readers
- **ZXing** for barcode/QR code scanning — reads order codes and product barcodes
- **ESCPOS thermal printing** — prints receipts on compatible thermal printers
- **Multi-build-type** configuration — debug, dev, staging, and release variants with environment-specific URLs

## Tech Stack

| Category     | Technology                              |
| ------------ | --------------------------------------- |
| Language     | Kotlin                                  |
| UI           | Jetpack Compose (Material 3)            |
| Payments     | SumUp Merchant SDK 6.0                  |
| QR / Barcode | ZXing Core 3.5.4 + Android Embedded 4.3 |
| Printing     | ESCPOS-ThermalPrinter-Android 3.4       |
| WebView      | AndroidX WebKit 1.15                    |
| Min SDK      | 26 (Android 8.0)                        |
| Target SDK   | 35                                      |
| Compile SDK  | 36                                      |

## Project Structure

```
android/
├── app/
│   ├── src/
│   │   ├── main/               Main source code & resources
│   │   │   ├── java/           Kotlin code
│   │   │   ├── res/            Layouts, drawables, strings
│   │   │   └── AndroidManifest.xml
│   │   ├── dev/                Dev-specific resources
│   │   ├── test/               Unit tests
│   │   └── androidTest/        Instrumentation tests
│   ├── build.gradle.kts        App configuration
│   └── proguard-rules.pro      R8 obfuscation rules
├── build.gradle.kts            Project configuration
├── gradle/                     Gradle wrapper
├── settings.gradle.kts
└── dist/                       Distribution APK/AAB files
```

## Build Variants

| Variant     | App Name              | POS URL                   | Debuggable |
| ----------- | --------------------- | ------------------------- | ---------- |
| **debug**   | Comanda POS (Debug)   | http://localhost:3000/pos | Yes        |
| **dev**     | Comanda POS (Dev)     | http://localhost:3000/pos | Yes        |
| **staging** | Comanda POS (Staging) | Configured via build      | No         |
| **release** | Comanda POS           | Configured via `POS_URL`  | No         |

## Prerequisites

- **Android Studio** (latest stable)
- **JDK 11+**
- Physical or emulated Android device (SDK 26+)
- **SumUp developer account** (for payment integration)

## Development Setup

```bash
# Open in Android Studio
# Sync Gradle
# Select "debug" build variant
# Run on device/emulator
```

### ADB Port Forwarding

When developing against a local backend, forward the required ports to the Android device:

```bash
adb reverse tcp:3000 tcp:3000     # Next.js web app
adb reverse tcp:8080 tcp:8080     # Go backend API
adb reverse tcp:9000 tcp:9000     # MinIO object storage
```

## Versioning

Version codes are automatically derived from git tags:

```
Tag: v2.1.2
Version Name: 2.1.2
Version Code: 20102  (major×10000 + minor×100 + patch)
```

## Release Signing

Release builds require a PKCS12 keystore configured via environment variables:

| Variable                        | Purpose                                    |
| ------------------------------- | ------------------------------------------ |
| `COMANDA_UPLOAD_STORE_FILE`     | Path to keystore file                      |
| `COMANDA_UPLOAD_STORE_PASSWORD` | Keystore password                          |
| `COMANDA_UPLOAD_KEY_ALIAS`      | Key alias                                  |
| `COMANDA_UPLOAD_KEY_PASSWORD`   | Key password (same as store password)      |
| `SUMUP_AFFILIATE_KEY`           | SumUp affiliate key (required for release) |
| `POS_URL`                       | Production POS URL (required for release)  |

The signing key is permanent: tablets only accept updates signed with the same key as the installed APK, so rotating it means reinstalling every device. Generate it once, keep it out of the repo, and back up the `.jks` plus password in the password manager.

### Generating a Keystore

```bash
keytool -genkeypair -v \
  -keystore comanda-upload.jks \
  -storepass "<password>" \
  -keypass "<password>" \
  -alias comandaUpload \
  -keyalg RSA -keysize 4096 \
  -validity 10000 -storetype PKCS12 \
  -dname "CN=Comanda,OU=Mobile Development,O=Comanda"

# Base64 encode for CI secrets
base64 -i comanda-upload.jks | pbcopy
```

PKCS12 keystores use one password for store and key — set both to the same value.

### GitHub Environment secrets

The release workflow (`.github/workflows/android-release.yml`) runs in the `staging` or `production` GitHub Environment. One keystore is shared across both. Each environment needs:

| Name                         | Kind     | Value                                                                    |
| ---------------------------- | -------- | ------------------------------------------------------------------------ |
| `ANDROID_KEYSTORE_BASE64`    | secret   | base64 of `comanda-upload.jks`                                            |
| `ANDROID_KEYSTORE_PASSWORD`  | secret   | keystore password                                                         |
| `ANDROID_KEY_ALIAS`          | secret   | `comandaUpload`                                                           |
| `ANDROID_KEY_ALIAS_PASSWORD` | secret   | key password (same as keystore password)                                  |
| `SUMUP_AFFILIATE_KEY`        | secret   | shared SumUp affiliate key                                                |
| `POS_URL`                    | secret   | default POS URL a fresh install points at (overridden in Device Setup)    |
| `ANDROID_SETUP_PIN`          | secret   | Device Setup PIN                                                          |
| `UPDATE_CHECK_URL`           | variable | `https://api.<domain>/v1/android/latest` (channel appended at build time) |

## Hardware Integration

- **SumUp Card Reader** — Bluetooth-connected card terminal for in-person payments
- **Thermal Printer** — ESCPOS-compatible printers for receipt printing
- **Camera** — QR code and barcode scanning via ZXing
