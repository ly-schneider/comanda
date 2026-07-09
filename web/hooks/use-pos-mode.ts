"use client"

import { useEffect, useState } from "react"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import type { PosFulfillmentMode } from "@/types/jeton"

// Cached for the session so the sidebar and dialogs don't refetch on every mount.
// QR_CODE is the default, so jeton surfaces stay hidden until proven otherwise.
let cached: PosFulfillmentMode = "QR_CODE"
let loaded = false

export function usePosMode(): PosFulfillmentMode {
  const fetchAuth = useAuthorizedFetch()
  const [mode, setMode] = useState<PosFulfillmentMode>(cached)

  useEffect(() => {
    if (loaded) return
    let cancelled = false
    ;(async () => {
      try {
        const res = await fetchAuth("/api/v1/settings")
        if (!res.ok) return
        const j = (await res.json()) as { posMode?: PosFulfillmentMode }
        if (cancelled) return
        cached = j.posMode ?? "QR_CODE"
        loaded = true
        setMode(cached)
      } catch {
        // keep the QR_CODE default on failure
      }
    })()
    return () => {
      cancelled = true
    }
  }, [fetchAuth])

  return mode
}
