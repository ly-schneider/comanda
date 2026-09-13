import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import type { ProductSummaryDTO } from "@/types/product"
import type { ArtistVoucherBatch, ArtistVoucherBatchVoucher } from "./api"
import VoucherBatchAdmin from "./batch-admin"
import { fmtDateTime } from "./format"

vi.mock("@/lib/csrf", () => ({ getCSRFToken: () => "csrf-token" }))
vi.mock("@/contexts/auth-context", () => ({ useAuth: () => ({ getToken: () => "session-token" }) }))

const BATCHES_URL = "/api/v1/plugins/artist-voucher/batches"

const fetchMock = vi.fn()

function jsonResponse(body: unknown, { ok = true, status = 200 } = {}) {
  return { ok, status, json: async () => body } as Response
}

const burger: ProductSummaryDTO = {
  id: "prod_burger",
  category: { id: "cat_food", name: "Essen" } as ProductSummaryDTO["category"],
  type: "simple",
  name: "Burger",
  image: null,
  priceCents: 900,
  depositCents: 0,
  isActive: true,
}

const shirt: ProductSummaryDTO = {
  ...burger,
  id: "prod_shirt",
  name: "T-Shirt",
  variants: [{ id: "var_m", value: "M", priceAdjustmentCents: 0, isActive: true, sequence: 1 }],
}

const menu: ProductSummaryDTO = { ...burger, id: "prod_menu", name: "Menü", type: "bundle", variants: undefined }

const batch: ArtistVoucherBatch = {
  id: "batch_1",
  label: "Aktion Bahnhofstrasse",
  validFrom: "2026-09-20T08:00:00Z",
  validUntil: "2026-09-27T20:00:00Z",
  products: [{ productId: "prod_burger", name: "Burger", quantity: 2, stock: 200 }],
  issued: 150,
  redeemed: 130,
  revoked: 2,
  outstanding: 18,
  status: "active",
  createdAt: "2026-09-19T09:00:00Z",
}

const vouchers: ArtistVoucherBatchVoucher[] = [
  { id: "vchRedeemed1", status: "redeemed", redeemedAt: "2026-09-21T12:30:00Z", revokedAt: null },
  { id: "vchActive111", status: "active", redeemedAt: null, revokedAt: null },
  { id: "vchRevoked11", status: "revoked", redeemedAt: null, revokedAt: "2026-09-22T09:00:00Z" },
]

function mockApi(batches: ArtistVoucherBatch[] = [batch]) {
  fetchMock.mockImplementation(async (url: string, init?: RequestInit) => {
    const method = (init?.method ?? "GET").toUpperCase()
    if (url === "/api/v1/products") return jsonResponse({ items: [burger, shirt, menu] })
    if (url === BATCHES_URL && method === "GET") return jsonResponse({ batches })
    if (url === BATCHES_URL && method === "POST") {
      const body = JSON.parse(String(init?.body)) as { label: string; count: number }
      return jsonResponse({ ...batch, id: "batch_2", label: body.label, issued: body.count }, { status: 201 })
    }
    if (url.endsWith("/revoke")) {
      return jsonResponse({ ...batch, outstanding: 0, revoked: 20, status: "revoked" })
    }
    if (url.startsWith(`${BATCHES_URL}/`) && method === "GET") return jsonResponse({ batch, vouchers })
    return jsonResponse({})
  })
}

function postBody(): Record<string, unknown> {
  const call = fetchMock.mock.calls.find(([url, init]) => url === BATCHES_URL && init?.method === "POST") as [
    string,
    RequestInit,
  ]
  return JSON.parse(String(call[1].body)) as Record<string, unknown>
}

async function openCreateDialog() {
  mockApi()
  render(<VoucherBatchAdmin />)
  fireEvent.click(await screen.findByRole("button", { name: "Neue Serie" }))
  fireEvent.change(screen.getByLabelText("Bezeichnung"), { target: { value: "Aktion Bahnhofstrasse" } })
  fireEvent.change(screen.getByLabelText("Gültig ab"), { target: { value: "2026-09-20T10:00" } })
  fireEvent.change(screen.getByLabelText("Gültig bis"), { target: { value: "2026-09-27T22:00" } })
}

async function pickProduct(name: string) {
  fireEvent.keyDown(screen.getByRole("combobox"), { key: "ArrowDown" })
  fireEvent.click(await screen.findByRole("option", { name }))
}

beforeEach(() => {
  vi.stubGlobal("fetch", fetchMock)
})

afterEach(() => {
  cleanup()
  fetchMock.mockReset()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe("VoucherBatchAdmin list", () => {
  it("shows the outstanding slips next to the stock behind them", async () => {
    mockApi()
    render(<VoucherBatchAdmin />)

    expect(await screen.findByText("18 Gutscheine offen · 200 Burger auf Lager")).toBeInTheDocument()
    expect(screen.getByText("2× Burger")).toBeInTheDocument()
    expect(screen.getByText("150 ausgegeben · 130 eingelöst · 2 widerrufen")).toBeInTheDocument()
    expect(screen.getByText("Aktiv")).toBeInTheDocument()
  })

  it("opens the slip PDF for the batch", async () => {
    mockApi()
    const open = vi.spyOn(window, "open").mockReturnValue(null)
    render(<VoucherBatchAdmin />)

    fireEvent.click(await screen.findByRole("button", { name: "Drucken" }))
    expect(open).toHaveBeenCalledWith(`${BATCHES_URL}/batch_1/print.pdf`, "_blank")
  })

  it("hides revoking once nothing is outstanding", async () => {
    mockApi([{ ...batch, outstanding: 0 }])
    render(<VoucherBatchAdmin />)

    expect(await screen.findByText("0 Gutscheine offen · 200 Burger auf Lager")).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Widerrufen" })).not.toBeInTheDocument()
  })
})

describe("VoucherBatchAdmin detail", () => {
  it("lists every slip with its status and redemption time", async () => {
    mockApi()
    render(<VoucherBatchAdmin />)

    fireEvent.click(await screen.findByRole("button", { name: "Details" }))
    const detail = await screen.findByRole("dialog")

    expect(within(detail).getByText("vchRedeemed1")).toBeInTheDocument()
    expect(within(detail).getByText("Eingelöst")).toBeInTheDocument()
    expect(within(detail).getByText(fmtDateTime("2026-09-21T12:30:00Z"))).toBeInTheDocument()
    expect(within(detail).getByText(`Widerrufen am ${fmtDateTime("2026-09-22T09:00:00Z")}`)).toBeInTheDocument()
    expect(within(detail).getByText("—")).toBeInTheDocument()
  })
})

describe("VoucherBatchAdmin create form", () => {
  it("submits label, window, entitlement and slip count", async () => {
    await openCreateDialog()
    await pickProduct("Burger")
    fireEvent.change(screen.getByLabelText("Menge"), { target: { value: "2" } })
    fireEvent.change(screen.getByLabelText("Anzahl Gutscheine"), { target: { value: "120" } })
    fireEvent.click(screen.getByRole("button", { name: "Erstellen" }))

    await waitFor(() => expect(postBody()).toBeTruthy())
    expect(postBody()).toEqual({
      label: "Aktion Bahnhofstrasse",
      validFrom: new Date("2026-09-20T10:00").toISOString(),
      validUntil: new Date("2026-09-27T22:00").toISOString(),
      products: [{ productId: "prod_burger", quantity: 2 }],
      count: 120,
    })
    expect(await screen.findByText("120 ausgegeben · 130 eingelöst · 2 widerrufen")).toBeInTheDocument()
  })

  it("offers only products a slip can carry", async () => {
    await openCreateDialog()
    fireEvent.keyDown(screen.getByRole("combobox"), { key: "ArrowDown" })

    expect(await screen.findByRole("option", { name: "Burger" })).toBeInTheDocument()
    expect(screen.queryByRole("option", { name: "T-Shirt" })).not.toBeInTheDocument()
    expect(screen.queryByRole("option", { name: "Menü" })).not.toBeInTheDocument()
  })

  it("refuses to submit without a product", async () => {
    await openCreateDialog()
    fireEvent.click(screen.getByRole("button", { name: "Erstellen" }))

    expect(await screen.findByRole("alert")).toHaveTextContent("Bitte für jede Zeile ein Produkt wählen.")
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === "POST")).toBe(false)
  })

  it("refuses a validity window that ends before it starts", async () => {
    await openCreateDialog()
    fireEvent.change(screen.getByLabelText("Gültig bis"), { target: { value: "2026-09-19T22:00" } })
    await pickProduct("Burger")
    fireEvent.click(screen.getByRole("button", { name: "Erstellen" }))

    expect(await screen.findByRole("alert")).toHaveTextContent("Das Enddatum muss nach dem Startdatum liegen.")
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === "POST")).toBe(false)
  })

  it("surfaces the backend's reason for a rejected product", async () => {
    await openCreateDialog()
    await pickProduct("Burger")
    fetchMock.mockImplementation(async () =>
      jsonResponse(
        {
          code: "product_has_variants",
          message: "Produkte mit Varianten können nicht als Gutschein ausgegeben werden.",
        },
        { ok: false, status: 400 }
      )
    )
    fireEvent.click(screen.getByRole("button", { name: "Erstellen" }))

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Produkte mit Varianten können nicht als Gutschein ausgegeben werden."
    )
  })
})

describe("VoucherBatchAdmin revoke", () => {
  it("revokes the outstanding slips after confirmation", async () => {
    mockApi()
    render(<VoucherBatchAdmin />)

    fireEvent.click(await screen.findByRole("button", { name: "Widerrufen" }))
    const confirm = await screen.findByRole("alertdialog")
    expect(confirm).toHaveTextContent("bereits eingelöste bleiben unverändert")
    fireEvent.click(within(confirm).getByRole("button", { name: "Widerrufen" }))

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        `${BATCHES_URL}/batch_1/revoke`,
        expect.objectContaining({ method: "POST" })
      )
    )
    expect(await screen.findByText("0 Gutscheine offen · 200 Burger auf Lager")).toBeInTheDocument()
    expect(screen.getByText("Widerrufen")).toBeInTheDocument()
  })

  it("keeps the batch untouched when the confirmation is dismissed", async () => {
    mockApi()
    render(<VoucherBatchAdmin />)

    fireEvent.click(await screen.findByRole("button", { name: "Widerrufen" }))
    fireEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Abbrechen" }))

    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument())
    expect(fetchMock.mock.calls.some(([url]) => String(url).endsWith("/revoke"))).toBe(false)
    expect(screen.getByText("18 Gutscheine offen · 200 Burger auf Lager")).toBeInTheDocument()
  })
})
