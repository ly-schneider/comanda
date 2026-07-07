import { render } from "@testing-library/react"
import { describe, expect, it } from "vitest"
import { PluginProvider, usePlugins } from "./plugin-provider"
import { PluginSlot } from "./plugin-slot"

describe("PluginSlot", () => {
  it("renders nothing when no plugin is enabled", () => {
    const { container } = render(
      <PluginProvider enabledPlugins={[]}>
        <PluginSlot name="pos.basket.actions" ctx={{ cartItems: [], setPluginDiscount: () => {} }} />
      </PluginProvider>
    )
    expect(container).toBeEmptyDOMElement()
  })

  it("renders nothing when an enabled plugin has no matching manifest slot", () => {
    const { container } = render(
      <PluginProvider enabledPlugins={[{ key: "unregistered", name: "X", config: {} }]}>
        <PluginSlot name="admin.settings.section" ctx={{ token: "t" }} />
      </PluginProvider>
    )
    expect(container).toBeEmptyDOMElement()
  })
})

describe("usePlugins", () => {
  it("exposes public config via getPluginConfig", () => {
    let config: Record<string, unknown> | undefined
    function Probe() {
      config = usePlugins().getPluginConfig("acme")
      return null
    }
    render(
      <PluginProvider enabledPlugins={[{ key: "acme", name: "Acme", config: { max: 3 } }]}>
        <Probe />
      </PluginProvider>
    )
    expect(config).toEqual({ max: 3 })
  })
})
