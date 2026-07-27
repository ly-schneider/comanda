"use client"

import { CalendarIcon } from "lucide-react"
import { useState } from "react"
import type { DateRange } from "react-day-picker"
import { Button } from "@/components/ui/button"
import { Calendar } from "@/components/ui/calendar"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"

function formatDay(date: Date) {
  return date.toLocaleDateString("de-CH", { day: "2-digit", month: "2-digit", year: "numeric" })
}

function label(range: DateRange | undefined) {
  if (!range?.from) return "Zeitraum wählen"
  if (!range.to || range.to.getTime() === range.from.getTime()) return formatDay(range.from)
  return `${formatDay(range.from)} – ${formatDay(range.to)}`
}

export function DateRangePicker({
  value,
  onChange,
  id,
}: {
  value: DateRange | undefined
  onChange: (range: DateRange | undefined) => void
  id?: string
}) {
  const [open, setOpen] = useState(false)

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button id={id} variant="outline" className="w-full justify-start font-normal">
          <CalendarIcon className="mr-2 size-4" />
          {label(value)}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-auto p-0" align="start">
        <Calendar
          mode="range"
          numberOfMonths={2}
          defaultMonth={value?.from}
          selected={value}
          onSelect={onChange}
          weekStartsOn={1}
        />
      </PopoverContent>
    </Popover>
  )
}
