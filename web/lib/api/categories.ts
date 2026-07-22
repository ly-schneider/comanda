import type { CategoryDTO } from "@/types"
import { apiRequest } from "../api"

interface RawCategoryList {
  items?: CategoryDTO[]
}

// Returns categories ordered by position (as served by the backend).
export async function listCategories(options: RequestInit = {}): Promise<CategoryDTO[]> {
  const raw = await apiRequest<RawCategoryList>("/v1/categories", options)
  return raw.items || []
}

export async function setCategoryActive(token: string, categoryId: string, isActive: boolean): Promise<CategoryDTO> {
  const res = await fetch(`/api/v1/pos/categories/${encodeURIComponent(categoryId)}/active`, {
    method: "PATCH",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ isActive }),
  })
  if (!res.ok) throw new Error("active_update_failed")
  return (await res.json()) as CategoryDTO
}
