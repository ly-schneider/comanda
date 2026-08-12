import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"
import StaffPerksCampaignList from "./campaign-list"

vi.mock("@/lib/csrf", () => ({ getCSRFToken: () => "csrf-token" }))
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }))

const fetchAuth = vi.fn()
vi.mock("@/hooks/use-authorized-fetch", () => ({ useAuthorizedFetch: () => fetchAuth }))

function jsonResponse(body: unknown, { ok = true, status = 200 } = {}) {
  return { ok, status, json: async () => body } as Response
}

const products = [
  { id: "prod_menu", name: "Menü Gross" },
  { id: "prod_cola", name: "Coca Cola" },
  { id: "prod_water", name: "Wasser" },
]

afterEach(() => {
  cleanup()
  fetchAuth.mockReset()
})

async function openCreateDialog() {
  fetchAuth.mockImplementation(async (url: string) => {
    if (url.startsWith("/api/v1/products")) return jsonResponse({ items: products })
    if (url.startsWith("/api/v1/plugins/staff-perks/campaigns")) return jsonResponse({ items: [] })
    return jsonResponse({})
  })
  render(<StaffPerksCampaignList />)
  fireEvent.click(await screen.findByRole("button", { name: /Neuer Gutschein/ }))
  fireEvent.change(await screen.findByLabelText("Name"), { target: { value: "Crew" } })
  await waitFor(() => expect(screen.getByRole("button", { name: /Gruppe hinzufügen/ })).toBeEnabled())
}

// Picks a product in the nth (0-based) still-empty product dropdown.
async function pickProduct(index: number, name: string) {
  const triggers = screen.getAllByRole("combobox")
  fireEvent.keyDown(triggers[index]!, { key: "ArrowDown" })
  fireEvent.click(await screen.findByRole("option", { name }))
}

type SubmittedProduct = { productId: string; quantity: number; choiceGroup: number }

function submittedProducts(): SubmittedProduct[] {
  const call = fetchAuth.mock.calls.find(([, init]) => init?.method === "POST") as [string, RequestInit]
  const body = JSON.parse(String(call[1].body)) as { products: SubmittedProduct[] }
  return body.products
}

describe("StaffPerksCampaignList create form", () => {
  it("puts each group in its own choice group", async () => {
    await openCreateDialog()

    fireEvent.click(screen.getByRole("button", { name: /Gruppe hinzufügen/ }))
    await pickProduct(0, "Menü Gross")
    fireEvent.click(screen.getByRole("button", { name: /Gruppe hinzufügen/ }))
    await pickProduct(1, "Coca Cola")
    fireEvent.click(screen.getByRole("button", { name: "Erstellen" }))

    await waitFor(() => expect(submittedProducts()).toHaveLength(2))
    expect(submittedProducts()).toEqual([
      { productId: "prod_menu", quantity: 1, choiceGroup: 1 },
      { productId: "prod_cola", quantity: 1, choiceGroup: 2 },
    ])
  })

  it("keeps the alternatives of one group together", async () => {
    await openCreateDialog()

    fireEvent.click(screen.getByRole("button", { name: /Gruppe hinzufügen/ }))
    await pickProduct(0, "Coca Cola")
    fireEvent.click(screen.getByRole("button", { name: /Alternative/ }))
    await pickProduct(1, "Wasser")
    fireEvent.click(screen.getByRole("button", { name: "Erstellen" }))

    await waitFor(() => expect(submittedProducts()).toHaveLength(2))
    expect(submittedProducts().map((p) => p.choiceGroup)).toEqual([1, 1])
  })

  it("merges a group into the one above when set to ODER", async () => {
    await openCreateDialog()

    fireEvent.click(screen.getByRole("button", { name: /Gruppe hinzufügen/ }))
    await pickProduct(0, "Menü Gross")
    fireEvent.click(screen.getByRole("button", { name: /Gruppe hinzufügen/ }))
    await pickProduct(1, "Coca Cola")
    fireEvent.click(screen.getByRole("button", { name: "ODER" }))
    fireEvent.click(screen.getByRole("button", { name: "Erstellen" }))

    await waitFor(() => expect(submittedProducts()).toHaveLength(2))
    expect(submittedProducts().map((p) => p.choiceGroup)).toEqual([1, 1])
  })

  it("spells out the structure before saving", async () => {
    await openCreateDialog()

    fireEvent.click(screen.getByRole("button", { name: /Gruppe hinzufügen/ }))
    await pickProduct(0, "Menü Gross")
    fireEvent.click(screen.getByRole("button", { name: /Gruppe hinzufügen/ }))
    await pickProduct(1, "Coca Cola")
    fireEvent.click(screen.getAllByRole("button", { name: /Alternative/ })[1]!)
    await pickProduct(2, "Wasser")

    expect(screen.getByText(/Pro Einlösung/)).toHaveTextContent(
      "Pro Einlösung: Menü Gross UND eines von (Coca Cola, Wasser)"
    )
  })

  it("refuses to submit a group with no product picked", async () => {
    await openCreateDialog()

    fireEvent.click(screen.getByRole("button", { name: /Gruppe hinzufügen/ }))
    fireEvent.click(screen.getByRole("button", { name: "Erstellen" }))

    expect(await screen.findByRole("alert")).toHaveTextContent("Bitte für jede Zeile ein Produkt wählen.")
    expect(fetchAuth.mock.calls.some(([, init]) => init?.method === "POST")).toBe(false)
  })
})
