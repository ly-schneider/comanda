"use client"

import { useEffect, useState } from "react"

// Tracks navigator connectivity. Starts optimistic (`true`) and corrects on
// mount, since `navigator` isn't available during SSR.
export function useOnline(): boolean {
  const [online, setOnline] = useState(true)
  useEffect(() => {
    const update = () => setOnline(navigator.onLine)
    update()
    window.addEventListener("online", update)
    window.addEventListener("offline", update)
    return () => {
      window.removeEventListener("online", update)
      window.removeEventListener("offline", update)
    }
  }, [])
  return online
}
