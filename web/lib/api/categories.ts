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
