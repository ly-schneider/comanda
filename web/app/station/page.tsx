"use client"

import { AlertCircle, Ban, CheckCircle2, Clock, Info, Loader2, PackageOpen, ScanLine, XCircle } from "lucide-react"
import Image from "next/image"
import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { DeviceLogoutButton } from "@/components/device/device-logout-button"
import { PairingCodeDisplay } from "@/components/device/pairing-code-display"
import { PluginSlot } from "@/components/plugins/plugin-slot"
import QrScanner, { type QrScannerControls, type QrScanResult } from "@/components/qr-scanner"
import { FullscreenButton } from "@/components/station/fullscreen-button"
import { HardRefreshButton } from "@/components/station/hard-refresh-button"
import { InstallHintDrawer } from "@/components/station/install-hint-drawer"
import { InstallPrompt } from "@/components/station/install-prompt"
import { OfflineReadiness } from "@/components/station/offline-readiness"
import { OfflineSetupRequired } from "@/components/station/offline-setup-required"
import { StationProductManager } from "@/components/station/product-manager"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { useBranding } from "@/contexts/branding-context"
import { useDeviceFetch } from "@/hooks/use-device-fetch"

import { playScanSound, primeScanAudio } from "@/lib/scan-sound"
import { idbLedger, type StationLedger } from "@/lib/station/ledger"
import { idbKVStore } from "@/lib/station/offline-db"
import {
  CATALOG_KEY,
  type CatalogEntry,
  PLUGINS_KEY,
  QR_CONFIG_KEY,
  type QRConfig,
  refreshCatalog,
  runOfflineSetup,
} from "@/lib/station/offline-setup"
import { verifyQrToken } from "@/lib/station/qr-verify"
import { type CatalogProduct, type RedeemLineView, redeemScan } from "@/lib/station/redeem"
import {
  EMPTY_REVOCATIONS,
  loadRevocations,
  reconcileRevocations,
  revokedIdSet,
  type ServerRevocations,
  syncedLabel,
} from "@/lib/station/revocations"
import { flushOutbox } from "@/lib/station/sync"
import { idbOutbox, type StationOutbox } from "@/lib/station/sync-queue"
import { useStationSession } from "@/lib/station/use-station-session"
import { ARTIST_VOUCHER_PLUGIN_KEY, evaluateVoucher, type VoucherVerdict } from "@/lib/station/voucher"
import { parseScan } from "@/lib/station-scan"

const SCAN_RESUME_DELAY_MS = 1400
const DUPLICATE_SCAN_WINDOW_MS = 5000
const HISTORY_LIMIT = 8
const CATALOG_REFRESH_INTERVAL_MS = 5 * 60_000

type OrderLine = {
  id: string
  orderId: string
  productId: string
  lineType?: string
  title: string
  quantity: number
  unitPriceCents: number
  productImage?: string | null
  redemption?: { id: string; redeemedAt: string } | null
  parentLineId?: string | null
  bundleSlotId?: string | null
  bundleSlotName?: string | null
  variantId?: string | null
  variantValue?: string | null
  childLines?: OrderLine[]
}

type ScanStatus = "loading" | "success" | "already-redeemed" | "no-items" | "error"

type OrderScanResult = {
  key: string
  scanId: string
  kind: "order" | "campaign"
  status: ScanStatus
  message?: string
  scannedAt: Date
  orderId?: string
  lines: OrderLine[]
}

type VoucherVerdictKind = VoucherVerdict["kind"]

type VoucherScanResult = {
  key: string
  scanId: string
  kind: "voucher"
  verdict: VoucherVerdictKind
  message?: string
  scannedAt: Date
  voucherId?: string
  label?: string
  categories?: string[]
  validFrom?: number // Unix seconds
  validUntil?: number // Unix seconds
  syncedAt: number | null
}

type ScanResult = OrderScanResult | VoucherScanResult

type CampaignChoiceOption = {
  productId: string
  productName: string
  productImage?: string | null
  quantity: number
}

type CampaignChoiceGroup = { choiceGroup: number; options: CampaignChoiceOption[] }

type CampaignChoiceSelection = { choiceGroup: number; productId: string }

// A campaign with alternatives ("Pommes ODER Getränk") is resolved here: the
// server rejects the first redemption with the open groups, the operator picks,
// and the same redemption is re-sent.
type ChoicePrompt = {
  scanKey: string
  claimToken: string
  idem: string
  groups: CampaignChoiceGroup[]
}

type LineGroup = { key: string; parent: OrderLine; children: OrderLine[] }

function groupLines(lines: OrderLine[], bundleParents?: Record<string, OrderLine>): LineGroup[] {
  const groups: LineGroup[] = []
  const groupByRootId: Record<string, LineGroup> = {}
  for (const l of lines) {
    const parentLine = l.parentLineId && bundleParents ? bundleParents[l.parentLineId] : null
    if (parentLine) {
      let g = groupByRootId[parentLine.id]
      if (!g) {
        g = { key: parentLine.id, parent: parentLine, children: [] }
        groupByRootId[parentLine.id] = g
        groups.push(g)
      }
      g.children.push(l)
    } else {
      groups.push({ key: l.id, parent: l, children: [] })
    }
  }
  return groups
}

function groupRedemptionTime(group: LineGroup): string | undefined {
  if (group.parent.redemption?.redeemedAt) return group.parent.redemption.redeemedAt
  return group.children.find((c) => c.redemption?.redeemedAt)?.redemption?.redeemedAt
}

type ConsolidatedItem = {
  productId: string
  variantId?: string | null
  variantValue?: string | null
  title: string
  productImage?: string | null
  quantity: number
  redeemed: boolean
  redemptionTime?: string
}

function consolidateLines(lines: OrderLine[]): ConsolidatedItem[] {
  const map = new Map<string, ConsolidatedItem>()
  for (const l of lines) {
    const key = `${l.productId}:${l.variantId ?? ""}`
    const existing = map.get(key)
    if (existing) {
      existing.quantity += l.quantity
      if (!l.redemption) existing.redeemed = false
      const t = l.redemption?.redeemedAt
      if (t && (!existing.redemptionTime || t > existing.redemptionTime)) {
        existing.redemptionTime = t
      }
    } else {
      map.set(key, {
        productId: l.productId,
        variantId: l.variantId,
        variantValue: l.variantValue,
        title: l.title,
        productImage: l.productImage,
        quantity: l.quantity,
        redeemed: !!l.redemption,
        redemptionTime: l.redemption?.redeemedAt,
      })
    }
  }
  return Array.from(map.values())
}

type StatusMeta = {
  label: string
  text: string
  bg: string
  dot: string
  icon: typeof CheckCircle2
}

const STATUS_STYLES: Record<ScanStatus, StatusMeta> = {
  loading: {
    label: "Wird geprüft",
    text: "text-muted-foreground",
    bg: "bg-card",
    dot: "bg-muted-foreground/50",
    icon: Loader2,
  },
  success: {
    label: "Eingelöst",
    text: "text-success",
    bg: "bg-success/10",
    dot: "bg-success",
    icon: CheckCircle2,
  },
  "already-redeemed": {
    label: "Bereits eingelöst",
    text: "text-destructive",
    bg: "bg-destructive/10",
    dot: "bg-destructive",
    icon: AlertCircle,
  },
  "no-items": {
    label: "Keine Artikel",
    text: "text-muted-foreground",
    bg: "bg-card",
    dot: "bg-muted-foreground/50",
    icon: PackageOpen,
  },
  error: {
    label: "Fehler",
    text: "text-destructive",
    bg: "bg-destructive/10",
    dot: "bg-destructive",
    icon: XCircle,
  },
}

type VoucherMeta = { label: string; text: string; bg: string; dot: string; icon: typeof CheckCircle2 }

const VOUCHER_STYLES: Record<VoucherVerdictKind, VoucherMeta> = {
  valid: { label: "Gültig", text: "text-success", bg: "bg-success/10", dot: "bg-success", icon: CheckCircle2 },
  scheduled: {
    label: "Noch nicht gültig",
    text: "text-muted-foreground",
    bg: "bg-card",
    dot: "bg-muted-foreground/50",
    icon: Clock,
  },
  expired: {
    label: "Abgelaufen",
    text: "text-destructive",
    bg: "bg-destructive/10",
    dot: "bg-destructive",
    icon: AlertCircle,
  },
  revoked: { label: "Gesperrt", text: "text-destructive", bg: "bg-destructive/10", dot: "bg-destructive", icon: Ban },
  invalid: {
    label: "Ungültig",
    text: "text-destructive",
    bg: "bg-destructive/10",
    dot: "bg-destructive",
    icon: XCircle,
  },
}

function voucherInvalidMessage(verdict: VoucherVerdict): string | undefined {
  if (verdict.kind !== "invalid") return undefined
  switch (verdict.reason) {
    case "not-ready":
      return "Offline-Einrichtung erforderlich"
    case "wrong-tenant":
      return "Falscher Event"
    default:
      return "Ungültiger Code"
  }
}

function voucherScanResult(
  key: string,
  token: string,
  verdict: VoucherVerdict,
  syncedAt: number | null
): VoucherScanResult {
  const base: VoucherScanResult = {
    key,
    scanId: token,
    kind: "voucher",
    verdict: verdict.kind,
    scannedAt: new Date(),
    syncedAt,
  }
  if (verdict.kind === "invalid") return { ...base, message: voucherInvalidMessage(verdict) }
  const v = verdict.voucher
  return {
    ...base,
    voucherId: v.voucherId,
    label: v.label,
    categories: v.categories,
    validFrom: v.validFrom,
    validUntil: v.validUntil,
  }
}

function voucherSound(kind: VoucherVerdictKind): "success" | "warning" | "error" {
  if (kind === "valid") return "success"
  if (kind === "invalid") return "error"
  return "warning"
}

function formatVoucherTime(unixSec: number): string {
  return new Date(unixSec * 1000).toLocaleString("de-CH", { dateStyle: "medium", timeStyle: "short" })
}

export default function StationPage() {
  const { state: sessionState, online, token, applyProducts } = useStationSession()
  const { displayName, logoUrl } = useBranding()
  const status = sessionState.phase === "active" ? sessionState.status : null
  const approved = status?.status === "approved"
  const canSelfManageProducts = status?.selfManageProducts === true
  // Keyed on the stable id list so a resolve that returns a new `status` object
  // with the same assignments doesn't rebuild the set (which would churn the
  // scan callbacks and restart the camera). Product ids are comma-free nanoids.
  const assignedProductKey = (status?.products ?? []).map((p) => p.productId).join(",")
  const assignedProductIds = useMemo(
    () => new Set(assignedProductKey ? assignedProductKey.split(",") : []),
    [assignedProductKey]
  )
  const assignedProductIdList = useMemo(() => Array.from(assignedProductIds), [assignedProductIds])
  const systemDisabled = sessionState.phase === "system-disabled"

  // Bumped after a save so the offline-replay effect re-reads the catalog cache.
  const [catalogVersion, setCatalogVersion] = useState(0)

  // Apply the new set instantly (live id-filter), then refresh the offline
  // caches + re-replay the catalog so a just-added product renders offline too.
  const handleProductsSaved = useCallback(
    (products: { productId: string; name?: string }[]) => {
      applyProducts(products)
      void runOfflineSetup()
        .catch(() => {})
        .finally(() => setCatalogVersion((v) => v + 1))
    },
    [applyProducts]
  )
  const [error, setError] = useState<string | null>(null)
  const [scans, setScans] = useState<ScanResult[]>([])
  const [detailScan, setDetailScan] = useState<OrderScanResult | null>(null)
  const [choicePrompt, setChoicePrompt] = useState<ChoicePrompt | null>(null)
  const [cameraPermission, setCameraPermission] = useState<"idle" | "pending" | "granted" | "denied">("idle")
  const [scannerActive, setScannerActive] = useState(false)
  const [scannerKey, setScannerKey] = useState(0)
  const scannerRef = useRef<QrScannerControls | null>(null)
  const scanLockedRef = useRef(false)
  const recentScanIdsRef = useRef<Map<string, number>>(new Map())
  const resumeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const bearerToken = token ?? ""

  // Verification material + catalog and the connectivity flag are read through
  // refs, not deps, so loading them never changes the scan-callback identity
  // (which would restart the camera).
  const offlineRef = useRef<{
    publicKey: string | null
    tenantId: string | null
    catalog: Map<string, CatalogProduct>
    voucherEnabled: boolean
  }>({ publicKey: null, tenantId: null, catalog: new Map(), voucherEnabled: false })
  // Read on the scan hot path through a ref (stable identity → no camera restart).
  const revokedIdsRef = useRef<Set<string>>(new Set())
  const revocationsRef = useRef<ServerRevocations>(EMPTY_REVOCATIONS)
  const applyRevocations = useCallback((next: ServerRevocations) => {
    revocationsRef.current = next
    revokedIdsRef.current = revokedIdSet(next)
  }, [])
  const ledgerRef = useRef<StationLedger | null>(null)
  if (!ledgerRef.current) ledgerRef.current = idbLedger()
  const outboxRef = useRef<StationOutbox | null>(null)
  if (!outboxRef.current) outboxRef.current = idbOutbox()
  // Read through a ref so a token refresh never rebuilds the scan callbacks
  // (which would restart the camera); the sync flush reads the latest value.
  const tokenRef = useRef(bearerToken)
  useEffect(() => {
    tokenRef.current = bearerToken
  }, [bearerToken])
  // Same ref trick for the device-fetch wrapper: triggerFlush stays stable, but a
  // flush still routes 401/403 through clear-token-and-redirect to /station.
  const deviceFetch = useDeviceFetch("station")
  const deviceFetchRef = useRef(deviceFetch)
  useEffect(() => {
    deviceFetchRef.current = deviceFetch
  }, [deviceFetch])

  const triggerReconcile = useCallback(() => {
    if (typeof navigator !== "undefined" && !navigator.onLine) return
    if (!offlineRef.current.voucherEnabled) return
    void reconcileRevocations({
      fetchFn: (input, init) => fetch(input, init),
      token: () => tokenRef.current || null,
      now: () => Date.now(),
    }).then((server) => {
      if (server) applyRevocations(server)
    })
  }, [applyRevocations])

  // Replay the public key, tenant id, product catalog, and block list cached by
  // the offline-setup ritual so a scan verifies and renders with zero network.
  useEffect(() => {
    if (!approved) return
    let cancelled = false
    void (async () => {
      const store = idbKVStore()
      const [cfg, catalog, plugins, revocations] = await Promise.all([
        store.get<QRConfig>(QR_CONFIG_KEY).catch(() => null),
        store.get<CatalogEntry[]>(CATALOG_KEY).catch(() => null),
        store.get<string[]>(PLUGINS_KEY).catch(() => null),
        loadRevocations(store).catch(() => null),
      ])
      if (cancelled) return
      const map = new Map<string, CatalogProduct>()
      for (const entry of catalog ?? []) {
        const variants = entry.variants?.length
          ? Object.fromEntries(entry.variants.map((v) => [v.id, v.value]))
          : undefined
        map.set(entry.id, { name: entry.name, image: entry.image, variants })
      }
      const enabled = (plugins ?? []).includes(ARTIST_VOUCHER_PLUGIN_KEY)
      offlineRef.current = {
        publicKey: cfg?.publicKey ?? null,
        tenantId: cfg?.tenantId ?? null,
        catalog: map,
        voucherEnabled: enabled,
      }
      if (revocations) applyRevocations(revocations)
      triggerReconcile()
    })()
    return () => {
      cancelled = true
    }
  }, [approved, applyRevocations, triggerReconcile, catalogVersion])

  useEffect(() => {
    if (!approved) {
      setCameraPermission("idle")
      setScannerActive(false)
    }
  }, [approved])

  useEffect(() => {
    if (!approved) return
    let cancelled = false
    let permissionStatus: PermissionStatus | null = null

    const applyState = (state: PermissionState) => {
      if (state === "granted") {
        setCameraPermission("granted")
      } else if (state === "denied") {
        setCameraPermission("denied")
      } else {
        setCameraPermission("idle")
      }
    }

    const handleChange = () => {
      if (permissionStatus) applyState(permissionStatus.state)
    }

    const checkPermission = async () => {
      try {
        if (!navigator.permissions?.query) return
        permissionStatus = await navigator.permissions.query({ name: "camera" as PermissionName })
        if (cancelled || !permissionStatus) return
        applyState(permissionStatus.state)
        permissionStatus.addEventListener("change", handleChange)
      } catch {
        // ignore; fallback to manual request
      }
    }

    checkPermission()

    return () => {
      cancelled = true
      permissionStatus?.removeEventListener("change", handleChange)
    }
  }, [approved])

  useEffect(() => {
    return () => {
      if (resumeTimerRef.current) clearTimeout(resumeTimerRef.current)
    }
  }, [])

  const requestCameraAccess = useCallback(async () => {
    setError(null)
    setScannerActive(false)
    setScannerKey((k) => k + 1)
    setCameraPermission("pending")
    primeScanAudio()
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: "environment" } })
      setCameraPermission("granted")
      stream.getTracks().forEach((t) => t.stop())
    } catch {
      setCameraPermission("denied")
      setError("Kamerazugriff verweigert oder nicht verfügbar.")
    }
  }, [])

  const handleScannerStartError = useCallback(() => {
    setError("Scanner konnte nicht gestartet werden.")
    setScannerActive(false)
  }, [])

  const handleScannerReady = useCallback((controls: QrScannerControls) => {
    scannerRef.current = controls
  }, [])

  const handleScannerStop = useCallback(() => {
    scannerRef.current = null
    scanLockedRef.current = false
  }, [])

  const startScanner = useCallback(() => {
    setError(null)
    primeScanAudio()
    setScannerActive(true)
    setScannerKey((k) => k + 1)
    scanLockedRef.current = false
  }, [])

  const resumeScanning = useCallback(() => {
    scanLockedRef.current = false
    try {
      scannerRef.current?.resume()
    } catch {}
  }, [])

  const scheduleResume = useCallback(
    (delayMs = SCAN_RESUME_DELAY_MS) => {
      if (resumeTimerRef.current) clearTimeout(resumeTimerRef.current)
      resumeTimerRef.current = setTimeout(() => {
        resumeScanning()
      }, delayMs)
    },
    [resumeScanning]
  )

  const pushScan = useCallback((scan: ScanResult) => {
    setScans((prev) => [scan, ...prev].slice(0, HISTORY_LIMIT))
  }, [])

  // Voucher verdicts are pushed final; only order/campaign scans get patched.
  const updateScan = useCallback((key: string, patch: Partial<OrderScanResult>) => {
    setScans((prev) => prev.map((s) => (s.key === key && s.kind !== "voucher" ? { ...s, ...patch } : s)))
  }, [])

  // Drains queued redemptions to the server. A no-op when offline; the flush
  // itself is idempotent and re-entrancy-safe, so firing it on every trigger
  // (scan, reconnect, interval) is cheap.
  const triggerFlush = useCallback(() => {
    if (typeof navigator !== "undefined" && !navigator.onLine) return
    void flushOutbox({
      outbox: outboxRef.current!,
      fetch: (input, init) => deviceFetchRef.current(input, init),
      token: () => tokenRef.current || null,
      now: () => Date.now(),
    })
  }, [])

  // Queue first (durable before we report success), then flush — immediate when
  // online, deferred to a later trigger when offline.
  const handleEnqueue = useCallback(
    async (orderId: string, productIds: string[], tenantId: string) => {
      await outboxRef.current!.enqueue({ orderId, productIds, tenantId }, Date.now())
      triggerFlush()
    },
    [triggerFlush]
  )

  // Scan-card names resolve from the cached catalog, so refresh it whenever the
  // station is online — otherwise a product created after the last offline-setup
  // ritual renders as its raw id. Throttled: the station runs as a long-lived
  // PWA, so the periodic tick matters, but the full product list doesn't need
  // refetching every 30s.
  const lastCatalogRefreshRef = useRef(0)
  const triggerCatalogRefresh = useCallback(() => {
    if (typeof navigator !== "undefined" && !navigator.onLine) return
    const now = Date.now()
    if (now - lastCatalogRefreshRef.current < CATALOG_REFRESH_INTERVAL_MS) return
    lastCatalogRefreshRef.current = now
    void refreshCatalog().then((ok) => {
      if (ok) setCatalogVersion((v) => v + 1)
    })
  }, [])

  useEffect(() => {
    if (!approved) return
    triggerFlush()
    triggerCatalogRefresh()
    const onOnline = () => {
      triggerFlush()
      triggerReconcile()
      triggerCatalogRefresh()
    }
    window.addEventListener("online", onOnline)
    const interval = setInterval(() => {
      triggerFlush()
      triggerReconcile()
      triggerCatalogRefresh()
    }, 30_000)
    return () => {
      window.removeEventListener("online", onOnline)
      clearInterval(interval)
    }
  }, [approved, triggerFlush, triggerReconcile, triggerCatalogRefresh])

  // Campaign redemption is server-side (it needs the shared counter), so this is
  // the one scan path that can't run offline. It re-runs unchanged when the
  // operator answers a choice prompt — the rejected first attempt consumed
  // nothing, and the idempotency key is carried over.
  const redeemCampaign = useCallback(
    async (key: string, claimToken: string, idem: string, selections: CampaignChoiceSelection[]) => {
      try {
        const res = await fetch(`/api/v1/stations/redeem-campaign`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${bearerToken}`,
            "Idempotency-Key": idem,
          },
          body: JSON.stringify({ claimToken, selections }),
        })
        type Problem = { code?: string; detail?: string; message?: string; groups?: CampaignChoiceGroup[] }
        if (!res.ok) {
          const j = (await res.json().catch(() => ({}))) as Problem
          if (res.status === 409 && j.code === "choice_required" && j.groups?.length) {
            // Leave the scanner paused: the dialog owns the flow from here.
            setChoicePrompt({ scanKey: key, claimToken, idem, groups: j.groups })
            return
          }
          const detail =
            j.detail ||
            j.message ||
            (res.status === 409
              ? "Maximale Einlösungen erreicht."
              : res.status === 410
                ? "Kampagne nicht mehr aktiv."
                : res.status === 404
                  ? "Kampagne nicht gefunden."
                  : `Fehler ${res.status}`)
          throw new Error(detail)
        }
        type CampaignResp = {
          orderId: string
          redemptionCount: number
          maxRedemptions: number
          station?: {
            orderId?: string
            redeemedAt?: string
            items?: Array<{
              id: string
              orderId: string
              productId: string
              title: string
              quantity: number
              isRedeemed: boolean
              parentItemId?: string | null
              bundleSlotId?: string | null
              bundleSlotName?: string | null
              variantId?: string | null
              variantValue?: string | null
            }>
          }
        }
        const data = (await res.json()) as CampaignResp
        const redeemedAt = data.station?.redeemedAt || new Date().toISOString()
        const items = data.station?.items || []
        const lines: OrderLine[] = items.map((it) => ({
          id: it.id,
          orderId: it.orderId,
          productId: it.productId,
          title: it.title,
          quantity: it.quantity,
          unitPriceCents: 0,
          redemption: it.isRedeemed ? { id: it.id, redeemedAt } : null,
          parentLineId: it.parentItemId ?? null,
          bundleSlotId: it.bundleSlotId ?? null,
          bundleSlotName: it.bundleSlotName ?? null,
          variantId: it.variantId ?? null,
          variantValue: it.variantValue ?? null,
        }))
        const unredeemed = lines.filter((l) => !l.redemption)
        const allRedeemed = lines.length > 0 && unredeemed.length === 0
        const nextStatus: ScanStatus = lines.length === 0 ? "no-items" : allRedeemed ? "already-redeemed" : "success"
        updateScan(key, { status: nextStatus, orderId: data.orderId, lines })
        playScanSound(nextStatus === "success" ? "success" : "warning")
      } catch (e: unknown) {
        const message = e instanceof Error ? e.message : "Scan fehlgeschlagen"
        updateScan(key, { status: "error", message })
        playScanSound("error")
      }
      scheduleResume()
    },
    [bearerToken, scheduleResume, updateScan]
  )

  const resolveChoice = useCallback(
    async (prompt: ChoicePrompt, selections: CampaignChoiceSelection[]) => {
      setChoicePrompt(null)
      await redeemCampaign(prompt.scanKey, prompt.claimToken, prompt.idem, selections)
    },
    [redeemCampaign]
  )

  const cancelChoice = useCallback(
    (prompt: ChoicePrompt) => {
      setChoicePrompt(null)
      updateScan(prompt.scanKey, { status: "error", message: "Abgebrochen" })
      // Drop the duplicate-scan guard: nothing was redeemed, so the same slip
      // has to be scannable again right away.
      recentScanIdsRef.current.delete(prompt.claimToken)
      scheduleResume()
    },
    [scheduleResume, updateScan]
  )

  const handleScanned = useCallback(
    async (scan: QrScanResult) => {
      const parsed = parseScan(scan.text, scan.bytes)
      if (!parsed) {
        playScanSound("error")
        setError("Ungültiger QR-Code")
        scheduleResume()
        return
      }

      const now = Date.now()
      const last = recentScanIdsRef.current.get(parsed.token)
      if (last && now - last < DUPLICATE_SCAN_WINDOW_MS) {
        scheduleResume()
        return
      }
      recentScanIdsRef.current.set(parsed.token, now)
      if (recentScanIdsRef.current.size > 32) {
        const cutoff = now - DUPLICATE_SCAN_WINDOW_MS * 4
        recentScanIdsRef.current.forEach((t, k) => {
          if (t < cutoff) recentScanIdsRef.current.delete(k)
        })
      }

      setError(null)
      const key = `${parsed.kind}-${now}`

      if (parsed.kind === "voucher") {
        const offline = offlineRef.current
        // Only honour vouchers when the plugin is enabled for this tenant
        // (cached during offline setup); otherwise it's an unrecognised code.
        if (!offline.voucherEnabled) {
          playScanSound("error")
          // A station never prepared for offline caches neither the plugin flag
          // nor the verification key, so a real voucher reads as invalid — point
          // the operator at offline setup instead of implying the code is bad.
          if (!offline.publicKey || !offline.tenantId) {
            pushScan(
              voucherScanResult(
                key,
                parsed.token,
                { kind: "invalid", reason: "not-ready" },
                revocationsRef.current.syncedAt
              )
            )
          } else {
            setError("Ungültiger QR-Code")
          }
          scheduleResume()
          return
        }
        const verdict = evaluateVoucher(parsed.token, {
          publicKey: offline.publicKey,
          stationTenantId: offline.tenantId,
          now: Date.now(),
          revoked: (id) => revokedIdsRef.current.has(id),
        })
        pushScan(voucherScanResult(key, parsed.token, verdict, revocationsRef.current.syncedAt))
        playScanSound(voucherSound(verdict.kind))
        scheduleResume()
        return
      }

      pushScan({
        key,
        scanId: parsed.token,
        kind: parsed.kind,
        status: "loading",
        scannedAt: new Date(),
        lines: [],
      })

      try {
        if (parsed.kind === "campaign") {
          const idem = `idem_${Date.now()}_${Math.random().toString(36).slice(2)}`
          await redeemCampaign(key, parsed.token, idem, [])
          return
        }

        const offline = offlineRef.current
        if (!offline.publicKey || !offline.tenantId) {
          updateScan(key, { status: "error", message: "Offline-Einrichtung erforderlich" })
          playScanSound("error")
          scheduleResume()
          return
        }

        const result = await redeemScan(parsed.token, {
          verify: (t) => verifyQrToken(t, offline.publicKey),
          stationTenantId: offline.tenantId,
          assignedProductIds,
          catalog: offline.catalog,
          ledger: ledgerRef.current!,
          enqueue: handleEnqueue,
          now: () => Date.now(),
        })

        const toLines = (orderId: string, views: RedeemLineView[]): OrderLine[] =>
          views.map((v) => {
            const lineId = `${orderId}:${v.productId}:${v.variantId ?? ""}`
            return {
              id: lineId,
              orderId,
              productId: v.productId,
              title: v.title,
              quantity: v.quantity,
              unitPriceCents: 0,
              productImage: v.productImage,
              variantId: v.variantId ?? null,
              variantValue: v.variantValue ?? null,
              redemption:
                v.redeemedAt != null ? { id: lineId, redeemedAt: new Date(v.redeemedAt).toISOString() } : null,
            }
          })

        switch (result.kind) {
          case "invalid":
            updateScan(key, { status: "error", message: "Ungültiger Code" })
            playScanSound("error")
            break
          case "wrong-event":
            updateScan(key, { status: "error", message: "Falscher Event" })
            playScanSound("error")
            break
          case "not-for-stand":
            updateScan(key, { status: "no-items" })
            playScanSound("warning")
            break
          case "already-redeemed":
            updateScan(key, {
              status: "already-redeemed",
              orderId: result.orderId,
              lines: toLines(result.orderId, result.lines),
            })
            playScanSound("warning")
            break
          case "redeemed":
            updateScan(key, {
              status: "success",
              orderId: result.orderId,
              lines: toLines(result.orderId, result.lines),
            })
            playScanSound("success")
            break
        }
        scheduleResume()
      } catch (e: unknown) {
        const message = e instanceof Error ? e.message : "Scan fehlgeschlagen"
        updateScan(key, { status: "error", message })
        playScanSound("error")
        scheduleResume()
      }
    },
    [assignedProductIds, handleEnqueue, pushScan, redeemCampaign, scheduleResume, updateScan]
  )

  const handleResult = useCallback(
    async (scan: QrScanResult) => {
      if (scanLockedRef.current) return
      scanLockedRef.current = true
      try {
        scannerRef.current?.pause()
      } catch {}
      await handleScanned(scan)
    },
    [handleScanned]
  )

  useEffect(() => {
    if (!systemDisabled) return
    const interval = setInterval(async () => {
      try {
        const res = await fetch(`/api/v1/system/status`)
        if (!res.ok) return
        const data = (await res.json()) as { enabled: boolean }
        if (data.enabled) {
          window.location.reload()
        }
      } catch {}
    }, 30_000)
    return () => clearInterval(interval)
  }, [systemDisabled])

  if (systemDisabled) {
    return (
      <div className="bg-background flex min-h-screen items-center justify-center">
        <div className="text-center">
          <h2 className="text-2xl font-semibold">System geschlossen</h2>
          <p className="text-muted-foreground mt-2">Das System ist momentan nicht verfügbar.</p>
        </div>
      </div>
    )
  }

  const latest = scans[0] ?? null
  const history = scans.slice(1)

  const loading = sessionState.phase === "loading"
  // `offline-unprepared` means we positively failed to resolve an identity
  // (offline, or the server was unreachable while navigator.onLine still read
  // true). Never offer a pairing code we can't complete in that state.
  const unresolvedOffline = sessionState.phase === "offline-unprepared"
  const needsOnlineSetup = !loading && !approved && (!online || unresolvedOffline)
  const showPairing = !loading && !approved && online && !unresolvedOffline

  return (
    <div className="bg-background text-foreground flex flex-1 flex-col gap-6 px-3 pt-6 pb-10 sm:px-6 sm:pt-8">
      <header className="mx-auto flex w-full max-w-xl items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2.5">
          {logoUrl && (
            <Image
              src={logoUrl}
              alt={`${displayName} Logo`}
              width={36}
              height={36}
              className="size-9 shrink-0 rounded-lg"
            />
          )}
          <p className="min-w-0 truncate text-base font-semibold sm:text-lg">{status?.name || displayName}</p>
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          {approved && online && canSelfManageProducts && (
            <StationProductManager
              token={bearerToken}
              assignedIds={assignedProductIdList}
              onSaved={handleProductsSaved}
            />
          )}
          {approved && <OfflineReadiness />}
          <InstallPrompt />
          <HardRefreshButton />
          <FullscreenButton />
          {approved && <DeviceLogoutButton deviceType="station" />}
        </div>
      </header>

      {loading && (
        <div className="flex flex-1 items-center justify-center">
          <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" aria-hidden />
        </div>
      )}

      {needsOnlineSetup && (
        <div className="mx-auto flex w-full max-w-xl flex-1 flex-col items-center justify-center">
          <OfflineSetupRequired />
        </div>
      )}

      {showPairing && (
        <div className="flex flex-1 flex-col items-center">
          <p className="text-muted-foreground mt-2 max-w-md text-center">Scanne Abhol-QR-Codes hier</p>
          <div className="flex w-full flex-1 items-center justify-center">
            <PairingCodeDisplay deviceType="STATION" />
          </div>
          <PluginSlot name="device.pairing.actions" ctx={{ deviceType: "STATION" }} />
        </div>
      )}

      {approved && (
        <div className="mx-auto flex w-full flex-col items-center gap-8 sm:max-w-xl">
          {!scannerActive && (
            <div className="bg-card w-full rounded-2xl p-6">
              <div className="bg-primary/10 text-primary mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-full">
                <ScanLine className="h-6 w-6" aria-hidden />
              </div>
              <h2 className="text-center text-xl font-semibold">Scanner starten</h2>
              <p className="text-muted-foreground mt-1 text-center text-sm">
                Erlaube den Kamerazugriff, um QR-Codes einzulösen.
              </p>

              <div className="mt-5 grid gap-3">
                {cameraPermission !== "granted" && (
                  <Button
                    onClick={requestCameraAccess}
                    disabled={cameraPermission === "pending"}
                    className="rounded-pill h-12 w-full text-base font-medium"
                  >
                    {cameraPermission === "pending" ? "Kamerazugriff wird angefragt…" : "Kamerazugriff erlauben"}
                  </Button>
                )}

                {cameraPermission === "granted" && (
                  <Button onClick={startScanner} className="rounded-pill h-12 w-full text-base font-medium">
                    Scanner starten
                  </Button>
                )}

                {cameraPermission === "denied" && (
                  <div className="text-muted-foreground bg-card rounded-xl p-3 text-sm">
                    Zugriff abgelehnt. Bitte erlaube den Kamerazugriff in den Browser-Einstellungen und versuche es
                    erneut.
                  </div>
                )}
              </div>
            </div>
          )}

          {scannerActive && cameraPermission === "granted" && (
            <>
              <div className="flex w-full flex-col items-center">
                <div className="w-full max-w-[16rem] overflow-hidden rounded-2xl">
                  <QrScanner
                    key={scannerKey}
                    fps={10}
                    maxWidthRem={16}
                    onResult={handleResult}
                    onReady={handleScannerReady}
                    onStartError={handleScannerStartError}
                    onStop={handleScannerStop}
                  />
                </div>
                <p className="text-muted-foreground mt-3 inline-flex items-center gap-1.5 text-center text-xs">
                  <ScanLine className="h-3.5 w-3.5" aria-hidden />
                  QR-Code in den Rahmen halten
                </p>
              </div>

              <div className="flex w-full flex-col gap-8">
                {latest ? (
                  <ScanCard scan={latest} onOpenDetail={setDetailScan} />
                ) : (
                  <div className="text-muted-foreground bg-card rounded-2xl p-8 text-center text-sm">
                    Bereit zum Scannen. Der nächste QR-Code wird automatisch eingelöst.
                  </div>
                )}

                {history.length > 0 && (
                  <div>
                    <div className="text-muted-foreground mb-3 px-1 text-xs font-medium">Frühere Scans</div>
                    <ul className="flex flex-col gap-1.5">
                      {history.map((s) => (
                        <ScanRow key={s.key} scan={s} onOpenDetail={setDetailScan} />
                      ))}
                    </ul>
                  </div>
                )}
              </div>
            </>
          )}

          {error && (
            <div role="alert" aria-live="assertive" className="text-destructive text-sm">
              {error}
            </div>
          )}
        </div>
      )}

      <OrderDetailDialog
        scan={detailScan}
        online={online}
        bearerToken={bearerToken}
        onClose={() => setDetailScan(null)}
      />
      <CampaignChoiceDialog
        prompt={choicePrompt}
        assignedProductIds={assignedProductIds}
        onConfirm={resolveChoice}
        onCancel={cancelChoice}
      />
      <InstallHintDrawer />
    </div>
  )
}

function CampaignChoiceDialog({
  prompt,
  assignedProductIds,
  onConfirm,
  onCancel,
}: {
  prompt: ChoicePrompt | null
  assignedProductIds: Set<string>
  onConfirm: (prompt: ChoicePrompt, selections: CampaignChoiceSelection[]) => void
  onCancel: (prompt: ChoicePrompt) => void
}) {
  const [picked, setPicked] = useState<Record<number, string>>({})

  useEffect(() => {
    setPicked({})
  }, [prompt])

  if (!prompt) return null

  const selections = prompt.groups
    .map((g) => ({ choiceGroup: g.choiceGroup, productId: picked[g.choiceGroup] }))
    .filter((s): s is CampaignChoiceSelection => !!s.productId)
  const complete = selections.length === prompt.groups.length

  return (
    <Dialog open onOpenChange={(open) => !open && onCancel(prompt)}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="text-center text-xl font-semibold">Auswahl treffen</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-5">
          {prompt.groups.map((group) => (
            <div key={group.choiceGroup} className="flex flex-col gap-2">
              <div className="text-muted-foreground text-xs font-medium">Eines auswählen</div>
              {group.options.map((option) => {
                const active = picked[group.choiceGroup] === option.productId
                // An unassigned product would be handed out nowhere on this
                // station, so the server rejects it — say so before the tap.
                const servedHere = assignedProductIds.size === 0 || assignedProductIds.has(option.productId)
                return (
                  <button
                    key={option.productId}
                    type="button"
                    disabled={!servedHere}
                    onClick={() => setPicked((p) => ({ ...p, [group.choiceGroup]: option.productId }))}
                    aria-pressed={active}
                    className={`flex items-center justify-between gap-3 rounded-xl border-2 px-4 py-3 text-left transition active:scale-[0.99] disabled:pointer-events-none disabled:opacity-45 ${
                      active ? "border-primary bg-primary/5" : "border-border bg-background"
                    }`}
                  >
                    <span className="min-w-0 truncate font-medium">
                      {option.productName}
                      {!servedHere && (
                        <span className="text-muted-foreground block text-xs font-normal">Nicht an dieser Station</span>
                      )}
                    </span>
                    <span className="text-muted-foreground shrink-0 text-sm tabular-nums">×{option.quantity}</span>
                  </button>
                )
              })}
            </div>
          ))}
          <div className="flex gap-2">
            <Button variant="outline" className="flex-1" onClick={() => onCancel(prompt)}>
              Abbrechen
            </Button>
            <Button className="flex-1" disabled={!complete} onClick={() => onConfirm(prompt, selections)}>
              Einlösen
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function ScanCard({ scan, onOpenDetail }: { scan: ScanResult; onOpenDetail?: (scan: OrderScanResult) => void }) {
  if (scan.kind === "voucher") return <VoucherScanCard scan={scan} />
  const meta = STATUS_STYLES[scan.status]
  const Icon = meta.icon
  const matched = scan.lines
  const isLoading = scan.status === "loading"
  const canOpenDetail = !!scan.orderId && !!onOpenDetail

  return (
    <div
      key={scan.key}
      className={`${meta.bg} animate-in fade-in slide-in-from-bottom-1 rounded-2xl duration-300 ease-out`}
    >
      <div className="flex items-center justify-between gap-3 px-5 pt-4 pb-3.5">
        <div className={`flex min-w-0 items-center gap-2 ${meta.text}`}>
          <Icon className={`h-[18px] w-[18px] shrink-0 ${isLoading ? "animate-spin" : ""}`} aria-hidden />
          <span className="font-primary text-sm leading-none font-medium whitespace-nowrap">{meta.label}</span>
        </div>
        {canOpenDetail && (
          <button
            type="button"
            onClick={() => onOpenDetail?.(scan)}
            aria-label="Bestelldetails anzeigen"
            className="text-muted-foreground hover:bg-foreground/5 hover:text-foreground focus-visible:ring-ring/50 inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-full transition-colors focus-visible:ring-2 focus-visible:outline-none"
          >
            <Info className="h-4 w-4" aria-hidden />
          </button>
        )}
      </div>

      {isLoading && (
        <div className="flex flex-col gap-3 px-5 pb-5">
          {[0, 1].map((i) => (
            <div key={i} className="rounded-xl border p-3">
              <div className="flex items-center gap-3">
                <div className="bg-foreground/5 h-16 w-16 shrink-0 animate-pulse rounded-[11px]" />
                <div className="flex-1 space-y-2">
                  <div className="bg-foreground/5 h-3.5 w-3/4 animate-pulse rounded" />
                  <div className="bg-foreground/5 h-3 w-1/2 animate-pulse rounded" />
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {scan.status === "error" && (
        <p className="text-destructive px-5 pt-1 pb-5 text-sm">{scan.message || "Scan fehlgeschlagen"}</p>
      )}

      {scan.status === "no-items" && (
        <p className="text-muted-foreground px-5 pt-1 pb-5 text-sm">
          Diese Station bietet keine Artikel aus dieser Bestellung an.
        </p>
      )}

      {(scan.status === "success" || scan.status === "already-redeemed") && (
        <div className="flex flex-col gap-3 px-5 pb-5">
          {(() => {
            const items = consolidateLines(matched)
            const unredeemedItems = items.filter((it) => !it.redeemed)
            const redeemedItems = items.filter((it) => it.redeemed)
            return (
              <>
                {unredeemedItems.map((it) => (
                  <ConsolidatedItemCard key={`${it.productId}:${it.variantId ?? ""}`} item={it} redeemed={false} />
                ))}
                {redeemedItems.length > 0 && scan.status === "success" && unredeemedItems.length > 0 && (
                  <div className="text-muted-foreground mt-1 text-xs font-medium">Bereits eingelöst</div>
                )}
                {redeemedItems.map((it) => (
                  <ConsolidatedItemCard key={`${it.productId}:${it.variantId ?? ""}`} item={it} redeemed />
                ))}
              </>
            )
          })()}
        </div>
      )}
    </div>
  )
}

function ScanRow({ scan, onOpenDetail }: { scan: ScanResult; onOpenDetail?: (scan: OrderScanResult) => void }) {
  if (scan.kind === "voucher") return <VoucherScanRow scan={scan} />
  const meta = STATUS_STYLES[scan.status]
  const itemCount = scan.lines.reduce((acc, l) => acc + l.quantity, 0)
  const canOpenDetail = !!scan.orderId && !!onOpenDetail
  return (
    <li className={`${meta.bg} flex items-center justify-between gap-3 rounded-xl px-4 py-3 text-sm`}>
      <div className="flex min-w-0 items-center gap-2.5">
        <span className={`h-2 w-2 shrink-0 rounded-full ${meta.dot}`} aria-hidden />
        <span className={`font-primary shrink-0 text-xs leading-none font-medium ${meta.text}`}>{meta.label}</span>
        {itemCount > 0 && (
          <span className="text-muted-foreground truncate text-xs tabular-nums">{itemCount} Artikel</span>
        )}
      </div>
      <div className="text-muted-foreground inline-flex shrink-0 items-center gap-1 text-xs tabular-nums">
        <span>{scan.scannedAt.toLocaleTimeString("de-CH", { hour: "2-digit", minute: "2-digit" })}</span>
        {canOpenDetail && (
          <button
            type="button"
            onClick={() => onOpenDetail?.(scan)}
            aria-label="Bestelldetails anzeigen"
            className="text-muted-foreground hover:bg-foreground/5 hover:text-foreground focus-visible:ring-ring/50 ml-1 inline-flex h-7 w-7 items-center justify-center rounded-full transition-colors focus-visible:ring-2 focus-visible:outline-none"
          >
            <Info className="h-4 w-4" aria-hidden />
          </button>
        )}
      </div>
    </li>
  )
}

function VoucherScanCard({ scan }: { scan: VoucherScanResult }) {
  const meta = VOUCHER_STYLES[scan.verdict]
  const Icon = meta.icon
  return (
    <div className={`${meta.bg} animate-in fade-in slide-in-from-bottom-1 rounded-2xl p-5 duration-300 ease-out`}>
      <div className={`flex items-center gap-2.5 ${meta.text}`}>
        <Icon className="h-8 w-8 shrink-0" aria-hidden />
        <span className="text-2xl leading-tight font-semibold">{meta.label}</span>
      </div>

      {scan.label && <p className="text-foreground mt-3 text-lg leading-snug font-medium">{scan.label}</p>}

      {scan.categories && scan.categories.length > 0 && (
        <div className="mt-3">
          <p className="text-muted-foreground text-xs">Deckt ab</p>
          <div className="mt-1.5 flex flex-wrap gap-1.5">
            {scan.categories.map((c) => (
              <span key={c} className="border-border text-foreground/80 rounded-lg border px-2 py-0.5 text-sm">
                {c}
              </span>
            ))}
          </div>
        </div>
      )}

      {scan.verdict === "valid" && scan.validUntil != null && (
        <p className="text-muted-foreground mt-3 text-sm">Gültig bis {formatVoucherTime(scan.validUntil)}</p>
      )}
      {scan.verdict === "scheduled" && scan.validFrom != null && (
        <p className="text-muted-foreground mt-3 text-sm">Gültig ab {formatVoucherTime(scan.validFrom)}</p>
      )}
      {scan.verdict === "expired" && scan.validUntil != null && (
        <p className="text-muted-foreground mt-3 text-sm">Abgelaufen am {formatVoucherTime(scan.validUntil)}</p>
      )}
      {scan.message && <p className={`mt-3 text-sm ${meta.text}`}>{scan.message}</p>}

      <p className="text-muted-foreground mt-4 text-[11px]">{syncedLabel(scan.syncedAt, Date.now())}</p>
    </div>
  )
}

function VoucherScanRow({ scan }: { scan: VoucherScanResult }) {
  const meta = VOUCHER_STYLES[scan.verdict]
  return (
    <li className={`${meta.bg} flex items-center justify-between gap-3 rounded-xl px-4 py-3 text-sm`}>
      <div className="flex min-w-0 items-center gap-2.5">
        <span className={`h-2 w-2 shrink-0 rounded-full ${meta.dot}`} aria-hidden />
        <span className={`font-primary shrink-0 text-xs leading-none font-medium ${meta.text}`}>{meta.label}</span>
        {scan.label && <span className="text-muted-foreground truncate text-xs">{scan.label}</span>}
      </div>
      <span className="text-muted-foreground shrink-0 text-xs tabular-nums">
        {scan.scannedAt.toLocaleTimeString("de-CH", { hour: "2-digit", minute: "2-digit" })}
      </span>
    </li>
  )
}

function VariantPill({ value }: { value: string }) {
  return (
    <div className="mt-1 flex flex-row flex-wrap gap-1.5">
      <span className="text-muted-foreground border-border rounded-lg border px-2 py-0.5 text-xs">{value}</span>
    </div>
  )
}

function ConsolidatedItemCard({ item, redeemed }: { item: ConsolidatedItem; redeemed: boolean }) {
  return (
    <div className="rounded-xl border p-3">
      <div className="flex items-center gap-3">
        {item.productImage ? (
          <div className="relative h-16 w-16 shrink-0 overflow-hidden rounded-[11px] bg-[#cec9c6]">
            {/* Unoptimized so the request hits the raw R2 URL the offline
                precache stored — the /_next/image optimizer URL is never cached. */}
            <Image
              src={item.productImage}
              alt={item.title}
              fill
              sizes="64px"
              unoptimized
              className="h-full w-full rounded-[11px] object-cover"
            />
          </div>
        ) : (
          <div className="bg-foreground/5 text-muted-foreground flex h-16 w-16 shrink-0 items-center justify-center rounded-[11px]">
            <PackageOpen className="h-5 w-5" aria-hidden />
          </div>
        )}
        <div className="min-w-0 flex-1">
          <p className="truncate font-medium">{item.title}</p>
          {item.variantValue && <VariantPill value={item.variantValue} />}
          {redeemed && item.redemptionTime && (
            <div className="text-muted-foreground mt-0.5 text-xs tabular-nums">
              Eingelöst {new Date(item.redemptionTime).toLocaleTimeString("de-CH")}
            </div>
          )}
        </div>
        <div className="text-foreground/85 shrink-0 text-lg font-semibold tabular-nums">×{item.quantity}</div>
      </div>
    </div>
  )
}

function LineGroupCard({ group, redeemed }: { group: LineGroup; redeemed: boolean }) {
  const { parent, children } = group
  const isBundle = children.length > 0
  const redemptionTime = redeemed ? groupRedemptionTime(group) : undefined

  return (
    <div className="rounded-xl border p-3">
      <div className="flex items-center gap-3">
        {parent.productImage ? (
          <div className="relative h-16 w-16 shrink-0 overflow-hidden rounded-[11px] bg-[#cec9c6]">
            {/* Unoptimized so the request hits the raw R2 URL the offline
                precache stored — the /_next/image optimizer URL is never cached. */}
            <Image
              src={parent.productImage}
              alt={parent.title}
              fill
              sizes="64px"
              unoptimized
              className="h-full w-full rounded-[11px] object-cover"
            />
          </div>
        ) : (
          <div className="bg-foreground/5 text-muted-foreground flex h-16 w-16 shrink-0 items-center justify-center rounded-[11px]">
            <PackageOpen className="h-5 w-5" aria-hidden />
          </div>
        )}
        <div className="min-w-0 flex-1">
          <p className="truncate font-medium">{parent.title}</p>
          {isBundle ? (
            <div className="mt-1 flex flex-row flex-wrap gap-1.5">
              {children.map((c) => (
                <span key={c.id} className="text-muted-foreground border-border rounded-lg border px-2 py-0.5 text-xs">
                  {c.bundleSlotName ?? "Option"}: {c.title}
                </span>
              ))}
            </div>
          ) : parent.variantValue ? (
            <VariantPill value={parent.variantValue} />
          ) : (
            parent.bundleSlotName && (
              <div className="text-muted-foreground mt-0.5 truncate text-xs">{parent.bundleSlotName}</div>
            )
          )}
          {redeemed && redemptionTime && (
            <div className="text-muted-foreground mt-0.5 text-xs tabular-nums">
              Eingelöst {new Date(redemptionTime).toLocaleTimeString("de-CH")}
            </div>
          )}
        </div>
        <div className="text-foreground/85 shrink-0 text-lg font-semibold tabular-nums">×{parent.quantity}</div>
      </div>
    </div>
  )
}

type DetailOrder = {
  id: string
  lines?: OrderLine[]
}

function OrderDetailDialog({
  scan,
  online,
  bearerToken,
  onClose,
}: {
  scan: OrderScanResult | null
  online: boolean
  bearerToken: string
  onClose: () => void
}) {
  const orderId = scan?.orderId ?? null
  const [order, setOrder] = useState<DetailOrder | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setOrder(null)
    setError(null)
    // Offline the station has no server to reach — render the locally-redeemed
    // lines carried on the scan instead of a failed fetch. Online we fetch the
    // full order for the richer view.
    if (!orderId || !online) {
      setLoading(false)
      return
    }
    let cancelled = false
    setLoading(true)
    ;(async () => {
      try {
        const res = await fetch(`/api/v1/orders/${encodeURIComponent(orderId)}`, {
          headers: { Authorization: `Bearer ${bearerToken}` },
        })
        if (!res.ok) {
          const j = (await res.json().catch(() => ({}))) as { detail?: string }
          throw new Error(j.detail || `Fehler ${res.status}`)
        }
        const data = (await res.json()) as DetailOrder
        if (!cancelled) setOrder(data)
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : "Bestellung konnte nicht geladen werden.")
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [orderId, online, bearerToken])

  // Prefer the fetched order; fall back to the scan's local lines whenever the
  // server view isn't available (offline, or the fetch failed).
  const allLines = order?.lines ?? scan?.lines ?? []
  const dialogBundleParents: Record<string, OrderLine> = {}
  for (const l of allLines) {
    if (l.lineType === "bundle") dialogBundleParents[l.id] = l
  }
  const dialogGroups = groupLines(
    allLines.filter((l) => l.lineType !== "bundle"),
    dialogBundleParents
  )
  const showError = !!error && !loading && allLines.length === 0

  return (
    <Dialog open={!!orderId} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="text-center text-xl font-semibold">Bestelldetails</DialogTitle>
        </DialogHeader>

        {loading && (
          <div className="flex items-center justify-center py-12">
            <Loader2 className="text-muted-foreground h-5 w-5 animate-spin" aria-hidden />
          </div>
        )}

        {showError && <p className="text-destructive py-4 text-sm">{error}</p>}

        {!loading &&
          !showError &&
          (dialogGroups.length === 0 ? (
            <p className="text-muted-foreground py-4 text-sm">Keine Artikel in dieser Bestellung.</p>
          ) : (
            <ul className="flex flex-col gap-3">
              {dialogGroups.map((g) => (
                <li key={g.key}>
                  <LineGroupCard group={g} redeemed={false} />
                </li>
              ))}
            </ul>
          ))}
      </DialogContent>
    </Dialog>
  )
}
