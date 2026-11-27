"use client"

import Image from "next/image"
import { useRouter } from "next/navigation"
import { useCallback, useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { fetchMembershipClaimView, type MembershipClaimView, redeemMembershipClaim } from "./api"
import { CardScanner } from "./card-scanner"
import { DEFAULT_LABEL } from "./label"

// The card's own QR may carry the code in the fragment, which never reaches a
// server log: #code=… wins, ?code=… is accepted for links that cannot do better.
function codeFromLocation(): string {
  if (typeof window === "undefined") return ""
  const hash = new URLSearchParams(window.location.hash.replace(/^#/, ""))
  const fromHash = hash.get("code")
  if (fromHash) return fromHash
  return new URLSearchParams(window.location.search).get("code") ?? ""
}

export default function MembershipClaimPage() {
  const router = useRouter()
  const [code, setCode] = useState("")
  const [view, setView] = useState<MembershipClaimView | null>(null)
  const [loading, setLoading] = useState(false)
  const [claiming, setClaiming] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async (value: string) => {
    setLoading(true)
    setError(null)
    try {
      setView(await fetchMembershipClaimView(value))
      setCode(value)
    } catch (e) {
      setError(e instanceof Error ? e.message : "Die Karte konnte nicht geprüft werden.")
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    const initial = codeFromLocation()
    if (initial) void load(initial)
  }, [load])

  const claim = useCallback(
    async (productId: string) => {
      setClaiming(productId)
      setError(null)
      try {
        const result = await redeemMembershipClaim(code, [{ productId, quantity: 1 }])
        router.push(`/orders/${result.orderId}?from=claim`)
      } catch (e) {
        setError(e instanceof Error ? e.message : "Das hat nicht geklappt.")
        setClaiming(null)
        void load(code)
      }
    },
    [code, load, router]
  )

  const label = view?.label ?? DEFAULT_LABEL

  return (
    <div className="container mx-auto max-w-xl p-4">
      <h1 className="mb-1 text-2xl font-semibold">{label}</h1>
      <p className="text-muted-foreground mb-6">Karte scannen und den Gratis-Artikel aussuchen.</p>

      {error && (
        <div role="alert" className="text-destructive bg-destructive/10 mb-4 rounded-xl px-3 py-2 text-sm">
          {error}
        </div>
      )}

      {!view && (
        <Card className="rounded-2xl">
          <CardContent className="pt-6">
            <CardScanner onCode={load} busy={loading} manualLabel="Mitglied-Nummer" />
          </CardContent>
        </Card>
      )}

      {view && (
        <Card className="rounded-2xl">
          <CardHeader>
            <CardTitle>{view.member?.name ?? "Mitglied"}</CardTitle>
            <p className="text-muted-foreground text-sm">{view.message}</p>
          </CardHeader>
          <CardContent className="space-y-3">
            {view.canRedeem ? (
              <>
                <p className="text-sm font-medium">Was möchtest du?</p>
                <ul className="flex flex-col gap-2">
                  {view.products.map((product) => (
                    <li key={product.id}>
                      <button
                        type="button"
                        disabled={claiming !== null}
                        onClick={() => claim(product.id)}
                        className="hover:bg-accent flex w-full items-center gap-3 rounded-xl border px-4 py-3 text-left disabled:opacity-50"
                      >
                        {product.image && (
                          <Image
                            src={product.image}
                            alt=""
                            width={48}
                            height={48}
                            className="size-12 rounded-lg object-cover"
                          />
                        )}
                        <span className="flex-1 font-medium">{product.name}</span>
                        <span className="text-muted-foreground text-sm">
                          {claiming === product.id ? "Wird geholt…" : "Gratis"}
                        </span>
                      </button>
                    </li>
                  ))}
                </ul>
              </>
            ) : (
              <p className="text-sm text-amber-600">
                {view.products.length === 0 && view.status === "ok"
                  ? "Im Moment gibt es nichts, was gratis abgegeben wird."
                  : "Mit dieser Karte lässt sich gerade nichts einlösen."}
              </p>
            )}
            <Button
              variant="outline"
              onClick={() => {
                setView(null)
                setCode("")
              }}
            >
              Andere Karte
            </Button>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
