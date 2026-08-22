"use client"

import { Ticket } from "lucide-react"
import { useState } from "react"
import { Button } from "@/components/ui/button"
import { getDeviceToken } from "@/lib/device-auth"
import type { SlotComponentProps } from "@/plugins/slots"
import type { Club100Person } from "./api"
import type { Club100Discount } from "./discount"
import { Club100PickerDialog } from "./picker-dialog"

export default function Club100GratisAction({ ctx, config }: SlotComponentProps<"pos.gratis.actions">) {
  const [open, setOpen] = useState(false)
  const token = getDeviceToken()
  const freeProductIds = (config.freeProductIds as string[] | undefined) ?? []

  if (!token) return null

  function handleSelect(person: Club100Person, discount: Club100Discount | null) {
    setOpen(false)
    if (!discount || discount.discountedItems.length === 0) {
      ctx.applyDiscount(null)
      return
    }
    const freeQuantity = discount.discountedItems.reduce((sum, d) => sum + d.discountedQuantity, 0)
    ctx.applyDiscount({
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
      <Button
        className="flex h-28 flex-col items-center justify-center gap-2 rounded-xl"
        variant="outline"
        onClick={() => setOpen(true)}
        aria-label="100 Club"
      >
        <Ticket className="size-10" />
        <span className="text-base font-medium">100 Club</span>
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
