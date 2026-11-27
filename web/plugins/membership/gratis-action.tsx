"use client"

import { ScanLine } from "lucide-react"
import { useState } from "react"
import { Button } from "@/components/ui/button"
import { getDeviceToken } from "@/lib/device-auth"
import type { SlotComponentProps } from "@/plugins/slots"
import type { MembershipLookupResult } from "./api"
import type { MembershipDiscount } from "./discount"
import { pluginLabel } from "./label"
import { MembershipScanDialog } from "./scan-dialog"

export default function MembershipGratisAction({ ctx, config }: SlotComponentProps<"pos.gratis.actions">) {
  const [open, setOpen] = useState(false)
  const token = getDeviceToken()
  const label = pluginLabel(config)
  const freeProductIds = (config.freeProductIds as string[] | undefined) ?? []

  if (!token) return null

  function handleApply(lookup: MembershipLookupResult, discount: MembershipDiscount) {
    setOpen(false)
    ctx.applyDiscount({
      pluginKey: "membership",
      label: `${label} · ${lookup.member?.name ?? "Mitglied"}`,
      amountCents: discount.totalDiscountCents,
      paymentData: {
        memberRef: lookup.memberRef,
        memberName: lookup.member?.name ?? "",
        quantity: discount.freeQuantity,
      },
    })
  }

  return (
    <>
      <Button
        className="flex h-28 flex-col items-center justify-center gap-2 rounded-xl"
        variant="outline"
        onClick={() => setOpen(true)}
        aria-label={label}
      >
        <ScanLine className="size-10" />
        <span className="text-base font-medium">{label}</span>
      </Button>
      <MembershipScanDialog
        open={open}
        onOpenChange={setOpen}
        onApply={handleApply}
        token={token}
        label={label}
        cartItems={ctx.cartItems}
        freeProductIds={freeProductIds}
      />
    </>
  )
}
