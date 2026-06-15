"use client"

import { useCallback, useEffect, useState } from "react"
import { idbKVStore } from "@/lib/station/offline-db"
import { getReadiness, type ReadinessRecord, runOfflineSetup } from "@/lib/station/offline-setup"
import { stationSWClient } from "@/lib/station/sw-client"
import { appEnvFromHost } from "@/lib/tenant"

export type ReadinessState =
  | "unsupported" // no service worker / not running as an installable PWA shell
  | "checking" // loading the persisted record
  | "unprepared" // never set up, or last setup did not complete
  | "preparing" // ritual in flight
  | "ready" // all assets cached for the active SW version
  | "stale" // cached for an older SW version — needs a re-run while online
  | "error" // last ritual failed

export interface OfflineReadiness {
  state: ReadinessState
  online: boolean
  record: ReadinessRecord | null
  prepare: () => void
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

export function useOfflineReadiness(): OfflineReadiness {
  const online = useOnline()
  const [state, setState] = useState<ReadinessState>("checking")
  const [record, setRecord] = useState<ReadinessRecord | null>(null)

  useEffect(() => {
    if (typeof navigator === "undefined" || !("serviceWorker" in navigator)) {
      setState("unsupported")
      return
    }
    // The SW is intentionally not registered in dev (see ServiceWorkerRegister),
    // so there is no offline shell to prepare there.
    if (appEnvFromHost(window.location.hostname) === "dev") {
      setState("unsupported")
      return
    }
    let cancelled = false
    ;(async () => {
      const stored = await getReadiness(idbKVStore()).catch(() => null)
      if (cancelled) return
      setRecord(stored)
      if (!stored || !stored.ready) {
        setState("unprepared")
        return
      }
      const liveVersion = await stationSWClient().version()
      if (cancelled) return
      // A version mismatch means the shell was re-cached under a new SW; the
      // stored assets may be gone, so prompt a re-run rather than lie.
      setState(stored.swVersion && liveVersion && stored.swVersion !== liveVersion ? "stale" : "ready")
    })()
    return () => {
      cancelled = true
    }
  }, [])

  const prepare = useCallback(() => {
    setState("preparing")
    runOfflineSetup()
      .then((result) => {
        setRecord(result)
        setState(result.ready ? "ready" : "error")
      })
      .catch(() => setState("error"))
  }, [])

  return { state, online, record, prepare }
}
