import { apiRequest } from "@/lib/api"

export interface StationqrCode {
  configured: boolean
  disabled: boolean
  token: string | null
}

function bearer(sessionToken: string) {
  return { Authorization: `Bearer ${sessionToken}` }
}

export async function getCode(sessionToken: string): Promise<StationqrCode> {
  return apiRequest<StationqrCode>("/v1/plugins/stationqr/code", { headers: bearer(sessionToken) })
}

export async function rotateCode(sessionToken: string): Promise<StationqrCode> {
  return apiRequest<StationqrCode>("/v1/plugins/stationqr/code", {
    method: "POST",
    headers: bearer(sessionToken),
  })
}

export async function setCodeDisabled(sessionToken: string, disabled: boolean): Promise<StationqrCode> {
  return apiRequest<StationqrCode>("/v1/plugins/stationqr/code", {
    method: "PATCH",
    headers: bearer(sessionToken),
    body: JSON.stringify({ disabled }),
  })
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

export async function enrollDevice(body: EnrollRequest): Promise<EnrollResult> {
  const res = await fetch(`/api/v1/plugins/stationqr/enroll`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const j = (await res.json().catch(() => ({}))) as { code?: string; message?: string }
    throw new Error(j.message || j.code || `HTTP ${res.status}`)
  }
  return res.json() as Promise<EnrollResult>
}
