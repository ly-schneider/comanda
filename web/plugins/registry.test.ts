import { describe, expect, it } from "vitest"
import { getPlatformConfigComponent, getPluginManifest } from "./registry"

describe("plugin registry", () => {
  it("registers the membership plugin with its slots", () => {
    const membership = getPluginManifest("membership")
    expect(membership?.name).toBe("Mitgliederkarten")
    expect(Object.keys(membership?.slots ?? {}).sort()).toEqual([
      "admin.settings.section",
      "platform.plugin.config",
      "pos.gratis.actions",
      "shop.claim.page",
      "shop.home.actions",
    ])
  })

  it("registers the stationqr plugin with its slots", () => {
    const stationqr = getPluginManifest("stationqr")
    expect(stationqr?.name).toBe("Station QR")
    expect(Object.keys(stationqr?.slots ?? {}).sort()).toEqual(["admin.stations.actions", "device.pairing.actions"])
  })

  it("returns undefined for an unknown manifest", () => {
    expect(getPluginManifest("nope")).toBeUndefined()
  })

  it("returns undefined platform config for an unknown plugin", () => {
    expect(getPlatformConfigComponent("nope")).toBeUndefined()
  })
})
