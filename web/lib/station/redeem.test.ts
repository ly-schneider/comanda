import * as ed from "@noble/ed25519"
import { describe, expect, it, vi } from "vitest"
import { bytesToBase64Url } from "@/lib/base64url"
import { memoryLedger } from "./ledger"
import { type QrPayload, verifyQrToken } from "./qr-verify"
import { type RedeemDeps, redeemScan } from "./redeem"

const keypair = ed.keygen()
const publicKey = bytesToBase64Url(keypair.publicKey)

// Encode a v2 binary body (mirrors backend encodeV2) so we sign real tokens the
// production verifier accepts. Ids are the fixed 12 ASCII bytes the format needs.
function encodeV2(p: QrPayload): Uint8Array {
  const enc = new TextEncoder()
  const out = new Uint8Array(30 + p.l.length * 13)
  out[0] = 0x02
  out.set(enc.encode(p.t), 1)
  out.set(enc.encode(p.o), 13)
  out[25] = (p.i >>> 24) & 0xff
  out[26] = (p.i >>> 16) & 0xff
  out[27] = (p.i >>> 8) & 0xff
  out[28] = p.i & 0xff
  out[29] = p.l.length
  let off = 30
  for (const l of p.l) {
    out.set(enc.encode(l.p), off)
    out[off + 12] = l.q
    off += 13
  }
  return out
}

function token(payload: QrPayload, sk: Uint8Array = keypair.secretKey): string {
  const body = encodeV2(payload)
  const sig = ed.sign(body, sk)
  const out = new Uint8Array(body.length + sig.length)
  out.set(body, 0)
  out.set(sig, body.length)
  return bytesToBase64Url(out)
}

const PA = "prod_pommes1"
const PB = "prod_cola001"

function payload(over: Partial<QrPayload> = {}): QrPayload {
  return { v: 2, t: "tenant_seed1", o: "order_abc123", i: 1_700_000_000, l: [{ p: PA, q: 2 }], ...over }
}

function makeDeps(over: Partial<RedeemDeps> = {}): RedeemDeps {
  return {
    verify: (t) => verifyQrToken(t, publicKey),
    stationTenantId: "tenant_seed1",
    assignedProductIds: new Set([PA]),
    catalog: new Map([[PA, { name: "Pommes", image: "https://r2.example/pA.png" }]]),
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
      { productId: PA, title: "Pommes", productImage: "https://r2.example/pA.png", quantity: 2, redeemedAt: 5000 },
    ])
    expect(await deps.ledger.get("order_abc123", PA)).toMatchObject({ quantity: 2, tenantId: "tenant_seed1" })
    expect(deps.enqueue).toHaveBeenCalledWith("order_abc123", [PA], "tenant_seed1")
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
    const result = await redeemScan(token(payload({ t: "tenant_othr1" })), deps)

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
    expect(result.lines[0]).toMatchObject({ productId: PA, redeemedAt: 5000 })
    expect(await deps.ledger.list()).toHaveLength(1)
    expect(deps.enqueue).not.toHaveBeenCalled()
  })

  it("reports not-for-stand when no line matches an assigned product", async () => {
    const deps = makeDeps({ assignedProductIds: new Set(["prod_zzzzz01"]) })
    const result = await redeemScan(token(payload()), deps)

    expect(result.kind).toBe("not-for-stand")
    if (result.kind !== "not-for-stand") return
    expect(result.orderId).toBe("order_abc123")
    expect(await deps.ledger.list()).toHaveLength(0)
    expect(deps.enqueue).not.toHaveBeenCalled()
  })

  it("refuses a blocked order id before matching stand lines, writing nothing", async () => {
    const deps = makeDeps({ blocked: (id) => id === "order_abc123" })
    const result = await redeemScan(token(payload()), deps)

    expect(result.kind).toBe("blocked")
    if (result.kind !== "blocked") return
    expect(result.orderId).toBe("order_abc123")
    expect(await deps.ledger.list()).toHaveLength(0)
    expect(deps.enqueue).not.toHaveBeenCalled()
  })

  it("ignores the block predicate for a different order id", async () => {
    const deps = makeDeps({ blocked: (id) => id === "order_other1" })
    expect((await redeemScan(token(payload()), deps)).kind).toBe("redeemed")
  })

  it("multi-product: redeeming one product's line leaves the others redeemable at their own stands", async () => {
    const ledger = memoryLedger()
    const multi = payload({
      l: [
        { p: PA, q: 1 },
        { p: PB, q: 1 },
      ],
    })

    // Stand A serves PA.
    const standA = makeDeps({ ledger, assignedProductIds: new Set([PA]) })
    const a = await redeemScan(token(multi), standA)
    expect(a.kind).toBe("redeemed")

    // Stand B serves PB: PA being redeemed must not block PB.
    const standB = makeDeps({
      ledger,
      assignedProductIds: new Set([PB]),
      catalog: new Map([[PB, { name: "Cola" }]]),
    })
    const b = await redeemScan(token(multi), standB)
    expect(b.kind).toBe("redeemed")

    expect(await ledger.has("order_abc123", PA)).toBe(true)
    expect(await ledger.has("order_abc123", PB)).toBe(true)
    // Re-scanning at stand A is now a duplicate, not a re-redeem.
    expect((await redeemScan(token(multi), standA)).kind).toBe("already-redeemed")
  })

  it("queues every fresh redemption exactly once; a repeat scan is deduped and not re-queued", async () => {
    const deps = makeDeps()
    const result = await redeemScan(token(payload()), deps)

    expect(result.kind).toBe("redeemed")
    expect(await deps.ledger.get("order_abc123", PA)).toMatchObject({ quantity: 2 })
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
    expect(result.lines[0]).toMatchObject({ productId: PA, title: PA, productImage: null })
  })

  it("treats a missing station tenant as wrong-event (never redeems unscoped)", async () => {
    const deps = makeDeps({ stationTenantId: null })
    expect((await redeemScan(token(payload()), deps)).kind).toBe("wrong-event")
  })
})
