// Device self-logout. The server revoke is best-effort on purpose: an offline
// tablet must still clear itself locally rather than stay stuck signed in, so a
// failed revoke never blocks the local wipe + redirect to the enrollment screen.

import { clearDeviceToken, type DeviceType, getDeviceToken } from "@/lib/device-auth"
import { clearStationDatabase } from "@/lib/station/offline-db"
import { idbOutbox, type StationOutbox } from "@/lib/station/sync-queue"

const POS_QUEUE_KEY = "comanda-pos-order-queue"

const DEVICE_ME_URL: Record<DeviceType, string> = {
  pos: "/api/v1/pos/me",
  station: "/api/v1/stations/me",
}

const DEVICE_HOME_PATH: Record<DeviceType, string> = {
  pos: "/pos",
  station: "/station",
}

export interface LogoutDeps {
  fetchFn?: typeof fetch
  token?: string | null
  clearStationDb?: () => Promise<void>
  clearCaches?: () => Promise<void>
  clearPosQueue?: () => void
  redirect?: (path: string) => void
}

async function clearAllCaches(): Promise<void> {
  if (typeof caches === "undefined") return
  const keys = await caches.keys()
  await Promise.all(keys.map((k) => caches.delete(k)))
}

function clearPosQueue(): void {
  if (typeof sessionStorage === "undefined") return
  try {
    sessionStorage.removeItem(POS_QUEUE_KEY)
  } catch {}
}

// Count of local work lost on logout: queued station redemptions or unsynced POS
// orders. Best-effort — an unreadable store counts as 0.
export async function unsyncedWorkCount(deviceType: DeviceType, outbox: StationOutbox = idbOutbox()): Promise<number> {
  try {
    if (deviceType === "station") {
      const items = await outbox.list()
      return items.length
    }
    if (typeof sessionStorage === "undefined") return 0
    const raw = sessionStorage.getItem(POS_QUEUE_KEY)
    if (!raw) return 0
    const orders = JSON.parse(raw) as Array<{ status?: string }>
    return orders.filter((o) => o.status !== "synced" && o.status !== "paid").length
  } catch {
    return 0
  }
}

export async function logoutDevice(deviceType: DeviceType, deps: LogoutDeps = {}): Promise<void> {
  const token = deps.token ?? getDeviceToken()
  const fetchFn = deps.fetchFn ?? ((...args: Parameters<typeof fetch>) => fetch(...args))

  if (token) {
    try {
      await fetchFn(DEVICE_ME_URL[deviceType], {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      })
    } catch {}
  }

  clearDeviceToken()

  if (deviceType === "station") {
    await (deps.clearStationDb ?? clearStationDatabase)().catch(() => {})
    await (deps.clearCaches ?? clearAllCaches)().catch(() => {})
  } else {
    ;(deps.clearPosQueue ?? clearPosQueue)()
  }

  const redirect = deps.redirect ?? ((path: string) => window.location.assign(path))
  redirect(DEVICE_HOME_PATH[deviceType])
}
