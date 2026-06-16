"use client"

import { useCallback, useEffect, useState } from "react"
import { idbKVStore } from "@/lib/station/offline-db"
import { getReadiness, runOfflineSetup } from "@/lib/station/offline-setup"
import { stationSWClient } from "@/lib/station/sw-client"
import { useOnline } from "@/lib/station/use-online"
import { appEnvFromHost } from "@/lib/tenant"

export type ReadinessState =
  | "unsupported" // no service worker / not running as an installable PWA shell
  | "checking" // loading the persisted record
  | "unprepared" // never set up, or last setup did not complete
  | "preparing" // ritual in flight
  | "ready" // all asserted assets confirmed present in the live cache
  | "stale" // assets missing from the live cache (e.g. wiped by an SW upgrade) — re-run while online
  | "error" // last ritual failed

export interface OfflineReadiness {
  state: ReadinessState
  online: boolean
  prepare: () => void
}

export function useOfflineReadiness(): OfflineReadiness {
  const online = useOnline()
  const [state, setState] = useState<ReadinessState>("checking")

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
      if (!stored || !stored.ready) {
        setState("unprepared")
        return
      }
      // Cache Storage is wiped on every SW version bump while this record
      // survives in IndexedDB, so confirm the asserted assets are still cached
      // rather than trusting the stored flag. If no worker can answer, we can't
      // serve offline anyway — prompt a re-run instead of claiming readiness.
      const status = await stationSWClient()
        .cacheStatus(stored.assets ?? [])
        .catch(() => null)
      if (cancelled) return
      setState(!status || status.missing.length > 0 ? "stale" : "ready")
    })()
    return () => {
      cancelled = true
    }
  }, [])

  const prepare = useCallback(() => {
    setState("preparing")
    runOfflineSetup()
      .then((result) => setState(result.ready ? "ready" : "error"))
      .catch(() => setState("error"))
  }, [])

  return { state, online, prepare }
}
