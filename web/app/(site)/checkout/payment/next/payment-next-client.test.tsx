import { cleanup, render, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import PaymentNextClient from "./payment-next-client"

const replace = vi.fn()
let searchParams = new URLSearchParams()

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace, push: vi.fn(), back: vi.fn() }),
  useSearchParams: () => searchParams,
}))

vi.mock("@/contexts/cart-context", () => ({
  useCart: () => ({ clearCart: vi.fn() }),
}))

const getPaymentStatus = vi.fn()
vi.mock("@/lib/api/payments", () => ({
  getPaymentStatus: (orderId: string) => getPaymentStatus(orderId),
}))

const addOrder = vi.fn()
vi.mock("@/lib/orders-storage", () => ({
  addOrder: (id: string) => addOrder(id),
}))

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  replace.mockClear()
  addOrder.mockClear()
  getPaymentStatus.mockReset()
  searchParams = new URLSearchParams("order_id=order0000001")
})

afterEach(() => {
  vi.useRealTimers()
  cleanup()
})

describe("payment return page", () => {
  it("stores the order before any status is known", async () => {
    getPaymentStatus.mockResolvedValue({ status: "pending" })
    render(<PaymentNextClient />)

    await waitFor(() => expect(addOrder).toHaveBeenCalledWith("order0000001"))
  })

  it("goes to the success page once the payment confirms", async () => {
    getPaymentStatus.mockResolvedValue({ status: "paid" })
    render(<PaymentNextClient />)

    await waitFor(() => expect(replace).toHaveBeenCalledWith("/checkout/success?order_id=order0000001"))
  })

  it("survives transient request failures and still confirms", async () => {
    getPaymentStatus
      .mockRejectedValueOnce(new Error("network"))
      .mockRejectedValueOnce(new Error("network"))
      .mockResolvedValue({ status: "paid" })
    render(<PaymentNextClient />)

    await vi.advanceTimersByTimeAsync(10_000)

    expect(replace).toHaveBeenCalledWith("/checkout/success?order_id=order0000001")
    expect(replace).not.toHaveBeenCalledWith("/checkout/error")
  })

  it("hands a still-pending payment to the order page instead of the error page", async () => {
    getPaymentStatus.mockResolvedValue({ status: "pending" })
    render(<PaymentNextClient />)

    await vi.advanceTimersByTimeAsync(120_000)

    expect(replace).toHaveBeenCalledWith("/orders/order0000001?from=pending")
    expect(replace).not.toHaveBeenCalledWith("/checkout/error")
  })

  it("keeps routing an unreachable backend to the order page", async () => {
    getPaymentStatus.mockRejectedValue(new Error("offline"))
    render(<PaymentNextClient />)

    await vi.advanceTimersByTimeAsync(30_000)

    expect(replace).toHaveBeenCalledWith("/orders/order0000001?from=pending")
    expect(replace).not.toHaveBeenCalledWith("/checkout/error")
  })

  it("still treats an explicit provider cancel as a cancel", async () => {
    searchParams = new URLSearchParams("order_id=order0000001&result=cancel")
    render(<PaymentNextClient />)

    await waitFor(() => expect(replace).toHaveBeenCalledWith("/checkout/cancel"))
    expect(getPaymentStatus).not.toHaveBeenCalled()
  })
})
