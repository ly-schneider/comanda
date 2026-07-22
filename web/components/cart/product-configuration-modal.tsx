"use client"

import Image from "next/image"
import React, { useState } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { useCart } from "@/contexts/cart-context"
import { activeVariants, findVariant, hasVariants } from "@/lib/variants"
import { BundleSlotDTO, CartItemConfiguration, CartItemConfigurationVariants, ProductDTO } from "@/types"

interface ProductConfigurationModalProps {
  product: ProductDTO
  isOpen: boolean
  onClose: () => void
  initialConfiguration?: CartItemConfiguration
  initialConfigurationVariants?: CartItemConfigurationVariants
  editingItemId?: string // when provided, modal acts in edit mode
  isPOS?: boolean
}

export function ProductConfigurationModal({
  product,
  isOpen,
  onClose,
  initialConfiguration,
  initialConfigurationVariants,
  editingItemId,
  isPOS = false,
}: ProductConfigurationModalProps) {
  const { addToCart, updateItemConfiguration } = useCart()
  const [selectedConfiguration, setSelectedConfiguration] = useState<CartItemConfiguration>(initialConfiguration || {})
  const [selectedVariants, setSelectedVariants] = useState<CartItemConfigurationVariants>(
    initialConfigurationVariants || {}
  )

  const handleSlotSelection = (slotId: string, productId: string) => {
    setSelectedConfiguration((prev) => ({
      ...prev,
      [slotId]: productId,
    }))
    // A different product invalidates the slot's previous variant choice.
    setSelectedVariants((prev) => {
      if (!(slotId in prev)) return prev
      const next = { ...prev }
      delete next[slotId]
      return next
    })
  }

  const handleVariantSelection = (slotId: string, variant: { id: string; value: string }) => {
    setSelectedVariants((prev) => ({ ...prev, [slotId]: variant }))
  }

  const handleSave = () => {
    const configurationVariants = Object.keys(selectedVariants).length > 0 ? selectedVariants : undefined
    if (editingItemId) {
      updateItemConfiguration(editingItemId, product, selectedConfiguration, configurationVariants)
    } else {
      addToCart(product, selectedConfiguration, undefined, configurationVariants)
    }
    setSelectedConfiguration(initialConfiguration || {})
    setSelectedVariants(initialConfigurationVariants || {})
    onClose()
  }

  // Reset configuration when modal opens/closes
  React.useEffect(() => {
    if (isOpen) {
      setSelectedConfiguration(initialConfiguration || {})
      setSelectedVariants(initialConfigurationVariants || {})
    }
  }, [isOpen, initialConfiguration, initialConfigurationVariants])

  const isConfigurationComplete = () => {
    if (!product.bundle?.slots) return true

    return product.bundle!.slots.every((slot) => {
      const sel = selectedConfiguration[slot.id]
      if (sel === undefined) return false
      const item = slot.options?.find((it) => it.id === sel)
      if (item && hasVariants(item)) return selectedVariants[slot.id] !== undefined
      return true
    })
  }

  const isConfigurationValid = () => {
    if (!product.bundle?.slots) return true
    // Ensure selected products (and their variants) are still active
    return product.bundle!.slots.every((slot) => {
      const sel = selectedConfiguration[slot.id]
      if (!sel) return false
      const item = slot.options?.find((it) => it.id === sel)
      if (!item) return false
      // must be active and available
      const isActive = item.isActive !== false
      const isAvailable = item.isAvailable !== false
      if (!isActive || !isAvailable) return false
      if (hasVariants(item)) {
        const chosen = findVariant(item, selectedVariants[slot.id]?.id)
        if (!chosen || !chosen.isActive || chosen.isAvailable === false) return false
      }
      return true
    })
  }

  if (!product.bundle?.slots) {
    return null
  }

  return (
    <Dialog open={isOpen} onOpenChange={onClose}>
      <DialogContent className="max-h-[80vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="font-family-primary text-xl">Konfigurieren</DialogTitle>
        </DialogHeader>

        <div className="space-y-6">
          {[...product.bundle!.slots]
            .sort((a, b) => a.sequence - b.sequence)
            .map((slot) => (
              <BundleSlotSelector
                key={slot.id}
                slot={slot}
                selectedProductId={selectedConfiguration[slot.id]}
                selectedVariantId={selectedVariants[slot.id]?.id}
                onSelect={(productId) => handleSlotSelection(slot.id, productId)}
                onSelectVariant={(variant) => handleVariantSelection(slot.id, variant)}
                showDescription={!isPOS}
              />
            ))}
        </div>

        <DialogFooter className="flex-col gap-4 sm:flex-col">
          <div className="flex w-full gap-2">
            <Button
              onClick={handleSave}
              disabled={!isConfigurationComplete() || !isConfigurationValid()}
              className="rounded-pill h-12 flex-1 text-base font-medium"
            >
              {editingItemId ? "Im Warenkorb aktualisieren" : "Zum Warenkorb hinzufügen"}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

interface BundleSlotSelectorProps {
  slot: BundleSlotDTO
  selectedProductId?: string
  selectedVariantId?: string
  onSelect: (productId: string) => void
  onSelectVariant: (variant: { id: string; value: string }) => void
  showDescription?: boolean
}

function BundleSlotSelector({
  slot,
  selectedProductId,
  selectedVariantId,
  onSelect,
  onSelectVariant,
  showDescription = false,
}: BundleSlotSelectorProps) {
  if (!slot.options) {
    return null
  }

  return (
    <div className="space-y-3">
      <h3 className="font-family-secondary text-lg font-medium">{slot.name}</h3>

      <div className="grid gap-2">
        {slot.options.map((item) => {
          const isActive = item.isActive !== false
          const isAvailable = item.isAvailable !== false
          const isSelected = selectedProductId === item.id
          const isLowStock = item.isLowStock === true
          const qty = typeof item.availableQuantity === "number" ? item.availableQuantity : null
          if (!isActive) return null
          return (
            <Card
              key={item.id}
              className={`relative rounded-2xl py-0 transition-all hover:shadow-md ${
                isSelected ? "ring-primary bg-primary/5 ring-2" : "bg-muted/40 hover:bg-muted/70"
              } ${!isAvailable ? "pointer-events-none opacity-60 grayscale" : "cursor-pointer"}`}
              onClick={() => {
                if (isAvailable) onSelect(item.id)
              }}
              aria-disabled={!isAvailable}
            >
              {!isAvailable && (
                <span className="absolute top-2 right-2 z-10 rounded-full bg-red-600 px-2 py-0.5 text-xs font-medium text-white">
                  Ausverkauft
                </span>
              )}
              {isAvailable && isLowStock && (
                <span className="absolute top-2 right-2 z-10 rounded-full bg-amber-600 px-2 py-0.5 text-xs font-medium text-white">
                  {qty !== null ? `Nur ${qty} übrig` : "Geringer Bestand"}
                </span>
              )}
              <CardContent className="p-3">
                <div className="flex items-center gap-3">
                  {item.image && (
                    <div className="relative flex size-14 shrink-0 items-center justify-center rounded-xl bg-[#cec9c6]">
                      <div className="relative size-11 overflow-hidden">
                        <Image
                          src={item.image}
                          alt={item.name}
                          fill
                          sizes="44px"
                          quality={90}
                          className="h-full w-full object-cover"
                          unoptimized
                        />
                      </div>
                    </div>
                  )}
                  <div className="min-w-0 flex-1">
                    <h4 className="font-family-secondary font-medium">{item.name}</h4>
                    {showDescription && item.description && (
                      <p className="text-muted-foreground mt-0.5 line-clamp-2 text-xs whitespace-pre-line">
                        {item.description}
                      </p>
                    )}
                  </div>
                </div>
                {isSelected && hasVariants(item) && (
                  <div className="mt-2 flex flex-wrap gap-1.5" onClick={(e) => e.stopPropagation()}>
                    {activeVariants(item).map((variant) => {
                      const isVariantSelected = selectedVariantId === variant.id
                      const isOutOfStock = variant.isAvailable === false
                      return (
                        <button
                          key={variant.id}
                          type="button"
                          disabled={isOutOfStock}
                          onClick={() => onSelectVariant({ id: variant.id, value: variant.value })}
                          className={[
                            "rounded-full border px-3 py-1 text-xs font-medium transition-all",
                            isVariantSelected
                              ? "bg-primary text-primary-foreground border-transparent"
                              : "bg-card hover:bg-muted",
                            isOutOfStock ? "cursor-not-allowed line-through opacity-50" : "cursor-pointer",
                          ].join(" ")}
                          aria-pressed={isVariantSelected}
                        >
                          {variant.value}
                        </button>
                      )
                    })}
                  </div>
                )}
              </CardContent>
            </Card>
          )
        })}
      </div>
    </div>
  )
}
