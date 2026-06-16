"use client"

import { useCallback, useEffect, useState } from "react"
import { clearDeviceToken, getDeviceToken } from "@/lib/device-auth"
import { idbKVStore } from "@/lib/station/offline-db"
import { resolveStationSession, type StationStatus } from "@/lib/station/station-session"

export type StationSessionPhase =
  | { phase: "loading" }
  | { phase: "needs-pairing" }
  | { phase: "system-disabled" }
  | { phase: "offline-unprepared" }
  | { phase: "active"; status: StationStatus }

export interface StationSession {
  state: StationSessionPhase
  online: boolean
}

function useOnline(): boolean {
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

export function useStationSession(): StationSession {
  const online = useOnline()
  const [state, setState] = useState<StationSessionPhase>({ phase: "loading" })

  const resolve = useCallback(async () => {
    const result = await resolveStationSession({
      // Fresh each time: an authoritative rejection clears it, so the next
      // resolve falls into the no-token branch instead of re-hitting the network.
      token: getDeviceToken(),
      online: typeof navigator === "undefined" ? true : navigator.onLine,
      fetchFn: (...args) => fetch(...args),
      store: idbKVStore(),
      now: () => Date.now(),
    })
    switch (result.kind) {
      case "unauthorized":
        clearDeviceToken()
        setState({ phase: "needs-pairing" })
        break
      case "no-token":
        setState({ phase: "needs-pairing" })
        break
      case "system-disabled":
        setState({ phase: "system-disabled" })
        break
      case "offline-unprepared":
        setState({ phase: "offline-unprepared" })
        break
      case "resolved":
        setState({ phase: "active", status: result.status })
        break
    }
  }, [])

  useEffect(() => {
    void resolve()
  }, [resolve])

  // Re-resolve when connectivity returns to refresh the identity.
  useEffect(() => {
    if (!online) return
    void resolve()
  }, [online, resolve])

  return { state, online }
}
