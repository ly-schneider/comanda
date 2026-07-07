import { describe, expect, it } from "vitest"
import { getPlatformConfigComponent, getPluginManifest } from "./registry"

describe("plugin registry", () => {
  it("registers the club100 plugin with its slots", () => {
    const club100 = getPluginManifest("club100")
    expect(club100?.name).toBe("100 Club")
    expect(Object.keys(club100?.slots ?? {}).sort()).toEqual([
      "admin.settings.section",
      "platform.plugin.config",
      "pos.basket.actions",
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
