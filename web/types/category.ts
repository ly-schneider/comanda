export interface Category {
  id: string
  name: string
  isActive: boolean
  // Required zero-based sort position within the sibling group; lower comes first
  position: number
  parentId: string | null
  createdAt: string // ISO date
  updatedAt: string // ISO date
}

export interface CategoryDTO {
  id: string
  name: string
  isActive: boolean
  // Required zero-based sort position within the sibling group; lower comes first
  position: number
  parentId: string | null
}
