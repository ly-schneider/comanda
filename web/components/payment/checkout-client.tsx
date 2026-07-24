"use client"

import { ArrowLeft } from "lucide-react"
import { useRouter } from "next/navigation"
import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { useAuth } from "@/contexts/auth-context"
import { useCart } from "@/contexts/cart-context"
import { isApiError } from "@/lib/api"
import { createOrder, initiatePayment } from "@/lib/api/payments"
import { extractSoldOutItems, extractUnavailableItems, stashSoldOut } from "@/lib/checkout-sold-out"
import { addOrder } from "@/lib/orders-storage"
import { cartDepositCents, formatChf } from "@/lib/utils"

export function CheckoutClient() {
  const { user, accessToken } = useAuth()
  const { cart } = useCart()
  const depositCents = cartDepositCents(cart.items)
  const router = useRouter()
  const [email, setEmail] = useState(user?.email || "")
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handlePayment = async () => {
    if (cart.items.length === 0) return

    setLoading(true)
    setError(null)

    try {
      const items = cart.items.map((i) => ({
        productId: i.product.id,
        quantity: i.quantity,
        bundleSelections: i.configuration
          ? Object.entries(i.configuration).map(([slotId, productId]) => ({
              slotId,
              productId,
              variantId: i.configurationVariants?.[slotId]?.id,
            }))
          : undefined,
        variantId: i.variantId,
      }))

      // Step 1: Create order
      const orderRes = await createOrder({ items, contactEmail: email || undefined }, accessToken || undefined)

      const returnUrl = `${window.location.origin}/checkout/payment/next?order_id=${encodeURIComponent(orderRes.id)}`

      // Before leaving for Payrexx: customers routinely pay and then close the
      // TWINT app instead of coming back, and anything stored on return is lost
      // with that trip.
      addOrder(orderRes.id)

      // Store order ID for success page
      try {
        sessionStorage.setItem(
          "comanda.pending_order",
          JSON.stringify({
            orderId: orderRes.id,
            items: cart.items,
            totalCents: cart.totalCents,
          })
        )
      } catch {}

      // Step 2: Initiate payment
      const paymentRes = await initiatePayment(
        orderRes.id,
        { method: "twint", channel: "web", returnUrl },
        accessToken || undefined
      )

      if (paymentRes.redirectUrl) {
        // Production: redirect to Payrexx payment page
        window.location.href = paymentRes.redirectUrl
      } else {
        throw new Error("Missing payment redirect URL")
      }
    } catch (e) {
      // Sold out or deactivated: send the customer back to the cart with the
      // offending lines flagged, rather than a generic error on the payment step.
      if (isApiError(e) && e.code === "out_of_stock") {
        stashSoldOut(extractSoldOutItems(e.details))
        router.push("/checkout")
        return
      }
      if (isApiError(e) && e.code === "product_unavailable") {
        stashSoldOut(extractUnavailableItems(e.details))
        router.push("/checkout")
        return
      }
      const msg = isApiError(e) || e instanceof Error ? e.message : "Fehler beim Erstellen der Zahlung"
      setError(msg)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-2 rounded-[11px] border p-4">
        {depositCents > 0 && (
          <>
            <div className="text-muted-foreground flex items-center justify-between text-sm">
              <span>Zwischensumme</span>
              <span>{formatChf(cart.totalCents)}</span>
            </div>
            <div className="text-muted-foreground flex items-center justify-between text-sm">
              <span>Depot</span>
              <span>{formatChf(depositCents)}</span>
            </div>
          </>
        )}
        <div className="flex items-center justify-between">
          <span>Gesamtsumme</span>
          <strong>{formatChf(cart.totalCents + depositCents)}</strong>
        </div>
      </div>

      <div className="flex flex-col gap-1">
        <label htmlFor="receipt-email" className="text-sm">
          E-Mail für Quittung (optional)
        </label>
        <Input
          id="receipt-email"
          type="email"
          placeholder="deine@email.com"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          inputMode="email"
          autoComplete="email"
          className="w-full rounded-[11px] border px-3 py-5"
        />
      </div>

      {error && (
        <p className="text-red-600" role="alert">
          {error}
        </p>
      )}

      <div className="bg-background fixed right-0 bottom-0 left-0 z-50 p-4">
        <div className="mx-auto flex max-w-xl items-center justify-between gap-3">
          <Button
            onClick={() => router.back()}
            size="icon"
            variant="outline"
            className="size-12 shrink-0 rounded-full bg-white text-black"
          >
            <ArrowLeft className="size-5" />
          </Button>

          <Button
            onClick={handlePayment}
            disabled={loading || cart.items.length === 0}
            className="rounded-pill h-12 flex-1 text-base font-medium md:min-w-64 md:flex-none"
          >
            {loading ? "Wird vorbereitet…" : "Jetzt mit TWINT zahlen"}
          </Button>
        </div>
      </div>
    </div>
  )
}
