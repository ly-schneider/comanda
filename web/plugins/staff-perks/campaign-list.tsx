"use client"

import { ArrowRight, Plus, Ticket, Trash2 } from "lucide-react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { useCallback, useEffect, useMemo, useState } from "react"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { getCSRFToken } from "@/lib/csrf"
import { readErrorMessage } from "@/lib/http"
import type { ProductSummaryDTO } from "@/types/product"
import type { VolunteerCampaignStatus, VolunteerCampaignSummary } from "@/types/volunteer"
import { joinPillActiveClass, joinPillClass, JoinRule } from "./join-rule"
import { STATUS_CLASS, STATUS_LABEL } from "./status"

type OptionRow = { productId: string; quantity: number | "" }
// A group holds interchangeable options — "Getränke" is one group of several
// drinks, and exactly one of them is handed out. joinPrevious is the connector
// to the group above: ODER merges both groups into a single choice, UND hands
// out both.
type GroupRow = { options: OptionRow[]; joinPrevious: boolean }

export default function StaffPerksCampaignList() {
  const fetchAuth = useAuthorizedFetch()
  const router = useRouter()
  const [meals, setMeals] = useState<VolunteerCampaignSummary[]>([])
  const [products, setProducts] = useState<ProductSummaryDTO[]>([])
  const [firstLoadDone, setFirstLoadDone] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [dialogOpen, setDialogOpen] = useState(false)
  const [formName, setFormName] = useState("")
  const [formMaxRedemptions, setFormMaxRedemptions] = useState<number | "">(20)
  const [formGroups, setFormGroups] = useState<GroupRow[]>([])
  const [formValidUntil, setFormValidUntil] = useState<string>("")
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)

  const load = useCallback(async () => {
    setError(null)
    try {
      const res = await fetchAuth("/api/v1/plugins/staff-perks/campaigns")
      if (!res.ok) throw new Error(await readErrorMessage(res))
      const j = (await res.json()) as { items?: VolunteerCampaignSummary[] }
      setMeals(j.items || [])
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Laden fehlgeschlagen")
    } finally {
      setFirstLoadDone(true)
    }
  }, [fetchAuth])

  useEffect(() => {
    load()
  }, [load])

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const res = await fetchAuth("/api/v1/products")
        if (!res.ok) return
        const j = (await res.json()) as { items?: ProductSummaryDTO[] }
        if (!cancelled) setProducts(j.items || [])
      } catch {}
    })()
    return () => {
      cancelled = true
    }
  }, [fetchAuth])

  // A product belongs to exactly one option anywhere in the campaign, so a
  // picked one leaves the remaining dropdowns.
  const usedProductIds = useMemo(
    () => new Set(formGroups.flatMap((g) => g.options.map((o) => o.productId)).filter(Boolean)),
    [formGroups]
  )
  const hasFreeProduct = products.some((p) => !usedProductIds.has(p.id))

  const resetForm = () => {
    setFormName("")
    setFormMaxRedemptions(20)
    setFormGroups([])
    setFormValidUntil("")
    setFormError(null)
  }

  function openCreate() {
    resetForm()
    setDialogOpen(true)
  }

  async function submitCreate() {
    setFormError(null)
    if (!formName.trim()) {
      setFormError("Name erforderlich.")
      return
    }
    if (formGroups.length === 0) {
      setFormError("Mindestens ein Produkt erforderlich.")
      return
    }
    const maxRedemptions = formMaxRedemptions === "" ? 0 : formMaxRedemptions
    if (maxRedemptions < 1) {
      setFormError("Maximale Einlösungen muss mindestens 1 sein.")
      return
    }
    // ODER merges a group into the choice above it, UND opens a new one.
    let choiceGroup = 0
    const productsPayload = formGroups.flatMap((group, idx) => {
      if (idx === 0 || !group.joinPrevious) choiceGroup += 1
      return group.options.map((option) => ({
        productId: option.productId,
        quantity: Math.max(1, option.quantity === "" ? 1 : option.quantity),
        choiceGroup,
      }))
    })
    if (productsPayload.some((p) => !p.productId)) {
      setFormError("Bitte für jede Zeile ein Produkt wählen.")
      return
    }
    setSaving(true)
    try {
      const res = await fetchAuth("/api/v1/plugins/staff-perks/campaigns", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-CSRF": getCSRFToken() || "",
        },
        body: JSON.stringify({
          name: formName.trim(),
          validUntil: formValidUntil ? new Date(formValidUntil).toISOString() : undefined,
          maxRedemptions,
          products: productsPayload,
        }),
      })
      if (!res.ok) throw new Error(await readErrorMessage(res))
      const created = (await res.json()) as VolunteerCampaignSummary
      setDialogOpen(false)
      resetForm()
      router.push(`/admin/staff-perks/${created.id}`)
    } catch (e: unknown) {
      setFormError(e instanceof Error ? e.message : "Erstellen fehlgeschlagen")
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-4 pt-4">
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 flex-col gap-1">
          <h1 className="font-primary text-2xl font-semibold">Mitarbeiter-Gutscheine</h1>
          <p className="text-muted-foreground text-sm">
            Gratis-Bezüge, die Mitarbeiter per QR-Code an der Station einlösen.
          </p>
        </div>
        <Button onClick={openCreate} className="shrink-0">
          <Plus className="size-4" aria-hidden />
          Neuer Gutschein
        </Button>
      </div>

      {error && <div className="bg-destructive/10 text-destructive rounded-xl px-3 py-2 text-sm">{error}</div>}

      {!firstLoadDone ? (
        <LoadingList />
      ) : meals.length === 0 ? (
        <EmptyState onCreate={openCreate} />
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {meals.map((m) => {
            const progress =
              m.maxRedemptions > 0 ? Math.min(100, Math.round((m.redemptionCount / m.maxRedemptions) * 100)) : 0
            return (
              <Link
                key={m.id}
                href={`/admin/staff-perks/${m.id}`}
                className="border-border bg-card hover:bg-accent/40 focus-visible:ring-ring/40 group flex flex-col gap-4 rounded-2xl border p-5 transition focus-visible:ring-2 focus-visible:outline-none"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="truncate font-semibold">{m.name}</span>
                      <StatusBadge status={m.status} />
                    </div>
                    <p className="text-muted-foreground mt-1 text-xs tabular-nums">
                      {m.redemptionCount} / {m.maxRedemptions} eingelöst
                    </p>
                  </div>
                  <ArrowRight
                    className="text-muted-foreground group-hover:text-foreground mt-0.5 size-4 shrink-0 transition-transform group-hover:translate-x-0.5"
                    aria-hidden
                  />
                </div>
                <div className="bg-muted-foreground/20 relative h-1.5 overflow-hidden rounded-full">
                  <div
                    className="bg-primary absolute inset-y-0 left-0 rounded-full transition-[width] duration-300"
                    style={{ width: `${progress}%` }}
                  />
                </div>
                <div className="flex items-center justify-between gap-2">
                  <span className="text-muted-foreground text-xs">Zugangscode</span>
                  <span className="text-sm font-medium tracking-wider">{m.accessCode}</span>
                </div>
              </Link>
            )
          })}
        </div>
      )}

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Neuer Mitarbeiter-Gutschein</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col gap-4">
            <div className="grid gap-2">
              <Label htmlFor="perk-name">Name</Label>
              <Input
                id="perk-name"
                value={formName}
                onChange={(e) => setFormName(e.target.value)}
                placeholder="z.B. April Comanda"
                autoFocus
              />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="grid gap-2">
                <Label htmlFor="perk-count">Maximale Einlösungen</Label>
                <Input
                  id="perk-count"
                  type="number"
                  value={formMaxRedemptions}
                  min={1}
                  onChange={(e) => {
                    const raw = e.target.value
                    setFormMaxRedemptions(raw === "" ? "" : parseInt(raw, 10))
                  }}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="perk-until">Gültig bis (optional)</Label>
                <Input
                  id="perk-until"
                  type="datetime-local"
                  value={formValidUntil}
                  onChange={(e) => setFormValidUntil(e.target.value)}
                />
              </div>
            </div>
            <div className="grid gap-2">
              <Label>Produkte pro Einlösung</Label>
              <p className="text-muted-foreground text-xs">
                Eine Gruppe mit mehreren Optionen wird an der Station zur Auswahl. UND gibt beide Gruppen ab, ODER macht
                daraus eine einzige Auswahl.
              </p>
              <div className="flex flex-col">
                {formGroups.map((group, groupIdx) => (
                  <div key={groupIdx} className="flex flex-col">
                    {groupIdx > 0 && (
                      <JoinSwitch
                        value={group.joinPrevious ? "or" : "and"}
                        onChange={(v) =>
                          setFormGroups((gs) =>
                            gs.map((g, i) => (i === groupIdx ? { ...g, joinPrevious: v === "or" } : g))
                          )
                        }
                      />
                    )}
                    <div className="border-border/70 bg-card flex flex-col gap-2 rounded-xl border p-2.5 shadow-xs">
                      <span className="text-muted-foreground px-0.5 text-[11px]">
                        Gruppe {groupIdx + 1}
                        {group.options.length > 1 && " · eines davon"}
                      </span>
                      {group.options.map((option, optionIdx) => (
                        <div key={optionIdx} className="flex items-center gap-2">
                          <Select
                            value={option.productId}
                            onValueChange={(v) =>
                              setFormGroups((gs) =>
                                gs.map((g, i) =>
                                  i === groupIdx
                                    ? {
                                        ...g,
                                        options: g.options.map((o, j) =>
                                          j === optionIdx ? { ...o, productId: v } : o
                                        ),
                                      }
                                    : g
                                )
                              )
                            }
                          >
                            <SelectTrigger className="flex-1">
                              <SelectValue placeholder="Produkt wählen …" />
                            </SelectTrigger>
                            <SelectContent>
                              {products
                                .filter((p) => p.id === option.productId || !usedProductIds.has(p.id))
                                .map((p) => (
                                  <SelectItem key={p.id} value={p.id}>
                                    {p.name}
                                  </SelectItem>
                                ))}
                            </SelectContent>
                          </Select>
                          <Input
                            type="number"
                            min={1}
                            className="w-16"
                            value={option.quantity}
                            aria-label="Menge"
                            onChange={(e) => {
                              const raw = e.target.value
                              setFormGroups((gs) =>
                                gs.map((g, i) =>
                                  i === groupIdx
                                    ? {
                                        ...g,
                                        options: g.options.map((o, j) =>
                                          j === optionIdx ? { ...o, quantity: raw === "" ? "" : parseInt(raw, 10) } : o
                                        ),
                                      }
                                    : g
                                )
                              )
                            }}
                          />
                          <Button
                            variant="ghost"
                            size="icon"
                            className="text-muted-foreground hover:text-destructive size-9 shrink-0"
                            onClick={() =>
                              setFormGroups((gs) =>
                                gs
                                  .map((g, i) =>
                                    i === groupIdx ? { ...g, options: g.options.filter((_, j) => j !== optionIdx) } : g
                                  )
                                  .filter((g) => g.options.length > 0)
                              )
                            }
                            aria-label="Produkt entfernen"
                          >
                            <Trash2 className="size-4" aria-hidden />
                          </Button>
                        </div>
                      ))}
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-muted-foreground h-7 self-start px-1.5 text-xs"
                        onClick={() =>
                          setFormGroups((gs) =>
                            gs.map((g, i) =>
                              i === groupIdx ? { ...g, options: [...g.options, { productId: "", quantity: 1 }] } : g
                            )
                          )
                        }
                        disabled={!hasFreeProduct}
                      >
                        <Plus className="size-3.5" aria-hidden />
                        Alternative
                      </Button>
                    </div>
                  </div>
                ))}
                <Button
                  variant="outline"
                  size="sm"
                  className={formGroups.length > 0 ? "mt-2.5 self-start" : "self-start"}
                  onClick={() =>
                    setFormGroups((gs) => [...gs, { options: [{ productId: "", quantity: 1 }], joinPrevious: false }])
                  }
                  disabled={!hasFreeProduct}
                >
                  <Plus className="size-4" aria-hidden />
                  Gruppe hinzufügen
                </Button>
              </div>
            </div>
            {formError && (
              <div className="bg-destructive/10 text-destructive rounded-xl p-3 text-sm" role="alert">
                {formError}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)} disabled={saving}>
              Abbrechen
            </Button>
            <Button onClick={submitCreate} disabled={saving}>
              {saving ? "Erstellen …" : "Erstellen"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function JoinSwitch({ value, onChange }: { value: "and" | "or"; onChange: (value: "and" | "or") => void }) {
  return (
    <JoinRule>
      {(["and", "or"] as const).map((option) => (
        <button
          key={option}
          type="button"
          aria-pressed={value === option}
          onClick={() => onChange(option)}
          className={`${joinPillClass} ${
            value === option ? joinPillActiveClass : "text-muted-foreground hover:text-foreground"
          }`}
        >
          {option === "and" ? "UND" : "ODER"}
        </button>
      ))}
    </JoinRule>
  )
}

function StatusBadge({ status }: { status: VolunteerCampaignStatus }) {
  return (
    <span className={`rounded-full px-2 py-0.5 text-xs font-medium ${STATUS_CLASS[status]}`}>
      {STATUS_LABEL[status]}
    </span>
  )
}

function LoadingList() {
  return (
    <div className="flex flex-col gap-2">
      {[0, 1, 2].map((i) => (
        <Skeleton key={i} className="h-14 w-full rounded-xl" />
      ))}
    </div>
  )
}

function EmptyState({ onCreate }: { onCreate: () => void }) {
  return (
    <div className="border-border/60 flex flex-col items-center gap-4 rounded-2xl border border-dashed py-12 text-center">
      <div className="bg-muted text-muted-foreground flex h-12 w-12 items-center justify-center rounded-full">
        <Ticket className="size-5" aria-hidden />
      </div>
      <div className="flex flex-col gap-1">
        <p className="text-sm font-medium">Noch keine Mitarbeiter-Gutscheine angelegt</p>
        <p className="text-muted-foreground text-xs">Lege das erste an und verteile den Link an dein Team.</p>
      </div>
      <Button variant="outline" size="sm" onClick={onCreate}>
        <Plus className="size-4" aria-hidden />
        Ersten Gutschein anlegen
      </Button>
    </div>
  )
}
