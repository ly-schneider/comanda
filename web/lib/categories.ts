export type CategoryLike = {
  id: string
  name: string
  position: number
  parentId?: string | null
}

const UNRANKED = 1_000_000

function byPositionThenName(a: CategoryLike, b: CategoryLike): number {
  return a.position - b.position || a.name.localeCompare(b.name, "de")
}

export function topLevelCategories<T extends CategoryLike>(categories: T[]): T[] {
  return categories.filter((c) => !c.parentId).sort(byPositionThenName)
}

export function childrenOf<T extends CategoryLike>(categories: T[], parentId: string): T[] {
  return categories.filter((c) => c.parentId === parentId).sort(byPositionThenName)
}

export function hasChildren(categories: CategoryLike[], id: string): boolean {
  return categories.some((c) => c.parentId === id)
}

// The -1 sorts a parent's own products ahead of its children (which start at 0).
export function categoryOrderMap(categories: CategoryLike[]): Map<string, [number, number]> {
  const byId = new Map(categories.map((c) => [c.id, c]))
  const order = new Map<string, [number, number]>()
  for (const c of categories) {
    if (c.parentId) {
      const parent = byId.get(c.parentId)
      order.set(c.id, [parent ? parent.position : UNRANKED, c.position])
    } else {
      order.set(c.id, [c.position, -1])
    }
  }
  return order
}

export function compareByCategory(
  order: Map<string, [number, number]>,
  aCatId: string | null | undefined,
  bCatId: string | null | undefined
): number {
  const a = (aCatId ? order.get(aCatId) : undefined) ?? [UNRANKED, UNRANKED]
  const b = (bCatId ? order.get(bCatId) : undefined) ?? [UNRANKED, UNRANKED]
  return a[0] - b[0] || a[1] - b[1]
}

export type ProductFilterState = { query: string; activeCat: string; activeSub: string }

type FilterableProduct = {
  name: string
  category?: { id?: string; parentId?: string | null } | null
}

export function matchesProductFilter(
  product: FilterableProduct,
  { query, activeCat, activeSub }: ProductFilterState
): boolean {
  const q = query.trim().toLowerCase()
  if (q && !product.name.toLowerCase().includes(q)) return false
  if (activeCat === "all") return true
  if (activeSub !== "all") return product.category?.id === activeSub
  return product.category?.id === activeCat || product.category?.parentId === activeCat
}
