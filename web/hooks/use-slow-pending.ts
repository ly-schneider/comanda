"use client"

import { useEffect, useState } from "react"

// Delayed so instant cash refunds stay quiet; only a slow (~20s) Payrexx refund shows the hint.
export function useSlowPending(busy: boolean, delayMs = 2500): boolean {
  const [slow, setSlow] = useState(false)
  useEffect(() => {
    if (!busy) {
      setSlow(false)
      return
    }
    const t = setTimeout(() => setSlow(true), delayMs)
    return () => clearTimeout(t)
  }, [busy, delayMs])
  return slow
}
