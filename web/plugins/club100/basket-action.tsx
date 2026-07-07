"use client"

import { useState } from "react"
import { Button } from "@/components/ui/button"
import { getDeviceToken } from "@/lib/device-auth"
import type { SlotComponentProps } from "@/plugins/slots"
import type { Club100Person } from "./api"
import { type Club100Discount, Club100PickerDialog } from "./picker-dialog"

export default function Club100BasketAction({ ctx, config }: SlotComponentProps<"pos.basket.actions">) {
  const [open, setOpen] = useState(false)
  const token = getDeviceToken()
  const freeProductIds = (config.freeProductIds as string[] | undefined) ?? []

  if (!token) return null

  function handleSelect(person: Club100Person, discount: Club100Discount | null) {
    setOpen(false)
    if (!discount || discount.discountedItems.length === 0) {
      ctx.setPluginDiscount(null)
      return
    }
    const freeQuantity = discount.discountedItems.reduce((sum, d) => sum + d.discountedQuantity, 0)
    ctx.setPluginDiscount({
      pluginKey: "club100",
      label: `100 Club · ${discount.person.firstName} ${discount.person.lastName}`,
      amountCents: discount.totalDiscountCents,
      paymentData: {
        elvantoPersonId: discount.person.id,
        elvantoPersonName: `${discount.person.firstName} ${discount.person.lastName}`,
        freeQuantity,
      },
    })
  }

  return (
    <>
      <Button variant="outline" className="h-11 w-full rounded-xl text-sm" onClick={() => setOpen(true)}>
        100 Club Rabatt
      </Button>
      <Club100PickerDialog
        open={open}
        onOpenChange={setOpen}
        onSelect={handleSelect}
        token={token}
        cartItems={ctx.cartItems}
        freeProductIds={freeProductIds}
      />
    </>
  )
}
