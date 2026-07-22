import { apiRequest } from "@/lib/api"
import { getCSRFToken } from "@/lib/csrf"

export interface StationqrPreset {
  id: string
  name: string
  disabled: boolean
  token: string
  enrollUrl: string
  productIds: string[]
  createdAt: string
  updatedAt: string
}

function bearer(sessionToken: string) {
  return { Authorization: `Bearer ${sessionToken}` }
}

export async function listPresets(sessionToken: string): Promise<StationqrPreset[]> {
  const res = await apiRequest<{ items: StationqrPreset[] }>("/v1/plugins/stationqr/presets", {
    headers: bearer(sessionToken),
  })
  return res.items ?? []
}

export async function createPreset(
  sessionToken: string,
  input: { name: string; productIds: string[] }
): Promise<StationqrPreset> {
  return apiRequest<StationqrPreset>("/v1/plugins/stationqr/presets", {
    method: "POST",
    headers: bearer(sessionToken),
    body: JSON.stringify(input),
  })
}

export interface PresetPatch {
  name?: string
  disabled?: boolean
  productIds?: string[]
}

export async function updatePreset(sessionToken: string, id: string, patch: PresetPatch): Promise<StationqrPreset> {
  return apiRequest<StationqrPreset>(`/v1/plugins/stationqr/presets/${id}`, {
    method: "PATCH",
    headers: bearer(sessionToken),
    body: JSON.stringify(patch),
  })
}

export async function rotatePreset(sessionToken: string, id: string): Promise<StationqrPreset> {
  return apiRequest<StationqrPreset>(`/v1/plugins/stationqr/presets/${id}/rotate`, {
    method: "POST",
    headers: bearer(sessionToken),
  })
}

export async function deletePreset(sessionToken: string, id: string): Promise<void> {
  const res = await fetch(`/api/v1/plugins/stationqr/presets/${id}`, {
    method: "DELETE",
    headers: { ...bearer(sessionToken), "X-CSRF": getCSRFToken() || "" },
  })
  if (!res.ok && res.status !== 204) {
    const j = (await res.json().catch(() => ({}))) as { code?: string; message?: string }
    throw new Error(j.message || j.code || `HTTP ${res.status}`)
  }
}

export interface Product {
  id: string
  name: string
}

export async function listProducts(sessionToken: string): Promise<Product[]> {
  const res = await apiRequest<{ items?: { id: string; name: string }[] }>("/v1/products", {
    headers: bearer(sessionToken),
  })
  return (res.items ?? []).map((p) => ({ id: p.id, name: p.name }))
}

export interface EnrollDevice {
  id: string
  name: string
  type: string
  status: string
}

export interface EnrollResult {
  token: string
  device: EnrollDevice
}

export interface EnrollRequest {
  token: string
  deviceKey: string
  name?: string
  model?: string
  os?: string
}

export class EnrollError extends Error {
  code?: string

  constructor(message: string, code?: string) {
    super(message)
    this.code = code
  }
}

export async function enrollDevice(body: EnrollRequest): Promise<EnrollResult> {
  const res = await fetch(`/api/v1/plugins/stationqr/enroll`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const j = (await res.json().catch(() => ({}))) as { code?: string; message?: string }
    throw new EnrollError(j.message || j.code || `HTTP ${res.status}`, j.code)
  }
  return res.json() as Promise<EnrollResult>
}
