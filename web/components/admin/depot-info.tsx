"use client"

import { Info } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"

export function DepotInfo() {
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="text-muted-foreground hover:text-foreground size-9 shrink-0"
          aria-label="Was ist ein Depot?"
        >
          <Info className="size-4" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72 text-sm">
        <p className="font-medium">Depot</p>
        <p className="text-muted-foreground mt-1">
          Ein rückerstattbarer Pfand pro Stück – etwa für Mehrweggeschirr. Der Betrag wird der Kundschaft bei der
          Rückgabe zurückerstattet. Leer oder 0 heisst kein Depot.
        </p>
      </PopoverContent>
    </Popover>
  )
}
