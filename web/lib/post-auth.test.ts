import { describe, expect, it } from "vitest"
import { postAuthPath } from "@/lib/post-auth"

describe("postAuthPath", () => {
  const admin = { isPlatformAdmin: true }
  const member = { isPlatformAdmin: false, activeOrganizationId: "org1" }
  const customer = { isPlatformAdmin: false }

  it("prefers a safe next over role defaults", () => {
    expect(postAuthPath(member, "/admin")).toBe("/admin")
    expect(postAuthPath(admin, "/admin")).toBe("/admin")
    expect(postAuthPath(customer, "/orders")).toBe("/orders")
  })

  it("falls back to role defaults without next", () => {
    expect(postAuthPath(admin)).toBe("/platform-admin")
    expect(postAuthPath(member)).toBe("/account")
    expect(postAuthPath(customer)).toBe("/")
  })

  it("rejects unsafe next values", () => {
    expect(postAuthPath(customer, "//evil.example")).toBe("/")
    expect(postAuthPath(customer, "https://evil.example")).toBe("/")
    expect(postAuthPath(member, "//evil.example")).toBe("/account")
  })
})
