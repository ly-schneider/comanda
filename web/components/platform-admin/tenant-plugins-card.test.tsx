import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"
import { TenantPluginsCard } from "./tenant-plugins-card"

vi.mock("@/lib/csrf", () => ({ getCSRFToken: () => "csrf-token" }))

function jsonResponse(body: unknown, { ok = true, status = 200 } = {}) {
  return { ok, status, json: async () => body } as Response
}

const plugin = { key: "demo", name: "Demo Plugin", enabled: false, config: {}, secretsConfiguredAt: null }

afterEach(cleanup)

describe("TenantPluginsCard", () => {
  it("lists the tenant's plugins on mount", async () => {
    const fetchAuth = vi.fn().mockResolvedValue(jsonResponse({ items: [plugin] }))
    render(<TenantPluginsCard tenantId="t1" fetchAuth={fetchAuth} />)

    expect(await screen.findByText("Demo Plugin")).toBeInTheDocument()
    // A plugin with no config form shows just the toggle — no configuration status line.
    expect(screen.queryByText("Nicht konfiguriert")).not.toBeInTheDocument()
    expect(fetchAuth).toHaveBeenCalledWith("/api/v1/platform/tenants/t1/plugins")
  })

  it("shows the empty state when no plugins are registered", async () => {
    const fetchAuth = vi.fn().mockResolvedValue(jsonResponse({ items: [] }))
    render(<TenantPluginsCard tenantId="t1" fetchAuth={fetchAuth} />)

    expect(await screen.findByText("Keine Plugins registriert.")).toBeInTheDocument()
  })

  it("saves an enable toggle with the CSRF header and reflects the returned state", async () => {
    const fetchAuth = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ items: [plugin] }))
      .mockResolvedValueOnce(jsonResponse({ ...plugin, enabled: true, secretsConfiguredAt: "2026-07-07T00:00:00Z" }))
    render(<TenantPluginsCard tenantId="t1" fetchAuth={fetchAuth} />)

    fireEvent.click(await screen.findByRole("switch"))

    await waitFor(() => expect(fetchAuth).toHaveBeenCalledTimes(2))
    expect(fetchAuth).toHaveBeenLastCalledWith("/api/v1/platform/tenants/t1/plugins/demo", {
      method: "PUT",
      headers: { "Content-Type": "application/json", "X-CSRF": "csrf-token" },
      body: JSON.stringify({ enabled: true }),
    })
    await waitFor(() => expect(screen.getByRole("switch")).toBeChecked())
  })

  it("maps a 403 to a permission error", async () => {
    const fetchAuth = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ items: [plugin] }))
      .mockResolvedValueOnce(jsonResponse({}, { ok: false, status: 403 }))
    render(<TenantPluginsCard tenantId="t1" fetchAuth={fetchAuth} />)

    fireEvent.click(await screen.findByRole("switch"))

    expect(await screen.findByText("Keine Berechtigung für diese Aktion.")).toBeInTheDocument()
  })

  it("maps the unknown_plugin code to German copy", async () => {
    const fetchAuth = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ items: [plugin] }))
      .mockResolvedValueOnce(jsonResponse({ code: "unknown_plugin" }, { ok: false, status: 400 }))
    render(<TenantPluginsCard tenantId="t1" fetchAuth={fetchAuth} />)

    fireEvent.click(await screen.findByRole("switch"))

    expect(await screen.findByText("Unbekanntes Plugin.")).toBeInTheDocument()
  })
})
