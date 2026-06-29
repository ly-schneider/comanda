import { formatChf } from "@/lib/utils"

export interface CartTotalSummaryProps {
  productCents: number
  depositCents: number
  itemCount: number
}

export function CartTotalSummary({ productCents, depositCents, itemCount }: CartTotalSummaryProps) {
  return (
    <div className="flex flex-col gap-1.5">
      {depositCents > 0 && (
        <>
          <div className="text-muted-foreground flex items-center justify-between text-sm">
            <span>Zwischensumme</span>
            <span>{formatChf(productCents)}</span>
          </div>
          <div className="text-muted-foreground flex items-center justify-between text-sm">
            <span>Depot</span>
            <span>{formatChf(depositCents)}</span>
          </div>
        </>
      )}
      <div className="flex items-center justify-between">
        <div className="flex flex-col">
          <p className="text-lg font-semibold">Total</p>
          <span className="text-muted-foreground text-sm">{itemCount} Produkte</span>
        </div>
        <p className="text-lg font-semibold">{formatChf(productCents + depositCents)}</p>
      </div>
    </div>
  )
}
