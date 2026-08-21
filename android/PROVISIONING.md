# Comanda POS — Release & Tablet Provisioning

Operational runbook for the Comanda POS Android app. Two parts:

1. [Release flow](#release-flow) — how the APKs are cut and how tablets self-update.
2. [Per-event tablet provisioning](#per-event-tablet-provisioning) — how any operator sets up a tablet for a tenant, and hands it over between events.

A release produces the **neutral APK plus one branded APK per tenant** that asks for one. A tenant build carries that tenant's name, icon and POS host, and installs alongside other tenants' apps on the same tablet — which is the point: one venue running two tenants needs both. The neutral build stays for demos and new-tenant bring-up, and is the only one where the host can still be switched at runtime.

---

## Release flow

### The APKs

- **Neutral** — `comanda-pos.apk`, `applicationId ch.leys.comanda` (`.staging` / `.dev` suffixes off prod). Host is switchable in Device Setup.
- **Per tenant** — `comanda-pos-<pkg>.apk`, `applicationId ch.leys.comanda.<pkg>`, where `<pkg>` is the tenant's immutable `android_package_id` (`t_` + slug with `-`→`_`). Carries that tenant's launcher name, generated icon, and baked POS host; the host field is removed from Device Setup entirely, because whatever page the app loads holds the native bridge for card payments and printing.
- Baked into every build:
  - `SUMUP_AFFILIATE_KEY` — shared across tenants. Which SumUp *account* a tablet settles into is chosen at login, not baked in.
  - `UPDATE_CHECK_URL` — the self-update endpoint, with channel, package and download token appended per build.
  - `API_BASE_URL` — derived from `UPDATE_CHECK_URL`; the Device Setup PIN is checked against it.
- The **Device Setup PIN is not in the APK.** It lives server-side in `ANDROID_SETUP_PIN` and is checked over the network, so Device Setup needs connectivity.

### Cutting a release

Releases are cut from GitHub Actions, not locally:

1. GitHub → **Actions** → **Android Release** → **Run workflow**.
2. Inputs:
   - `environment` — `staging` or `production`.
   - `version` — semver `X.Y.Z` (e.g. `1.0.9`).
   - `tenants` — JSON array of tenant builds, or `[]` for the neutral build only:
     `[{"pkg":"t_acme","label":"Acme","posUrl":"https://acme.comanda.leys.ch/pos","iconBg":"#0B5D3B","iconUrl":"https://…/logo.png"}]`.
     `iconBg`/`iconUrl` are optional — with no usable logo the icon falls back to the tenant's initials.
     Get `pkg` from the tenant's **POS-App installieren** card in platform admin.
3. The workflow (`.github/workflows/android-release.yml`):
   - runs quality checks (`lint`, `test`, `assembleDebug`);
   - builds and signs one APK per matrix leg (`assembleStaging` for staging, `assembleRelease` for production);
   - names each `comanda-pos[-<pkg>].apk` with a matching `.sha256` — the update feed matches these names exactly;
   - publishes a **GitHub Release** tagged `android-<environment>-v<version>` (staging is flagged *prerelease*).

The version code is derived from the version: `major×10000 + minor×100 + patch` (so `1.0.9` → `10009`). Self-update compares this number, so **versions must only ever increase**.

Signing keys and shared secrets are per-environment GitHub Environment secrets/vars — `ANDROID_KEYSTORE_BASE64`, `ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS`, `ANDROID_KEY_ALIAS_PASSWORD`, `SUMUP_AFFILIATE_KEY`, `POS_URL`, `ANDROID_DOWNLOAD_SECRET`, and the `UPDATE_CHECK_URL` variable. `ANDROID_DOWNLOAD_SECRET` must match the backend's Doppler value for the same environment, or tablets fail their update check once `ANDROID_REQUIRE_DOWNLOAD_TOKEN` is on. See [README → Release Signing](./README.md#release-signing) for the keystore details.

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
- You know the **Device Setup PIN** (platform-level, set server-side as `ANDROID_SETUP_PIN`). The tablet needs network to check it.
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
