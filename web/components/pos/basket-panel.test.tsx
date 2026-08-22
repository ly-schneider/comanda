import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"
import { BasketPanel } from "@/components/pos/basket-panel"
import type { CreateOrderResult } from "@/types/order-queue"

// One over-stock basket line: the cashier holds qty 1 but live stock (stockMap)
// reads 0, so the sold-out UI ("Ausverkauft") shows — until their own pending
// order goes in flight, at which point it must be suppressed.
const h = vi.hoisted(() => {
  const product = {
    id: "p1",
    type: "simple",
    name: "Cola",
    image: null,
    priceCents: 350,
    depositCents: 0,
    isActive: true,
    category: { id: "c1", name: "Drinks" },
    bundle: null,
  }
  const item = { id: "i1", product, quantity: 1, totalPriceCents: 350 }
  return {
    cart: { items: [item], totalCents: 350 },
    clearCart: vi.fn(),
    updateQuantity: vi.fn(),
    removeFromCart: vi.fn(),
    discount: { pluginKey: "club100", label: "100 Club · Testperson", amountCents: 350, paymentData: { p: 1 } },
  }
})

vi.mock("@/contexts/cart-context", () => ({
  useCart: () => ({
    cart: h.cart,
    clearCart: h.clearCart,
    updateQuantity: h.updateQuantity,
    removeFromCart: h.removeFromCart,
    addToCart: vi.fn(),
  }),
}))
vi.mock("@/hooks/use-mobile", () => ({ useIsMobile: () => false }))
vi.mock("@/components/cart/use-best-bundle-suggestion", () => ({
  useBestBundleSuggestion: () => ({
    suggestion: null,
    allProductsReady: false,
    contiguous: false,
    startIndex: -1,
    endIndex: -1,
    dismissSuggestion: vi.fn(),
  }),
}))
vi.mock("@/components/plugins/plugin-slot", () => ({
  PluginSlot: ({ name, ctx }: { name: string; ctx: { applyDiscount?: (d: unknown) => void } }) =>
    name === "pos.gratis.actions" ? <button onClick={() => ctx.applyDiscount?.(h.discount)}>Testrabatt</button> : null,
}))

const OK_RESULT: CreateOrderResult = { ok: true, serverId: "o1", idempotencyKey: "k1" }

function renderPanel(overrides?: {
  createOrder?: () => Promise<CreateOrderResult>
  discardOrder?: () => Promise<void>
  submitOrder?: () => unknown
}) {
  const createOrder = vi.fn(overrides?.createOrder ?? (() => Promise.resolve(OK_RESULT)))
  const discardOrder = vi.fn(overrides?.discardOrder ?? (() => Promise.resolve()))
  const submitOrder = vi.fn<(...args: unknown[]) => unknown>(
    overrides?.submitOrder ?? (() => ({ localId: "l1", serverId: "o1" }))
  )
  render(
    <BasketPanel
      submitOrder={submitOrder as never}
      createOrder={createOrder as never}
      discardOrder={discardOrder as never}
      stockMap={new Map([["p1", 0]])}
    />
  )
  return { createOrder, discardOrder, submitOrder }
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
  h.discount.amountCents = 350
})

async function openGratisDialog() {
  fireEvent.click(screen.getByRole("button", { name: "Jetzt bezahlen" }))
  fireEvent.click(await screen.findByRole("button", { name: "Gratis" }))
  fireEvent.click(await screen.findByRole("button", { name: "Testrabatt" }))
}

describe("BasketPanel plugin discount from the Gratis dialog", () => {
  it("comps the order as gratis_plugin when the discount covers the basket", async () => {
    const { submitOrder } = renderPanel()

    await openGratisDialog()

    await waitFor(() => expect(submitOrder).toHaveBeenCalledTimes(1))
    const call = submitOrder.mock.calls[0]!
    expect(call[1]).toBe(0)
    expect(call[2]).toBe("gratis_plugin")
    expect(call[6]).toEqual({ key: "club100", data: { p: 1 } })
    expect(call[7]).toMatchObject({ serverId: "o1" })
  })

  it("returns to the tender grid and attaches the payload when a remainder is left", async () => {
    const { submitOrder } = renderPanel()
    h.discount.amountCents = 100

    await openGratisDialog()

    // Once in the basket summary, once in the tender dialog.
    expect(await screen.findAllByText("100 Club · Testperson")).toHaveLength(2)
    expect(submitOrder).not.toHaveBeenCalled()

    fireEvent.click(await screen.findByRole("button", { name: "Mit TWINT bezahlen" }))
    fireEvent.click(screen.getByRole("button", { name: "TWINT-Zahlung abschliessen" }))

    await waitFor(() => expect(submitOrder).toHaveBeenCalledTimes(1))
    const call = submitOrder.mock.calls[0]!
    expect(call[2]).toBe("twint")
    expect(call[6]).toEqual({ key: "club100", data: { p: 1 } })
  })
})

describe("BasketPanel sold-out suppression while a checkout is in flight", () => {
  it("flags the over-stock line before checkout, then hides it once the hold is created", async () => {
    const { createOrder } = renderPanel()

    expect(screen.getByText("Ausverkauft")).toBeInTheDocument()

    fireEvent.click(screen.getByRole("button", { name: "Jetzt bezahlen" }))

    await waitFor(() => expect(createOrder).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(screen.queryByText("Ausverkauft")).not.toBeInTheDocument())
  })

  it("restores the sold-out flag when the till is locked mid-checkout", async () => {
    renderPanel()

    fireEvent.click(screen.getByRole("button", { name: "Jetzt bezahlen" }))
    await waitFor(() => expect(screen.queryByText("Ausverkauft")).not.toBeInTheDocument())

    act(() => {
      window.dispatchEvent(new Event("pos:lock"))
    })

    expect(await screen.findByText("Ausverkauft")).toBeInTheDocument()
  })

  it("releases the hold and restores the flag when the checkout is abandoned", async () => {
    const { discardOrder } = renderPanel()

    fireEvent.click(screen.getByRole("button", { name: "Jetzt bezahlen" }))
    await waitFor(() => expect(screen.queryByText("Ausverkauft")).not.toBeInTheDocument())

    fireEvent.click(screen.getByRole("button", { name: "Close" }))

    await waitFor(() => expect(discardOrder).toHaveBeenCalledWith("o1"))
    expect(await screen.findByText("Ausverkauft")).toBeInTheDocument()
  })

  it("hands the pre-created order to the tender so completing consumes the hold", async () => {
    const { submitOrder, discardOrder } = renderPanel()

    fireEvent.click(screen.getByRole("button", { name: "Jetzt bezahlen" }))
    fireEvent.click(await screen.findByRole("button", { name: "Mit TWINT bezahlen" }))
    fireEvent.click(screen.getByRole("button", { name: "TWINT-Zahlung abschliessen" }))

    await waitFor(() => expect(submitOrder).toHaveBeenCalledTimes(1))
    // The 8th argument is the pre-created order; its presence proves the tender
    // consumed the in-flight hold rather than leaving it to be discarded.
    expect(submitOrder.mock.calls[0]?.[7]).toMatchObject({ serverId: "o1" })
    expect(discardOrder).not.toHaveBeenCalled()
  })
})
