export type ChoiceGrouped = { choiceGroup: number; optionGroup: number }

export type CampaignBranch<T extends ChoiceGrouped> = {
  optionGroup: number
  options: [T, ...T[]]
}

export type CampaignPosition<T extends ChoiceGrouped> = {
  choiceGroup: number
  branches: [CampaignBranch<T>, ...CampaignBranch<T>[]]
}

// Rebuilds the two levels a campaign is authored in: every position is handed
// out (UND), and a position with several branches is a choice (ODER) — a branch
// holding several products being the "one of these drinks" case.
export function campaignPositions<T extends ChoiceGrouped>(products: T[]): CampaignPosition<T>[] {
  const positions: CampaignPosition<T>[] = []
  const byChoice = new Map<number, CampaignPosition<T>>()
  const byBranch = new Map<string, CampaignBranch<T>>()

  for (const product of products) {
    // Positive test on purpose: a response without choiceGroup must degrade to
    // one position per product, not merge every product into one big choice.
    if (!(product.choiceGroup > 0)) {
      positions.push({ choiceGroup: 0, branches: [{ optionGroup: 0, options: [product] }] })
      continue
    }
    const optionGroup = product.optionGroup > 0 ? product.optionGroup : 1
    const branchKey = `${product.choiceGroup}:${optionGroup}`
    const branch = byBranch.get(branchKey)
    if (branch) {
      branch.options.push(product)
      continue
    }
    const created: CampaignBranch<T> = { optionGroup, options: [product] }
    byBranch.set(branchKey, created)

    const position = byChoice.get(product.choiceGroup)
    if (position) {
      position.branches.push(created)
      continue
    }
    const newPosition: CampaignPosition<T> = { choiceGroup: product.choiceGroup, branches: [created] }
    byChoice.set(product.choiceGroup, newPosition)
    positions.push(newPosition)
  }
  return positions
}

// Every product of a position, ignoring which branch it sits in — the station
// resolves a position down to one concrete product either way.
export function positionOptions<T extends ChoiceGrouped>(position: CampaignPosition<T>): T[] {
  return position.branches.flatMap((branch) => branch.options)
}
