/**
 * Integration-level test of the full offline redemption flow.
 *
 * Covers every Gherkin scenario that the pure-orchestration unit tests
 * can't reach because they inject fakes for the ledger and outbox:
 *
 *   - signed scan → redeemed; ledger + outbox written with real IDB
 *   - rescan same token → already-redeemed; outbox not re-queued
 *   - wrong-tenant token → wrong-event; nothing written
 *   - offline: outbox survives; flush sends exactly once with correct
 *     Idempotency-Key; second flush is a no-op (idempotency)
 *   - ledger survives a "simulated reload" (independent raw IDB connection
 *     that bypasses the module's cached handle)
 *
 * Ed25519 key material is generated fresh for this suite so verification
 * is never mocked — the real crypto path runs end-to-end.
 */

import "fake-indexeddb/auto"
import * as ed from "@noble/ed25519"
import { beforeEach, describe, expect, it, vi } from "vitest"
import { bytesToBase64Url } from "@/lib/base64url"
import { idbLedger } from "./ledger"
import { signV3 } from "./qr-test-helpers"
import { type QrPayload, verifyQrToken } from "./qr-verify"
import { type RedeemDeps, redeemScan } from "./redeem"
import { flushOutbox } from "./sync"
import { idbOutbox } from "./sync-queue"

// Real keypair — verify is never mocked in this suite.
const kp = ed.keygen()
const pubKeyB64 = bytesToBase64Url(kp.publicKey)

const TENANT = "tenant_seed1"
const ORDER = "order_flow_1"
const PROD = "prod_pommes1"

function basePayload(over: Partial<QrPayload> = {}): QrPayload {
  return { v: 3, t: TENANT, o: ORDER, i: 1_700_000_000, l: [{ p: PROD, q: 2 }], ...over }
}

describe("offline redemption flow (integration)", () => {
  let ledger: ReturnType<typeof idbLedger>
  let outbox: ReturnType<typeof idbOutbox>

  beforeEach(async () => {
    ledger = idbLedger()
    outbox = idbOutbox()
    await ledger.clear()
    await outbox.clear()
  })

  function deps(over: Partial<RedeemDeps> = {}): RedeemDeps {
    return {
      verify: (t) => verifyQrToken(t, pubKeyB64),
      stationTenantId: TENANT,
      assignedProductIds: new Set([PROD]),
      catalog: new Map([[PROD, { name: "Pommes Frites", image: "https://r2.example/p1.png" }]]),
      ledger,
      enqueue: async (orderId, productIds, tenantId) => {
        await outbox.enqueue({ orderId, productIds, tenantId }, 9000)
      },
      now: () => 9000,
      ...over,
    }
  }

  it("signed scan → redeemed, enqueued in outbox, ledger persists", async () => {
    const token = signV3(basePayload(), kp.secretKey)
    const result = await redeemScan(token, deps())

    expect(result.kind).toBe("redeemed")
    if (result.kind !== "redeemed") return
    expect(result.orderId).toBe(ORDER)
    expect(result.lines).toEqual([
      expect.objectContaining({ productId: PROD, title: "Pommes Frites", quantity: 2, redeemedAt: 9000 }),
    ])

    expect(await ledger.has(ORDER, PROD, "")).toBe(true)
    expect(await ledger.get(ORDER, PROD, "")).toMatchObject({ quantity: 2, tenantId: TENANT, redeemedAt: 9000 })

    const items = await outbox.list()
    expect(items).toHaveLength(1)
    expect(items[0]).toMatchObject({ orderId: ORDER, productIds: [PROD], tenantId: TENANT })
  })

  it("rescan same token → already-redeemed, outbox not re-queued", async () => {
    const token = signV3(basePayload(), kp.secretKey)
    await redeemScan(token, deps())
    // Drain the outbox so we can clearly see whether a re-queue happened.
    await outbox.clear()

    const second = await redeemScan(token, deps())

    expect(second.kind).toBe("already-redeemed")
    if (second.kind !== "already-redeemed") return
    expect(second.lines[0]).toMatchObject({ productId: PROD, redeemedAt: 9000 })
    expect(await outbox.list()).toHaveLength(0)
  })

  it("wrong-tenant token → wrong-event, nothing written to ledger or outbox", async () => {
    const token = signV3(basePayload({ t: "tenant_othr1" }), kp.secretKey)
    const result = await redeemScan(token, deps())

    expect(result.kind).toBe("wrong-event")
    expect(await ledger.list()).toHaveLength(0)
    expect(await outbox.list()).toHaveLength(0)
  })

  it("offline enqueue → outbox row → flush calls server once with Idempotency-Key → outbox drained", async () => {
    const token = signV3(basePayload(), kp.secretKey)
    await redeemScan(token, deps())

    expect(await outbox.list()).toHaveLength(1)

    const calls: Array<{ url: string; headers: Record<string, string> }> = []
    const fakeFetch = vi.fn(async (url: string, init: RequestInit) => {
      calls.push({ url: String(url), headers: init.headers as Record<string, string> })
      return { ok: true, status: 200 } as Response
    })

    const summary = await flushOutbox({
      outbox,
      fetch: fakeFetch as unknown as typeof fetch,
      token: () => "device-tok",
      now: () => 10_000,
    })

    expect(summary).toMatchObject({ synced: 1, failed: 0, remaining: 0 })
    expect(await outbox.list()).toHaveLength(0)
    expect(calls).toHaveLength(1)
    expect(calls[0]!.headers["Idempotency-Key"]).toBe(ORDER)
    expect(calls[0]!.headers["Authorization"]).toBe("Bearer device-tok")

    // Second flush: outbox is empty, so no requests are made.
    const second = await flushOutbox({
      outbox,
      fetch: fakeFetch as unknown as typeof fetch,
      token: () => "device-tok",
      now: () => 11_000,
    })
    expect(second).toMatchObject({ synced: 0, remaining: 0 })
    expect(calls).toHaveLength(1)
  })

  it("ledger survives a simulated reload (independent raw IDB connection)", async () => {
    const token = signV3(basePayload(), kp.secretKey)
    await redeemScan(token, deps())

    // Open a raw IDB connection that bypasses the module's cached handle to
    // confirm the entry is durably stored, not just in process memory.
    const fromDisk = await new Promise<unknown>((resolve, reject) => {
      const req = indexedDB.open("comanda-station", 4)
      req.onsuccess = () => {
        const db = req.result
        const tx = db.transaction("ledger", "readonly")
        const get = tx.objectStore("ledger").get([ORDER, PROD, ""])
        get.onsuccess = () => {
          resolve(get.result)
          db.close()
        }
        get.onerror = () => reject(get.error)
      }
      req.onerror = () => reject(req.error)
    })

    expect(fromDisk).toMatchObject({ orderId: ORDER, productId: PROD, quantity: 2, redeemedAt: 9000 })
  })
})
