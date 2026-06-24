import { describe, expect, it } from "vitest"
import { isMember, isStaff, isTenantAdmin, type MemberTier, ROLE_LABELS_DE, tierAllows } from "./rbac"

const TIERS: MemberTier[] = ["owner", "admin", "staff", "catalog"]
// Values outside the union still reach these helpers at runtime (e.g. a stale
// member.role from the API), so the fail-closed paths must be exercised.
const NON_TIERS = [null, undefined, "" as unknown as MemberTier, "superuser" as unknown as MemberTier]

describe("tierAllows", () => {
  it("admin/owner satisfy every tier", () => {
    for (const t of ["owner", "admin"] as MemberTier[]) {
      expect(tierAllows(t, "admin")).toBe(true)
      expect(tierAllows(t, "staff")).toBe(true)
      expect(tierAllows(t, "catalog")).toBe(true)
    }
  })

  it("staff satisfies staff and catalog but not admin", () => {
    expect(tierAllows("staff", "admin")).toBe(false)
    expect(tierAllows("staff", "staff")).toBe(true)
    expect(tierAllows("staff", "catalog")).toBe(true)
  })

  it("catalog satisfies only catalog", () => {
    expect(tierAllows("catalog", "admin")).toBe(false)
    expect(tierAllows("catalog", "staff")).toBe(false)
    expect(tierAllows("catalog", "catalog")).toBe(true)
  })

  it("fails closed for null/undefined/unknown tiers at every required level", () => {
    for (const t of NON_TIERS) {
      for (const required of TIERS) {
        expect(tierAllows(t, required)).toBe(false)
      }
    }
  })
})

describe("isTenantAdmin", () => {
  it("is true only for owner and admin", () => {
    expect(isTenantAdmin("owner")).toBe(true)
    expect(isTenantAdmin("admin")).toBe(true)
    expect(isTenantAdmin("staff")).toBe(false)
    expect(isTenantAdmin("catalog")).toBe(false)
    for (const t of NON_TIERS) expect(isTenantAdmin(t)).toBe(false)
  })
})

describe("isStaff", () => {
  it("is true for owner, admin and staff, not catalog", () => {
    expect(isStaff("owner")).toBe(true)
    expect(isStaff("admin")).toBe(true)
    expect(isStaff("staff")).toBe(true)
    expect(isStaff("catalog")).toBe(false)
    for (const t of NON_TIERS) expect(isStaff(t)).toBe(false)
  })
})

describe("isMember", () => {
  it("is true for any valid tier and false otherwise", () => {
    for (const t of TIERS) expect(isMember(t)).toBe(true)
    for (const t of NON_TIERS) expect(isMember(t)).toBe(false)
  })
})

describe("ROLE_LABELS_DE", () => {
  it("has a German label for every tier", () => {
    for (const t of TIERS) expect(ROLE_LABELS_DE[t]).toBeTruthy()
  })
})
