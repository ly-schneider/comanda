import { apiRequest } from "@/lib/api"

type SystemStatus = { enabled: boolean }

export async function getSystemStatus(options: RequestInit = {}): Promise<SystemStatus> {
  try {
    return await apiRequest<SystemStatus>("/v1/system/status", options)
  } catch {
    return { enabled: true }
  }
}
