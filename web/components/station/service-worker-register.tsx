"use client"

import { useEffect } from "react"
import { STATION_SW_SCOPE } from "@/lib/station/sw-client"
import { appEnvFromHost } from "@/lib/tenant"

export function ServiceWorkerRegister() {
  useEffect(() => {
    if (appEnvFromHost(window.location.hostname) === "dev") return
    if (!("serviceWorker" in navigator)) return

    const register = () => {
      navigator.serviceWorker.register("/station-sw.js", { scope: STATION_SW_SCOPE }).catch(() => {})
    }

    if (document.readyState === "complete") register()
    else window.addEventListener("load", register, { once: true })
  }, [])

  return null
}
