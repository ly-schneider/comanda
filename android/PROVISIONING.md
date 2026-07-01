# Comanda POS — Release & Tablet Provisioning

Operational runbook for the Comanda POS Android app. Two parts:

1. [Single-app release flow](#single-app-release-flow) — how one neutral APK is cut and how tablets self-update.
2. [Per-event tablet provisioning](#per-event-tablet-provisioning) — how any operator sets up a tablet for a tenant, and hands it over between events.

There is **one app for every tenant.** The tenant a tablet serves is chosen at runtime (host + SumUp login on the Device Setup screen), never by building a different APK. Nothing here is per-tenant except the runtime steps in the provisioning section.

---

## Single-app release flow

### One neutral APK

- A single build — `comanda-pos.apk`, `applicationId ch.leys.comanda` (`.staging` / `.dev` suffixes on non-prod builds) — runs for all tenants.
- The build bakes in only **defaults and shared secrets**, not tenant identity:
  - `POS_URL` — the *default* host a fresh install points at. Overridden per-tablet at runtime in Device Setup, so it never needs to match the tenant.
  - `SUMUP_AFFILIATE_KEY` — the shared SumUp affiliate key (same for every tenant). Which SumUp *account* a tablet settles into is chosen at login time, not baked in.
  - `SETUP_PIN` — gates the Device Setup screen.
  - `UPDATE_CHECK_URL` — the self-update endpoint, with `?channel=staging|production` appended per build type.
- **No per-tenant builds.** Onboarding a new tenant needs zero release work.

### Cutting a release

Releases are cut from GitHub Actions, not locally:

1. GitHub → **Actions** → **Android Release** → **Run workflow**.
2. Inputs:
   - `environment` — `staging` or `production`.
   - `version` — semver `X.Y.Z` (e.g. `1.0.9`).
3. The workflow (`.github/workflows/android-release.yml`):
   - runs quality checks (`lint`, `test`, `assembleDebug`);
   - builds and signs the APK (`assembleStaging` for staging, `assembleRelease` for production);
   - renames the output to `comanda-pos.apk`, writes `comanda-pos.apk.sha256`;
   - publishes a **GitHub Release** tagged `android-<environment>-v<version>` (staging is flagged *prerelease*).

The version code is derived from the version: `major×10000 + minor×100 + patch` (so `1.0.9` → `10009`). Self-update compares this number, so **versions must only ever increase**.

Signing keys and shared secrets are per-environment GitHub Environment secrets/vars — `ANDROID_KEYSTORE_BASE64`, `ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS`, `ANDROID_KEY_ALIAS_PASSWORD`, `SUMUP_AFFILIATE_KEY`, `POS_URL`, `ANDROID_SETUP_PIN`, and the `UPDATE_CHECK_URL` variable. See [README → Release Signing](./README.md#release-signing) for the keystore details.

### How tablets self-update

Tablets pull updates themselves — no MDM, no manual reinstall for routine version bumps:

1. On launch, the POS calls its build's `UPDATE_CHECK_URL`, i.e. `GET <backend>/v1/android/latest?channel=<staging|production>` (`backend/internal/service/android_update.go`).
2. The backend finds the newest `android-<channel>-v*` GitHub Release and returns its `versionName`, `versionCode`, `downloadUrl`, and `sha256` (cached ~5 min).
3. If the remote `versionCode` is **higher** than the installed one, the app shows an **"Update verfügbar"** dialog. Tapping **"Installieren"** downloads the APK, verifies its SHA-256, and launches the system installer.

The first self-update on a device asks the user to allow **"install unknown apps"** for Comanda POS — grant it once.

A staging tablet only sees staging releases and a production tablet only sees production releases, because the channel is baked into each build's `UPDATE_CHECK_URL`.

---

## Per-event tablet provisioning

Follow this to set up a tablet for a tenant. It is repeatable: any operator can run it for any tenant, and the [teardown](#teardown--handover-between-events) leaves no trace of the previous tenant.

### Prerequisites (per tenant)

- The tenant has a **SumUp account** and a **SumUp card reader** (Solo / Air), charged and ready to pair.
- The tenant's **expected SumUp merchant code** is recorded on its settings (`sumup_merchant_code`). This is what the on-device guardrail checks the logged-in account against.
  - ⚠️ There is currently **no admin form field** for this — it is set via `PATCH /v1/settings` (tenant-admin scoped) or provisioned by Leys during onboarding. Until it is set, the guardrail shows *"Kein erwartetes SumUp-Konto…"* and **warns only** (it cannot block a wrong account). Set it before the event so the guardrail is active.
- The tenant's **POS host** is live and reachable — a tenant subdomain (`<slug>.comanda.leys.ch`, staging `<slug>.staging.comanda.leys.ch`) or a verified BYO custom domain. The platform apex (`comanda.leys.ch`) is **not** a valid host and is rejected.
- You know the **Device Setup PIN** for this build's environment (`ANDROID_SETUP_PIN`).
- A **Bluetooth thermal printer** (ESC/POS, or a Phomemo T02) for receipts.
- Access to the tenant's **web admin** (to approve the tablet's pairing code).

### Steps

**1. Install or update the app.**

- *First install (new tablet):* connect the tablet by USB, enable USB debugging, then from the repo:
  ```bash
  cd android && ./install-comanda-apk.sh
  ```
  Enter the environment (`production`/`staging`) and version; the script downloads `comanda-pos.apk` from the `android-<env>-v<version>` release and `adb install -r`s it. It needs `gh` authenticated (or a `GH_TOKEN`/`GITHUB_TOKEN` with Contents: Read on the repo). Alternatively, download the APK from the GitHub Release and sideload it.
- *Update (existing tablet):* just open the app. If a newer release exists on its channel, the **"Update verfügbar"** prompt appears → **"Installieren"** (see [self-update](#how-tablets-self-update)).

**2. Open Device Setup.**
On the POS screen, **long-press the top-left corner** (~72 dp square — a hidden operator gesture). The **"Geräte-Einrichtung"** PIN gate appears. Enter the PIN → **"Weiter"**.

**3. Point the tablet at the tenant host.**
In the **"Tenant-Host"** field, type the host (e.g. `kasse.comanda.leys.ch`), or tap **"QR-Code scannen"** and scan the tenant's setup QR (it encodes the host). Previously used hosts appear under **"Zuletzt verwendet"**. Tap **"Speichern & neu laden"** — the POS reloads at `https://<host>/pos`.

**4. Approve the pairing code.**
The reloaded POS shows a **6-character pairing code** (the device isn't approved yet). On a phone or laptop, sign in to the **tenant's web admin → Geräte** → **"Gerät koppeln"**, type the 6-character code, and tap **"Koppeln"**. The tablet finishes pairing, stores its device token, and loads the POS interface.

**5. Log into the tenant's SumUp account and confirm the match.**
Back in Device Setup, scroll to **"SumUp-Konto"**. It shows the expected account: *"Erwartet: `<code>` (`<tenant>`)"*. Tap **"SumUp-Konto anmelden"** (or **"SumUp-Konto wechseln"** if a previous account is still signed in) and complete the SumUp login. Then read the status line:

| Status line | Meaning | Action |
| --- | --- | --- |
| *"Angemeldet als `<code>` — stimmt mit `<tenant>` überein."* | ✅ Correct account | Done — card payments are allowed. |
| *"Falsches Konto: … Kartenzahlung ist gesperrt …"* | ❌ Wrong account — **card checkout is blocked** | Log out, log into the correct account. |
| *"Kein erwartetes SumUp-Konto … hinterlegt."* | ⚠️ No expected code set on the tenant | Set `sumup_merchant_code` (see prerequisites), or verify the account manually — the guardrail can't. |

**6. Pair the card reader and the printer.**
- *SumUp reader (Bluetooth):* pair it through SumUp's own reader setup, which SumUp prompts for during login / the first checkout. Reader charged and in pairing mode.
- *Thermal printer (Bluetooth):* bond it in the tablet's Android Bluetooth settings first. The POS printer picker uses paired devices (and falls back to the first paired printer if none is explicitly selected). A Phomemo T02 is detected and printed via the raster path automatically.

**7. Run a test card payment.**
Take a small **real** order in the POS, choose **card**, and run it on the reader. Confirm all three:
1. the payment succeeds on the reader and in the POS;
2. it appears in the **tenant's** SumUp dashboard;
3. the receipt prints.

This proves the tablet serves the right tenant and settles into the right SumUp account.

### Teardown / handover between events

Before moving a tablet from tenant A to tenant B, wipe tenant A:

1. Open **Device Setup** (long-press corner + PIN).
2. **"SumUp-Konto" → "Abmelden"** — log out of tenant A's SumUp account.
3. **"Auf Standard zurücksetzen"** (or enter tenant B's host and **"Speichern & neu laden"**). Changing or clearing the host **wipes tenant A's web session** — the device token in `localStorage` and all cookies — and clears the cached tenant config (expected merchant + name). If SumUp is still logged in, the **"Tenant gewechselt"** dialog offers **"Abmelden"** — choose it.

Result: no tenant-A SumUp session and no tenant-A device token remain. Re-provision for tenant B from **step 3** onward — the POS will show a fresh pairing code to approve in tenant B's admin.
