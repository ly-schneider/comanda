import { describe, expect, it } from "vitest"
import { getPublicHost, parseTenantHost } from "./tenant"

describe("parseTenantHost", () => {
  // Platform bare hosts
  it("classifies comanda.leys.ch as platform", () => {
    expect(parseTenantHost("comanda.leys.ch")).toEqual({ type: "platform" })
  })
  it("classifies beta.comanda.leys.ch as platform", () => {
    expect(parseTenantHost("beta.comanda.leys.ch")).toEqual({ type: "platform" })
  })
  it("classifies localhost as platform", () => {
    expect(parseTenantHost("localhost")).toEqual({ type: "platform" })
  })
  it("classifies 127.0.0.1 as platform", () => {
    expect(parseTenantHost("127.0.0.1")).toEqual({ type: "platform" })
  })

  // Tenant subdomains — prod apex
  it("classifies acme.comanda.leys.ch as tenant-subdomain", () => {
    expect(parseTenantHost("acme.comanda.leys.ch")).toEqual({ type: "tenant-subdomain", slug: "acme" })
  })
  it("classifies my-tenant.comanda.leys.ch as tenant-subdomain", () => {
    expect(parseTenantHost("my-tenant.comanda.leys.ch")).toEqual({ type: "tenant-subdomain", slug: "my-tenant" })
  })

  // Tenant subdomains — staging apex
  it("classifies acme.beta.comanda.leys.ch as tenant-subdomain", () => {
    expect(parseTenantHost("acme.beta.comanda.leys.ch")).toEqual({ type: "tenant-subdomain", slug: "acme" })
  })

  // Tenant subdomains — dev
  it("classifies acme.localhost as tenant-subdomain", () => {
    expect(parseTenantHost("acme.localhost")).toEqual({ type: "tenant-subdomain", slug: "acme" })
  })

  // Reserved slugs must NOT become tenant subdomains
  it("classifies www.comanda.leys.ch as platform (reserved)", () => {
    expect(parseTenantHost("www.comanda.leys.ch")).toEqual({ type: "platform" })
  })
  it("classifies api.comanda.leys.ch as platform (reserved)", () => {
    expect(parseTenantHost("api.comanda.leys.ch")).toEqual({ type: "platform" })
  })
  it("classifies beta.localhost as platform (reserved slug 'beta')", () => {
    expect(parseTenantHost("beta.localhost")).toEqual({ type: "platform" })
  })

  // Custom domains
  it("classifies customdomain.com as custom-domain", () => {
    expect(parseTenantHost("customdomain.com")).toEqual({ type: "custom-domain" })
  })
  it("classifies order.myrestaurant.ch as custom-domain", () => {
    expect(parseTenantHost("order.myrestaurant.ch")).toEqual({ type: "custom-domain" })
  })

  // Port stripping
  it("strips port before classifying", () => {
    expect(parseTenantHost("acme.localhost:3000")).toEqual({ type: "tenant-subdomain", slug: "acme" })
    expect(parseTenantHost("localhost:3000")).toEqual({ type: "platform" })
  })

  // Case-insensitive
  it("is case-insensitive", () => {
    expect(parseTenantHost("ACME.comanda.leys.ch")).toEqual({ type: "tenant-subdomain", slug: "acme" })
  })
})

describe("getPublicHost", () => {
  function req(headers: Record<string, string>): Request {
    return new Request("http://placeholder/", { headers })
  }

  it("uses x-forwarded-host when present", () => {
    expect(getPublicHost(req({ "x-forwarded-host": "acme.comanda.leys.ch" }))).toBe("acme.comanda.leys.ch")
  })

  it("falls back to host header", () => {
    expect(getPublicHost(req({ host: "localhost:3000" }))).toBe("localhost")
  })

  it("takes first value of a comma-list", () => {
    expect(getPublicHost(req({ "x-forwarded-host": "acme.comanda.leys.ch, internal.host" }))).toBe(
      "acme.comanda.leys.ch"
    )
  })

  it("strips port from x-forwarded-host", () => {
    expect(getPublicHost(req({ "x-forwarded-host": "acme.comanda.leys.ch:443" }))).toBe("acme.comanda.leys.ch")
  })

  it("lowercases the result", () => {
    expect(getPublicHost(req({ "x-forwarded-host": "ACME.comanda.leys.ch" }))).toBe("acme.comanda.leys.ch")
  })

  it("returns empty string when no host headers present", () => {
    expect(getPublicHost(req({}))).toBe("")
  })
})
