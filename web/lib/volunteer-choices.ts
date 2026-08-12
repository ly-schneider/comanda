export type ChoiceGrouped = { choiceGroup: number }

export type CampaignPosition<T extends ChoiceGrouped> = {
  choiceGroup: number
  options: [T, ...T[]]
}

// Collapses campaign products into the positions handed out per redemption:
// choiceGroup 0 yields one position per product, every positive group yields a
// single position the redeemer picks one option from.
export function campaignPositions<T extends ChoiceGrouped>(products: T[]): CampaignPosition<T>[] {
  const positions: CampaignPosition<T>[] = []
  const byGroup = new Map<number, CampaignPosition<T>>()

  for (const product of products) {
    if (product.choiceGroup <= 0) {
      positions.push({ choiceGroup: 0, options: [product] })
      continue
    }
    const existing = byGroup.get(product.choiceGroup)
    if (existing) {
      existing.options.push(product)
      continue
    }
    const position: CampaignPosition<T> = { choiceGroup: product.choiceGroup, options: [product] }
    byGroup.set(product.choiceGroup, position)
    positions.push(position)
  }
  return positions
}
