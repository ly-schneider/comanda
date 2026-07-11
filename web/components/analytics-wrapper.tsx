"use client"

import { usePathname } from "next/navigation"
import Script from "next/script"
import { useEffect, useState } from "react"
import { appEnvFromHost } from "@/lib/tenant"

const UMAMI_WEBSITE_ID = "3090be94-b431-4787-b053-904fc918e899"
const EXCLUDED_PATHS = ["/pos", "/station"]

export default function AnalyticsWrapper() {
  const pathname = usePathname()
  const [isProd, setIsProd] = useState(false)

  useEffect(() => {
    setIsProd(appEnvFromHost(window.location.hostname) === "prod")
  }, [])

  if (!isProd || EXCLUDED_PATHS.some((path) => pathname.startsWith(path))) {
    return null
  }

  return <Script defer src="https://cloud.umami.is/script.js" data-website-id={UMAMI_WEBSITE_ID} />
}
