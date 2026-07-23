"use client"

import { useState } from "react"
import { Bar, BarChart, XAxis, YAxis } from "recharts"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { type ChartConfig, ChartContainer, ChartTooltip } from "@/components/ui/chart"
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog"
import { ScrollArea } from "@/components/ui/scroll-area"
import { formatChf } from "@/lib/utils"

type ProductData = { title: string; value: number }

type TopProductsChartProps = {
  title: string
  data: ProductData[]
  loading: boolean
  formatAsCurrency?: boolean
}

const TOP_N = 15

export function TopProductsChart({ title, data, loading, formatAsCurrency }: TopProductsChartProps) {
  const chartConfig = {
    value: {
      label: formatAsCurrency ? "Umsatz" : "Menge",
      color: formatAsCurrency ? "var(--chart-3)" : "var(--chart-2)",
    },
  } satisfies ChartConfig

  const formatValue = (v: number) => (formatAsCurrency ? formatChf(v) : `${v}×`)

  const topData = data.slice(0, TOP_N)
  const chartHeight = Math.max(200, topData.length * 32 + 40)

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-2">
        <CardTitle className="text-base">{title}</CardTitle>
        {!loading && data.length > 0 && <AllProductsDialog title={title} data={data} formatValue={formatValue} />}
      </CardHeader>
      <CardContent>
        {loading ? (
          <div className="text-muted-foreground flex h-[250px] items-center justify-center text-sm">Lade…</div>
        ) : data.length === 0 ? (
          <div className="text-muted-foreground flex h-[250px] items-center justify-center text-sm">
            Keine Daten vorhanden.
          </div>
        ) : (
          <ChartContainer config={chartConfig} className="w-full" style={{ height: chartHeight }}>
            <BarChart data={topData} layout="vertical" margin={{ top: 0, right: 4, bottom: 0, left: 0 }}>
              <XAxis
                type="number"
                tickLine={false}
                axisLine={false}
                fontSize={12}
                tickFormatter={formatAsCurrency ? (v: number) => `${Math.round(v / 100)}` : undefined}
              />
              <YAxis
                type="category"
                dataKey="title"
                tickLine={false}
                axisLine={false}
                fontSize={12}
                width={120}
                tickFormatter={(v: string) => (v.length > 16 ? v.slice(0, 14) + "…" : v)}
              />
              <ChartTooltip
                content={({ active, payload }) => {
                  if (!active || !payload?.length || !payload[0]) return null
                  const d = payload[0].payload as ProductData
                  return (
                    <div className="border-border/50 bg-background rounded-lg border px-3 py-2 text-xs shadow-xl">
                      <div className="font-medium">{d.title}</div>
                      <div className="text-muted-foreground">{formatValue(d.value)}</div>
                    </div>
                  )
                }}
              />
              <Bar dataKey="value" fill="var(--color-value)" radius={[0, 4, 4, 0]} />
            </BarChart>
          </ChartContainer>
        )}
      </CardContent>
    </Card>
  )
}

function AllProductsDialog({
  title,
  data,
  formatValue,
}: {
  title: string
  data: ProductData[]
  formatValue: (v: number) => string
}) {
  const [open, setOpen] = useState(false)

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm">
          Alle anzeigen ({data.length})
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <ScrollArea className="max-h-[60vh] pr-4">
          <ol className="divide-border divide-y text-sm">
            {data.map((d, i) => (
              <li key={d.title} className="flex items-center justify-between gap-4 py-2">
                <span className="flex min-w-0 items-baseline gap-2">
                  <span className="text-muted-foreground w-6 shrink-0 tabular-nums">{i + 1}.</span>
                  <span className="truncate">{d.title}</span>
                </span>
                <span className="shrink-0 font-medium tabular-nums">{formatValue(d.value)}</span>
              </li>
            ))}
          </ol>
        </ScrollArea>
      </DialogContent>
    </Dialog>
  )
}
