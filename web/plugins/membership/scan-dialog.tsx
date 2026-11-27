"use client"

import { useCallback, useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { formatChf } from "@/lib/utils"
import type { CartItem } from "@/types/cart"
import { lookupMembershipCard, type MembershipLookupResult } from "./api"
import { CardScanner } from "./card-scanner"
import { computeMembershipDiscount, type MembershipDiscount } from "./discount"

interface ScanDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onApply: (lookup: MembershipLookupResult, discount: MembershipDiscount) => void
  token: string
  label: string
  cartItems: CartItem[]
  freeProductIds: string[]
}

export function MembershipScanDialog({
  open,
  onOpenChange,
  onApply,
  token,
  label,
  cartItems,
  freeProductIds,
}: ScanDialogProps) {
  const [checking, setChecking] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [lookup, setLookup] = useState<MembershipLookupResult | null>(null)

  useEffect(() => {
    if (open) {
      setLookup(null)
      setError(null)
      setChecking(false)
    }
  }, [open])

  const check = useCallback(
    async (code: string) => {
      setChecking(true)
      setError(null)
      try {
        setLookup(await lookupMembershipCard(token, code))
      } catch (e) {
        setError(e instanceof Error ? e.message : "Karte konnte nicht geprüft werden")
      } finally {
        setChecking(false)
      }
    },
    [token]
  )

  const remaining = lookup?.entitlement?.remaining ?? 0
  const discount = lookup?.canRedeem ? computeMembershipDiscount(cartItems, freeProductIds, remaining) : null

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] flex-col overflow-y-auto" onOpenAutoFocus={(e) => e.preventDefault()}>
        <DialogHeader>
          <DialogTitle>{label}</DialogTitle>
        </DialogHeader>

        {!lookup && <CardScanner onCode={check} busy={checking} manualLabel="Mitglied-Nummer" />}
        {checking && <p className="text-muted-foreground text-sm">Karte wird geprüft…</p>}
        {error && (
          <div role="alert" className="text-destructive bg-destructive/10 rounded-xl px-3 py-2 text-sm">
            {error}
          </div>
        )}

        {lookup && (
          <div className="space-y-3">
            <div>
              <p className="font-medium">{lookup.member?.name ?? "Mitglied"}</p>
              <p className="text-muted-foreground text-sm">{lookup.message}</p>
            </div>

            {discount ? (
              <div className="space-y-1 rounded-xl bg-green-100 px-3 py-2 dark:bg-green-950/30">
                {discount.items.map((item) => (
                  <div
                    key={item.cartItemId}
                    className="flex justify-between text-sm text-green-700 dark:text-green-400"
                  >
                    <span>
                      {item.freeQuantity} × {item.productName}
                    </span>
                    <span>−{formatChf(item.freeQuantity * item.unitPriceCents)}</span>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-sm text-amber-600">
                {lookup.canRedeem
                  ? "Im Warenkorb ist nichts, was gratis abgegeben wird."
                  : "Es kann nichts gratis abgegeben werden."}
              </p>
            )}
          </div>
        )}

        <DialogFooter>
          {lookup && (
            <Button variant="outline" onClick={() => setLookup(null)}>
              Andere Karte
            </Button>
          )}
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Abbrechen
          </Button>
          {lookup && discount && <Button onClick={() => onApply(lookup, discount)}>Gratis übernehmen</Button>}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
