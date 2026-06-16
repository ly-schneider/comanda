"use client"

import { useEffect, useState } from "react"
import { clearDeviceToken, getDeviceToken } from "@/lib/device-auth"
import { idbKVStore } from "@/lib/station/offline-db"
import { resolveStationSession, type StationStatus } from "@/lib/station/station-session"
import { useOnline } from "@/lib/station/use-online"

export type StationSessionPhase =
  | { phase: "loading" }
  | { phase: "needs-pairing" }
  | { phase: "system-disabled" }
  | { phase: "offline-unprepared" }
  | { phase: "active"; status: StationStatus }

export interface StationSession {
  state: StationSessionPhase
  online: boolean
  token: string | null
}

export function useStationSession(): StationSession {
  const online = useOnline()
  const [state, setState] = useState<StationSessionPhase>({ phase: "loading" })
  const [token, setToken] = useState<string | null>(() => getDeviceToken())

  // A single resolve covers both mount and every connectivity change. The
  // `cancelled` guard drops a stale in-flight resolution so a slow request can't
  // overwrite a newer one when connectivity flaps.
  useEffect(() => {
    let cancelled = false
    void (async () => {
      // Re-read fresh: an authoritative rejection clears the token, so the next
      // resolve falls into the no-token branch instead of re-hitting the network.
      const current = getDeviceToken()
      setToken(current)
      const result = await resolveStationSession({
        token: current,
        online,
        fetchFn: (...args) => fetch(...args),
        store: idbKVStore(),
        now: () => Date.now(),
      })
      if (cancelled) return
      switch (result.kind) {
        case "unauthorized":
          clearDeviceToken()
          setToken(null)
          setState({ phase: "needs-pairing" })
          break
        case "no-token":
          setToken(null)
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
    })()
    return () => {
      cancelled = true
    }
  }, [online])

  return { state, online, token }
}
