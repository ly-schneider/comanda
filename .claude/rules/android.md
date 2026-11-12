---
paths:
  - "android/**"
---

# Android

- There is one neutral app. Organiser-branded builds come from the `tenants` input of `.github/workflows/android-release.yml`, not from Gradle flavors.
- A tenant's `android_package_id` never changes once a build has shipped, because tablets update by package name.
- The app is a WebView around the web till plus native bridges for SumUp and the Bluetooth printer. Fix till UI in `web/`, not here.
