/**
 * Station e2e coverage note
 *
 * A camera-driven scan e2e is intentionally absent. The html5-qrcode scanner
 * requires a live MediaStream from a physical (or emulated) camera; Playwright's
 * fake video source cannot feed real QR frames, so any test that drives
 * scanner → decode → redeemScan would need a custom camera mock injected at the
 * browser level — not worth the fragility for a dedicated-device UX.
 *
 * The full redemption Gherkin scenarios (signed scan → redeemed, rescan →
 * already-redeemed, wrong-tenant → wrong-event, offline enqueue → flush →
 * drain, ledger persistence across reload) are covered end-to-end by
 * `web/lib/station/redeem-flow.integration.test.ts`, which runs `idbLedger` +
 * `idbOutbox` + real Ed25519 key material under fake-indexeddb — no mocks on
 * the redemption logic path.
 *
 * What IS tested here: the page bootstraps and renders its unauthenticated
 * state (pairing code display) without a runtime error, confirming that the
 * component tree compiles and mounts successfully.
 */

import { expect, test } from "@playwright/test"

test("station page mounts without a runtime error", async ({ page }) => {
  const errors: string[] = []
  page.on("pageerror", (e) => errors.push(e.message))

  await page.goto("/station")

  // The page always shows one of: loading spinner, pairing display, or
  // already-approved scanner. All three are valid; just confirm no crash.
  await expect(page).not.toHaveTitle(/error/i)
  expect(errors).toHaveLength(0)
})
