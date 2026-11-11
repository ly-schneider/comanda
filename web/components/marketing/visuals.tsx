import { Globe } from "lucide-react"
import QRCode from "@/components/qrcode"
import { cn } from "@/lib/utils"

const shadow = "shadow-[0_10px_28px_rgba(0,0,0,0.08)]"

function Stage({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <div
      aria-hidden
      className={cn(
        "bg-secondary flex h-[340px] w-full items-center justify-center overflow-hidden rounded-[var(--radius-surface)] select-none sm:aspect-[588/380] sm:h-auto",
        className
      )}
    >
      {children}
    </div>
  )
}

function Pill({ children, tone }: { children: React.ReactNode; tone: "success" | "destructive" }) {
  return (
    <span
      className={cn(
        "rounded-pill px-2.5 py-0.5 text-xs font-medium",
        tone === "success" ? "bg-success/10 text-success" : "bg-destructive/10 text-destructive"
      )}
    >
      {children}
    </span>
  )
}

export function OrderingVisual() {
  const items = [
    ["Cappuccino", "4.50"],
    ["Croissant", "3.50"],
    ["Glace", "5.00"],
    ["Eistee", "4.00"],
  ]
  return (
    <Stage className="items-start pt-8">
      <div className={cn("bg-background w-[280px] rounded-t-[var(--radius-surface)] p-5", shadow)}>
        <p className="text-[15px] font-semibold">Kaffeebar</p>
        <p className="text-muted-foreground text-[13px]">Bestellen und am Fenster abholen</p>
        <div className="mt-3.5 grid grid-cols-2 gap-2">
          {items.map(([name, price]) => (
            <div key={name} className="bg-secondary rounded-[var(--radius-control)] p-3">
              <div className="bg-chart-4 h-9 rounded-md" />
              <p className="mt-2 text-[13px] font-medium">{name}</p>
              <p className="text-muted-foreground text-[13px]">CHF {price}</p>
            </div>
          ))}
        </div>
        <div className="bg-primary text-primary-foreground rounded-pill mt-3.5 flex justify-between px-4 py-3 text-[13px] font-medium">
          <span>2 Artikel</span>
          <span>Weiter · CHF 8.00</span>
        </div>
      </div>
    </Stage>
  )
}

export function PickupVisual() {
  return (
    <Stage>
      <div
        className={cn(
          "bg-background flex w-[280px] flex-col items-center gap-3 rounded-[var(--radius-surface)] p-6",
          shadow
        )}
      >
        <Pill tone="success">Bereit zur Abholung</Pill>
        <p className="text-[22px] leading-7 font-semibold tracking-tight">Bestellung 142</p>
        <QRCode value="https://comanda.leys.ch" size={130} />
        <p className="text-muted-foreground text-center text-[13px]">Zeig diesen Code am Ausgabefenster.</p>
      </div>
    </Stage>
  )
}

export function TillVisual() {
  const tiles = [
    ["Burger", "14.00"],
    ["Pommes", "6.00"],
    ["Glace", "5.00"],
    ["Cappuccino", "4.50"],
    ["Espresso", "3.50"],
    ["Wasser", "3.00"],
    ["Eistee", "4.00"],
    ["Croissant", "3.50"],
    ["Becher", "2.00"],
  ]
  return (
    <Stage>
      <div
        className={cn(
          "border-border bg-background flex shrink-0 gap-3 rounded-[var(--radius-surface)] border-[6px] p-3.5 max-sm:scale-[0.7]",
          shadow
        )}
      >
        <div className="grid grid-cols-3 gap-2">
          {tiles.map(([name, price]) => (
            <div key={name} className="bg-secondary w-[92px] rounded-[var(--radius-control)] px-2.5 pt-7 pb-2.5">
              <p className="truncate text-xs font-medium">{name}</p>
              <p className="text-muted-foreground text-xs">{price}</p>
            </div>
          ))}
        </div>
        <div className="bg-sidebar flex w-[150px] flex-col gap-2 rounded-[var(--radius-control)] px-3.5 py-3 text-xs whitespace-nowrap">
          <p className="font-semibold">Warenkorb</p>
          <p className="text-muted-foreground flex justify-between">
            <span>2× Burger</span>
            <span>28.00</span>
          </p>
          <p className="text-muted-foreground flex justify-between">
            <span>1× Pommes</span>
            <span>6.00</span>
          </p>
          <p className="mt-auto flex justify-between font-semibold">
            <span>Total</span>
            <span>34.00</span>
          </p>
          <p className="bg-primary text-primary-foreground rounded-[var(--radius-control)] py-2 text-center font-medium">
            Kassieren
          </p>
        </div>
      </div>
    </Stage>
  )
}

export function StockVisual() {
  const rows: [string, string, boolean][] = [
    ["Burger", "Ausverkauft", true],
    ["Veggie-Burger", "Ausverkauft", true],
    ["Glace", "12 übrig", false],
    ["Pommes", "40 übrig", false],
    ["Cappuccino", "85 übrig", false],
  ]
  return (
    <Stage>
      <div className={cn("bg-background w-[340px] rounded-[var(--radius-surface)] px-6 pt-5.5 pb-3", shadow)}>
        <div className="flex justify-between pb-3">
          <p className="text-[15px] font-semibold">Sortiment</p>
          <p className="text-muted-foreground text-[13px]">Samstag, 12:10</p>
        </div>
        {rows.map(([name, state, soldOut]) => (
          <div key={name} className="border-border flex items-center justify-between border-t py-3 text-[13px]">
            <span className="font-medium">{name}</span>
            {soldOut ? <Pill tone="destructive">{state}</Pill> : <span className="text-muted-foreground">{state}</span>}
          </div>
        ))}
      </div>
    </Stage>
  )
}

const EXAMPLE_BRAND = ["#e4572e", "#ffc914", "#2e282a"]

export function BrandingVisual() {
  return (
    <Stage className="flex-col gap-5">
      <div
        className={cn(
          "bg-background rounded-pill flex items-center gap-2 py-2 pr-4 pl-3.5 text-[13px] font-medium",
          shadow
        )}
      >
        <Globe className="text-muted-foreground size-3.5" />
        bestellen.sommerfest.ch
      </div>
      <div className={cn("bg-background flex w-[320px] flex-col gap-4 rounded-[var(--radius-surface)] p-6", shadow)}>
        <div className="flex items-center gap-3">
          <span className="size-10 rounded-full" style={{ backgroundColor: EXAMPLE_BRAND[0] }} />
          <div>
            <p className="text-[15px] font-semibold">Sommerfest</p>
            <p className="text-muted-foreground text-[13px]">Grill und Bar</p>
          </div>
        </div>
        <div className="flex gap-2">
          {EXAMPLE_BRAND.map((color) => (
            <span key={color} className="h-8 w-14 rounded-lg" style={{ backgroundColor: color }} />
          ))}
        </div>
        <p
          className="rounded-pill py-3 text-center text-[13px] font-medium text-white"
          style={{ backgroundColor: EXAMPLE_BRAND[0] }}
        >
          Jetzt bestellen
        </p>
      </div>
    </Stage>
  )
}
