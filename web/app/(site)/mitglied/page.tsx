import type { Metadata } from "next"
import { PluginSlot } from "@/components/plugins/plugin-slot"

export const metadata: Metadata = {
  title: "Mitgliederkarte",
  description: "Karte scannen und den Gratis-Artikel holen.",
}

export const dynamic = "force-dynamic"

// The page belongs to whichever plugin serves the slot; without one there is
// nothing to redeem here.
export default function MemberBenefitPage() {
  return (
    <PluginSlot
      name="shop.claim.page"
      ctx={{}}
      fallback={
        <div className="container mx-auto max-w-xl p-4">
          <h1 className="text-2xl font-semibold">Nicht verfügbar</h1>
          <p className="text-muted-foreground mt-2">Hier gibt es im Moment nichts einzulösen.</p>
        </div>
      }
    />
  )
}
