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
  return products.map((p) => `${p.quantity}× ${p.name}`).join(", ")
}

export function outstandingStockText(batch: ArtistVoucherBatch): string {
  const slips = `${batch.outstanding} ${batch.outstanding === 1 ? "Gutschein" : "Gutscheine"} offen`
  return [slips, ...batch.products.map((p) => `${p.stock} ${p.name} auf Lager`)].join(" · ")
}
