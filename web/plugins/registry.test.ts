import { describe, expect, it } from "vitest"
import { getPlatformConfigComponent, getPluginManifest, registry } from "./registry"

describe("plugin registry", () => {
  it("has no plugins registered by default", () => {
    expect(registry).toEqual([])
  })

  it("returns undefined for an unknown manifest", () => {
    expect(getPluginManifest("nope")).toBeUndefined()
  })

  it("returns undefined platform config for an unknown plugin", () => {
    expect(getPlatformConfigComponent("nope")).toBeUndefined()
  })
})
