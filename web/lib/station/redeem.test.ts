import * as ed from "@noble/ed25519"
import { describe, expect, it, vi } from "vitest"
import { memoryLedger } from "./ledger"
import { type QrPayload, verifyQrToken } from "./qr-verify"
import { type RedeemDeps, redeemScan } from "./redeem"

function b64url(bytes: Uint8Array): string {
  let bin = ""
  for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]!)
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "")
}

const keypair = ed.keygen()
const publicKey = b64url(keypair.publicKey)

function token(payload: QrPayload, sk: Uint8Array = keypair.secretKey): string {
  const body = new TextEncoder().encode(JSON.stringify(payload))
  const sig = ed.sign(body, sk)
  const out = new Uint8Array(body.length + sig.length)
  out.set(body, 0)
  out.set(sig, body.length)
  return b64url(out)
}

function payload(over: Partial<QrPayload> = {}): QrPayload {
  return { v: 1, t: "tenant_seed1", o: "order_abc123", i: 1_700_000_000, l: [{ p: "pA", q: 2 }], ...over }
}

function makeDeps(over: Partial<RedeemDeps> = {}): RedeemDeps {
  return {
    verify: (t) => verifyQrToken(t, publicKey),
    stationTenantId: "tenant_seed1",
    assignedProductIds: new Set(["pA"]),
    catalog: new Map([["pA", { name: "Pommes", image: "https://r2.example/pA.png" }]]),
    ledger: memoryLedger(),
    enqueue: vi.fn(async () => {}),
    now: () => 5000,
    ...over,
  }
}

describe("redeemScan", () => {
  it("redeems a valid, unredeemed code: writes the ledger and queues the server write", async () => {
    const deps = makeDeps()
    const result = await redeemScan(token(payload()), deps)

    expect(result.kind).toBe("redeemed")
    if (result.kind !== "redeemed") return
    expect(result.orderId).toBe("order_abc123")
    expect(result.lines).toEqual([
      { productId: "pA", title: "Pommes", productImage: "https://r2.example/pA.png", quantity: 2, redeemedAt: 5000 },
    ])
    expect(await deps.ledger.get("order_abc123", "pA")).toMatchObject({ quantity: 2, tenantId: "tenant_seed1" })
    expect(deps.enqueue).toHaveBeenCalledWith("order_abc123", ["pA"], "tenant_seed1")
  })

  it("rejects a bad signature as invalid and writes nothing", async () => {
    const other = ed.keygen()
    const deps = makeDeps()
    const result = await redeemScan(token(payload(), other.secretKey), deps)

    expect(result.kind).toBe("invalid")
    expect(await deps.ledger.list()).toHaveLength(0)
    expect(deps.enqueue).not.toHaveBeenCalled()
  })

  it("rejects a token for another tenant as wrong-event and writes nothing", async () => {
    const deps = makeDeps()
    const result = await redeemScan(token(payload({ t: "tenant_other1" })), deps)

    expect(result.kind).toBe("wrong-event")
    expect(await deps.ledger.list()).toHaveLength(0)
    expect(deps.enqueue).not.toHaveBeenCalled()
  })

  it("reports already-redeemed on a second scan without duplicating the ledger or re-queueing", async () => {
    const deps = makeDeps()
    await redeemScan(token(payload()), deps)
    ;(deps.enqueue as ReturnType<typeof vi.fn>).mockClear()

    const result = await redeemScan(token(payload()), deps)
    expect(result.kind).toBe("already-redeemed")
    if (result.kind !== "already-redeemed") return
    expect(result.lines[0]).toMatchObject({ productId: "pA", redeemedAt: 5000 })
    expect(await deps.ledger.list()).toHaveLength(1)
    expect(deps.enqueue).not.toHaveBeenCalled()
  })

  it("reports not-for-stand when no line matches an assigned product", async () => {
    const deps = makeDeps({ assignedProductIds: new Set(["pZ"]) })
    const result = await redeemScan(token(payload()), deps)

    expect(result.kind).toBe("not-for-stand")
    expect(await deps.ledger.list()).toHaveLength(0)
    expect(deps.enqueue).not.toHaveBeenCalled()
  })

  it("multi-product: redeeming one product's line leaves the others redeemable at their own stands", async () => {
    const ledger = memoryLedger()
    const multi = payload({
      l: [
        { p: "pA", q: 1 },
        { p: "pB", q: 1 },
      ],
    })

    // Stand A serves pA.
    const standA = makeDeps({ ledger, assignedProductIds: new Set(["pA"]) })
    const a = await redeemScan(token(multi), standA)
    expect(a.kind).toBe("redeemed")

    // Stand B serves pB: pA being redeemed must not block pB.
    const standB = makeDeps({
      ledger,
      assignedProductIds: new Set(["pB"]),
      catalog: new Map([["pB", { name: "Cola" }]]),
    })
    const b = await redeemScan(token(multi), standB)
    expect(b.kind).toBe("redeemed")

    expect(await ledger.has("order_abc123", "pA")).toBe(true)
    expect(await ledger.has("order_abc123", "pB")).toBe(true)
    // Re-scanning at stand A is now a duplicate, not a re-redeem.
    expect((await redeemScan(token(multi), standA)).kind).toBe("already-redeemed")
  })

  it("queues every fresh redemption exactly once; a repeat scan is deduped and not re-queued", async () => {
    const deps = makeDeps()
    const result = await redeemScan(token(payload()), deps)

    expect(result.kind).toBe("redeemed")
    expect(await deps.ledger.get("order_abc123", "pA")).toMatchObject({ quantity: 2 })
    expect(deps.enqueue).toHaveBeenCalledTimes(1)

    // A later scan of the same code is deduped from the ledger and not re-queued;
    // the connectivity of the actual upload is the flusher's concern.
    expect((await redeemScan(token(payload()), deps)).kind).toBe("already-redeemed")
    expect(deps.enqueue).toHaveBeenCalledTimes(1)
  })

  it("falls back to the product id for the title when the catalog has no entry", async () => {
    const deps = makeDeps({ catalog: new Map() })
    const result = await redeemScan(token(payload()), deps)
    if (result.kind !== "redeemed") throw new Error("expected redeemed")
    expect(result.lines[0]).toMatchObject({ productId: "pA", title: "pA", productImage: null })
  })

  it("treats a missing station tenant as wrong-event (never redeems unscoped)", async () => {
    const deps = makeDeps({ stationTenantId: null })
    expect((await redeemScan(token(payload()), deps)).kind).toBe("wrong-event")
  })
})
