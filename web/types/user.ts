export interface User {
  id: string
  email: string
  firstName?: string
  lastName?: string
  isVerified: boolean
  isClub100: boolean
  // Orthogonal to member.role: a Leys operator with cross-tenant platform access.
  isPlatformAdmin: boolean
  createdAt: string // ISO date
  updatedAt: string // ISO date
}
