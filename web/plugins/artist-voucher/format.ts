import type { ArtistVoucherBatch, ArtistVoucherBatchProduct } from "./api"

const dtf = new Intl.DateTimeFormat("de-CH", {
  day: "2-digit",
  month: "2-digit",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
})

export function fmtDateTime(iso: string): string {
  return dtf.format(new Date(iso))
}

export function entitlementText(products: ArtistVoucherBatchProduct[]): string {
  const groups = new Map<number, ArtistVoucherBatchProduct[]>()
  const parts: string[] = []

  for (const p of products) {
    const group = p.choiceGroup ?? 0
    if (group <= 0) {
      parts.push(`${p.quantity}× ${p.name}`)
      continue
    }
    const bucket = groups.get(group)
    if (bucket) bucket.push(p)
    else groups.set(group, [p])
  }

  for (const group of Array.from(groups.keys()).sort((a, b) => a - b)) {
    parts.push(
      groups
        .get(group)!
        .map((p) => `${p.quantity}× ${p.name}`)
        .join(" oder ")
    )
  }
  return parts.join(", ")
}

export function outstandingStockText(batch: ArtistVoucherBatch): string {
  const slips = `${batch.outstanding} ${batch.outstanding === 1 ? "Gutschein" : "Gutscheine"} offen`
  return [slips, ...batch.products.map((p) => `${p.stock} ${p.name} auf Lager`)].join(" · ")
}
