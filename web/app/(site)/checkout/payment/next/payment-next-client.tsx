"use client"

import { useRouter, useSearchParams } from "next/navigation"
import { useEffect, useRef } from "react"
import { useCart } from "@/contexts/cart-context"
import { getPaymentStatus } from "@/lib/api/payments"
import { addOrder } from "@/lib/orders-storage"

// Confirmation depends on Payrexx notifying our backend, which can lag well
// past the moment the customer lands back here. Waiting ~90s costs an impatient
// customer nothing (the order page keeps polling either way) and saves a paid
// customer from an error screen.
const POLL_WINDOW_MS = 90_000
const POLL_MIN_INTERVAL_MS = 1_000
const POLL_MAX_INTERVAL_MS = 4_000
// The trip back from the TWINT app routinely lands before the network does, so
// a lone failed request means nothing.
const MAX_CONSECUTIVE_ERRORS = 4

export default function PaymentNextClient() {
  const sp = useSearchParams()
  const router = useRouter()
  const { clearCart } = useCart()
  const clearedRef = useRef(false)
  const pollingRef = useRef(false)

  useEffect(() => {
    const result = sp.get("result")
    const orderIdParam = sp.get("order_id")

    if (result === "cancel") {
      router.replace("/checkout/cancel")
      return
    }
    if (result === "failed") {
      router.replace("/checkout/error")
      return
    }

    let cancelled = false

    const resolve = async () => {
      if (orderIdParam) {
        await pollOrder(orderIdParam)
        return
      }

      try {
        const raw = sessionStorage.getItem("comanda.pending_order")
        if (raw) {
          const parsed = JSON.parse(raw) as { orderId?: string }
          if (parsed?.orderId) {
            await pollOrder(parsed.orderId)
            return
          }
        }
      } catch {}

      router.replace("/checkout")
    }

    const settle = (orderId: string) => {
      if (!clearedRef.current) {
        clearedRef.current = true
        addOrder(orderId)
        clearCart()
      }
      try {
        sessionStorage.removeItem("comanda.pending_order")
      } catch {}
    }

    const pollOrder = async (orderId: string) => {
      if (pollingRef.current) return
      pollingRef.current = true

      // Keep the order reachable even if the customer closes this screen now.
      addOrder(orderId)

      const deadline = Date.now() + POLL_WINDOW_MS
      let interval = POLL_MIN_INTERVAL_MS
      let consecutiveErrors = 0

      while (!cancelled) {
        try {
          const res = await getPaymentStatus(orderId)
          consecutiveErrors = 0

          if (res.status === "paid") {
            settle(orderId)
            router.replace(`/checkout/success?order_id=${encodeURIComponent(orderId)}`)
            return
          }
          if (res.status === "cancelled") {
            router.replace("/checkout/cancel")
            return
          }
          if (res.status !== "pending") {
            router.replace("/checkout/error")
            return
          }
        } catch {
          consecutiveErrors++
          if (consecutiveErrors >= MAX_CONSECUTIVE_ERRORS) {
            // Repeatedly unreachable: the payment may well have gone through, so
            // hand over to the order page rather than claiming it failed.
            router.replace(`/orders/${encodeURIComponent(orderId)}?from=pending`)
            return
          }
        }

        if (Date.now() >= deadline) break

        await new Promise((r) => setTimeout(r, interval))
        interval = Math.min(interval * 1.5, POLL_MAX_INTERVAL_MS)
      }

      if (cancelled) return

      // Still unconfirmed. The order exists and keeps polling on its own page;
      // an error screen here would strand a customer whose money is already gone.
      router.replace(`/orders/${encodeURIComponent(orderId)}?from=pending`)
    }

    void resolve()

    return () => {
      cancelled = true
    }
  }, [sp, clearCart, router])

  return (
    <div className="flex min-h-[70vh] items-center justify-center">
      <p className="text-muted-foreground">Zahlungsstatus wird geprüft…</p>
    </div>
  )
}
