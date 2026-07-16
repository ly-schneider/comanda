import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"
import { StationProductManager } from "./product-manager"

function jsonResponse(body: unknown, { ok = true, status = 200 } = {}) {
  return { ok, status, json: async () => body } as Response
}

const catalog = {
  items: [
    { id: "p1", name: "Prod 1" },
    { id: "p2", name: "Prod 2" },
    { id: "p3", name: "Prod 3" },
  ],
}

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe("StationProductManager", () => {
  it("loads the catalog on open and shows the current assignments", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse(catalog))
    vi.stubGlobal("fetch", fetchMock)

    render(<StationProductManager token="tok" assignedIds={["p1", "p2"]} onSaved={vi.fn()} />)
    fireEvent.click(screen.getByRole("button", { name: "Produkte verwalten" }))

    expect(await screen.findByText("Prod 1")).toBeInTheDocument()
    expect(screen.getByText("Prod 2")).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/products", { headers: { Authorization: "Bearer tok" } })
    expect(screen.getByRole("button", { name: "Speichern" })).toBeDisabled()
  })

  it("PUTs the replaced set (device-scoped, no station id) and calls onSaved", async () => {
    const onSaved = vi.fn()
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(catalog))
      .mockResolvedValueOnce(jsonResponse({ items: [{ productId: "p2" }] }))
    vi.stubGlobal("fetch", fetchMock)

    render(<StationProductManager token="tok" assignedIds={["p1", "p2"]} onSaved={onSaved} />)
    fireEvent.click(screen.getByRole("button", { name: "Produkte verwalten" }))

    fireEvent.click(await screen.findByRole("button", { name: "Produkt Prod 1 entfernen" }))
    fireEvent.click(screen.getByRole("button", { name: "Speichern" }))

    await waitFor(() => expect(onSaved).toHaveBeenCalledTimes(1))
    expect(fetchMock).toHaveBeenLastCalledWith("/api/v1/stations/me/products", {
      method: "PUT",
      headers: { "Content-Type": "application/json", Authorization: "Bearer tok" },
      body: JSON.stringify({ productIds: ["p2"] }),
    })
    expect(onSaved).toHaveBeenCalledWith([{ productId: "p2" }])
    expect(await screen.findByText(/Gespeichert/)).toBeInTheDocument()
  })

  it("surfaces a 403 (flag turned off) without calling onSaved", async () => {
    const onSaved = vi.fn()
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(catalog))
      .mockResolvedValueOnce(jsonResponse({}, { ok: false, status: 403 }))
    vi.stubGlobal("fetch", fetchMock)

    render(<StationProductManager token="tok" assignedIds={["p1", "p2"]} onSaved={onSaved} />)
    fireEvent.click(screen.getByRole("button", { name: "Produkte verwalten" }))

    fireEvent.click(await screen.findByRole("button", { name: "Produkt Prod 1 entfernen" }))
    fireEvent.click(screen.getByRole("button", { name: "Speichern" }))

    expect(await screen.findByText("Selbstverwaltung ist nicht mehr aktiviert.")).toBeInTheDocument()
    expect(onSaved).not.toHaveBeenCalled()
  })

  it("keeps in-progress edits across parent re-renders (fresh assignedIds array)", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(catalog))
    vi.stubGlobal("fetch", fetchMock)

    const { rerender } = render(<StationProductManager token="tok" assignedIds={["p1", "p2"]} onSaved={vi.fn()} />)
    fireEvent.click(screen.getByRole("button", { name: "Produkte verwalten" }))

    fireEvent.click(await screen.findByRole("button", { name: "Produkt Prod 1 entfernen" }))
    expect(screen.queryByText("Prod 1")).not.toBeInTheDocument()

    rerender(<StationProductManager token="tok" assignedIds={["p1", "p2"]} onSaved={vi.fn()} />)

    expect(screen.queryByText("Prod 1")).not.toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})
