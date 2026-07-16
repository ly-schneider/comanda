"use client"

import { useCallback, useState } from "react"
import { matchesProductFilter } from "@/lib/categories"

export function useProductFilter() {
  const [query, setQuery] = useState("")
  const [activeCat, setActiveCat] = useState("all")
  const [activeSub, setActiveSub] = useState("all")

  const selectCat = useCallback((value: string) => {
    setActiveCat(value)
    setActiveSub("all")
  }, [])

  const matches = useCallback(
    (p: { name: string; category?: { id?: string; parentId?: string | null } | null }) =>
      matchesProductFilter(p, { query, activeCat, activeSub }),
    [query, activeCat, activeSub]
  )

  return { query, setQuery, activeCat, activeSub, selectCat, setActiveSub, matches }
}
