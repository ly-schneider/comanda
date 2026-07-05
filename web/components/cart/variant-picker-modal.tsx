"use client"

import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { useCart } from "@/contexts/cart-context"
import { activeVariants, formatPriceAdjustment, variantPickerLabel } from "@/lib/variants"
import type { ProductDTO, ProductVariantDTO } from "@/types"

interface VariantPickerModalProps {
  product: ProductDTO
  isOpen: boolean
  onClose: () => void
  isPOS?: boolean
}

export function VariantPickerModal({ product, isOpen, onClose, isPOS = false }: VariantPickerModalProps) {
  const { addToCart } = useCart()
  const [selected, setSelected] = useState<ProductVariantDTO | null>(null)

  const variants = activeVariants(product)
  const label = variantPickerLabel(product)

  const handleConfirm = () => {
    if (!selected) return
    addToCart(product, undefined, selected.id)
    setSelected(null)
    onClose()
  }

  const handleOpenChange = (open: boolean) => {
    if (!open) {
      setSelected(null)
      onClose()
    }
  }

  return (
    <Dialog open={isOpen} onOpenChange={handleOpenChange}>
      <DialogContent className="max-h-[80vh] max-w-sm overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="font-family-primary text-xl">{label} wählen</DialogTitle>
        </DialogHeader>

        <div className="space-y-2">
          {variants.map((variant) => {
            const isSelected = selected?.id === variant.id
            const isOutOfStock = variant.isAvailable === false
            const isLow = variant.isLowStock === true
            const qty = typeof variant.availableQuantity === "number" ? variant.availableQuantity : null
            const adj = formatPriceAdjustment(variant.priceAdjustmentCents)

            return (
              <button
                key={variant.id}
                type="button"
                disabled={isOutOfStock}
                onClick={() => setSelected(variant)}
                className={[
                  "relative w-full rounded-2xl border px-4 py-3 text-left transition-all",
                  isSelected ? "ring-primary bg-primary/5 border-transparent ring-2" : "bg-muted/40 hover:bg-muted/70",
                  isOutOfStock ? "pointer-events-none opacity-60" : "cursor-pointer",
                ].join(" ")}
                aria-pressed={isSelected}
                aria-disabled={isOutOfStock}
              >
                <div className="flex items-center justify-between gap-2">
                  <span className={`font-family-secondary font-medium ${isPOS ? "text-sm" : "text-base"}`}>
                    {variant.value}
                  </span>
                  <div className="flex shrink-0 flex-col items-end gap-1">
                    {adj && <span className={`text-muted-foreground ${isPOS ? "text-xs" : "text-sm"}`}>{adj}</span>}
                    {isOutOfStock && (
                      <span className="rounded-full bg-red-600 px-2 py-0.5 text-xs font-medium text-white">
                        Ausverkauft
                      </span>
                    )}
                    {!isOutOfStock && isLow && qty !== null && (
                      <span className="rounded-full bg-amber-600 px-2 py-0.5 text-xs font-medium text-white">
                        Nur {qty} übrig
                      </span>
                    )}
                  </div>
                </div>
              </button>
            )
          })}
        </div>

        <DialogFooter>
          <Button
            onClick={handleConfirm}
            disabled={!selected}
            className="rounded-pill h-12 w-full text-base font-medium"
          >
            {isPOS ? "Hinzufügen" : "Zum Warenkorb hinzufügen"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
