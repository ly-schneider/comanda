"use client"

import { ScanLine } from "lucide-react"
import Link from "next/link"
import type { SlotComponentProps } from "@/plugins/slots"
import { pluginLabel } from "./label"

export default function MembershipHomeAction({ config }: SlotComponentProps<"shop.home.actions">) {
  const label = pluginLabel(config)

  return (
    <Link href="/mitglied" className="hover:bg-accent mb-4 flex items-center gap-3 rounded-2xl border px-4 py-3">
      <ScanLine className="size-6 shrink-0" />
      <span className="flex-1">
        <span className="block font-medium">{label}</span>
        <span className="text-muted-foreground text-sm">Karte scannen und Gratis-Artikel holen</span>
      </span>
    </Link>
  )
}
