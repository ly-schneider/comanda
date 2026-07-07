import { apiRequest } from "@/lib/api"

export interface Club100Person {
  id: string
  firstName: string
  lastName: string
  remaining: number
  max: number
}

export interface Club100Settings {
  freeProductIds: string[]
  maxRedemptions: number
}

function bearer(token: string) {
  return { Authorization: `Bearer ${token}` }
}

export async function listClub100People(token: string): Promise<Club100Person[]> {
  const res = await apiRequest<{ items: Club100Person[] }>("/v1/plugins/club100/people", {
    headers: bearer(token),
  })
  return res.items
}

export async function getClub100Settings(token: string): Promise<Club100Settings> {
  return apiRequest<Club100Settings>("/v1/plugins/club100/settings", {
    headers: bearer(token),
  })
}

export async function updateClub100Settings(token: string, patch: Partial<Club100Settings>): Promise<Club100Settings> {
  return apiRequest<Club100Settings>("/v1/plugins/club100/settings", {
    method: "PUT",
    headers: bearer(token),
    body: JSON.stringify(patch),
  })
}
