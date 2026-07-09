"use client"

import { ArrowDown, ArrowUp, ChevronDown, ChevronRight, Pencil, Plus, RefreshCw, Trash2, X } from "lucide-react"
import Image from "next/image"
import { useCallback, useEffect, useMemo, useState } from "react"
import { ImageUpload } from "@/components/admin/image-upload"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader } from "@/components/ui/card"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"
import { getCSRFToken } from "@/lib/csrf"
import { readErrorMessage } from "@/lib/http"
import type { Bundle, BundleSlot, BundleSlotOption } from "@/types/bundle"
import type { ProductSummaryDTO } from "@/types/product"

// ─── Helpers ──────────────────────────────────────────────────────────

function formatPriceLabel(cents: number): string {
  const francs = Math.floor(cents / 100)
  const rappen = cents % 100
  if (rappen === 0) return `CHF ${francs}.-`
  return `CHF ${(cents / 100).toFixed(2)}`
}

function parsePriceInputToCents(input: string): number | null {
  if (!input) return null
  const norm = input.replace(",", ".").trim()
  const val = Number(norm)
  if (!isFinite(val) || val < 0) return null
  return Math.round(val * 100)
}

function csrfHeaders(method: string, json = true): Record<string, string> {
  const csrf = getCSRFToken()
  const h: Record<string, string> = { "X-CSRF": csrf || "" }
  if (json && method !== "DELETE") h["Content-Type"] = "application/json"
  return h
}

// ─── API helpers ──────────────────────────────────────────────────────

type FetchFn = ReturnType<typeof useAuthorizedFetch>

async function apiCreateBundle(
  fetchAuth: FetchFn,
  body: { categoryId: string; name: string; priceCents: number; image?: string; description?: string | null }
): Promise<Bundle> {
  const res = await fetchAuth(`/api/v1/bundles`, {
    method: "POST",
    headers: csrfHeaders("POST"),
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(await readErrorMessage(res))
  return (await res.json()) as Bundle
}

async function apiUploadBundleImage(fetchAuth: FetchFn, bundleId: string, file: File): Promise<string> {
  const formData = new FormData()
  formData.append("file", file)
  const res = await fetchAuth(`/api/v1/products/${encodeURIComponent(bundleId)}/image`, {
    method: "POST",
    headers: { "X-CSRF": getCSRFToken() || "" },
    body: formData,
  })
  if (!res.ok) throw new Error(await readErrorMessage(res))
  const data = (await res.json()) as { imageUrl: string }
  return data.imageUrl
}

async function apiDeleteBundleImage(fetchAuth: FetchFn, bundleId: string): Promise<void> {
  const res = await fetchAuth(`/api/v1/products/${encodeURIComponent(bundleId)}/image`, {
    method: "DELETE",
    headers: { "X-CSRF": getCSRFToken() || "" },
  })
  if (!res.ok) throw new Error(await readErrorMessage(res))
}

async function apiUpdateBundle(
  fetchAuth: FetchFn,
  bundleId: string,
  body: Partial<{
    name: string
    priceCents: number
    isActive: boolean
    image: string | null
    description: string | null
  }>
): Promise<Bundle> {
  const res = await fetchAuth(`/api/v1/bundles/${encodeURIComponent(bundleId)}`, {
    method: "PATCH",
    headers: csrfHeaders("PATCH"),
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(await readErrorMessage(res))
  return (await res.json()) as Bundle
}

async function apiDeleteBundle(fetchAuth: FetchFn, bundleId: string): Promise<void> {
  const res = await fetchAuth(`/api/v1/bundles/${encodeURIComponent(bundleId)}`, {
    method: "DELETE",
    headers: csrfHeaders("DELETE", false),
  })
  if (!res.ok) throw new Error(await readErrorMessage(res))
}

async function apiCreateSlot(fetchAuth: FetchFn, bundleId: string, name: string): Promise<BundleSlot> {
  const res = await fetchAuth(`/api/v1/bundles/${encodeURIComponent(bundleId)}/slots`, {
    method: "POST",
    headers: csrfHeaders("POST"),
    body: JSON.stringify({ name }),
  })
  if (!res.ok) throw new Error(await readErrorMessage(res))
  return (await res.json()) as BundleSlot
}

async function apiUpdateSlot(fetchAuth: FetchFn, bundleId: string, slotId: string, name: string): Promise<BundleSlot> {
  const res = await fetchAuth(`/api/v1/bundles/${encodeURIComponent(bundleId)}/slots/${encodeURIComponent(slotId)}`, {
    method: "PATCH",
    headers: csrfHeaders("PATCH"),
    body: JSON.stringify({ name }),
  })
  if (!res.ok) throw new Error(await readErrorMessage(res))
  return (await res.json()) as BundleSlot
}

async function apiDeleteSlot(fetchAuth: FetchFn, bundleId: string, slotId: string): Promise<void> {
  const res = await fetchAuth(`/api/v1/bundles/${encodeURIComponent(bundleId)}/slots/${encodeURIComponent(slotId)}`, {
    method: "DELETE",
    headers: csrfHeaders("DELETE", false),
  })
  if (!res.ok) throw new Error(await readErrorMessage(res))
}

async function apiReorderSlots(fetchAuth: FetchFn, bundleId: string, positions: Record<string, number>): Promise<void> {
  const res = await fetchAuth(`/api/v1/bundles/${encodeURIComponent(bundleId)}/slots/reorder`, {
    method: "PATCH",
    headers: csrfHeaders("PATCH"),
    body: JSON.stringify({ positions }),
  })
  if (!res.ok) throw new Error(await readErrorMessage(res))
}

async function apiAddOption(
  fetchAuth: FetchFn,
  bundleId: string,
  slotId: string,
  productId: string
): Promise<BundleSlotOption> {
  const res = await fetchAuth(
    `/api/v1/bundles/${encodeURIComponent(bundleId)}/slots/${encodeURIComponent(slotId)}/options`,
    {
      method: "POST",
      headers: csrfHeaders("POST"),
      body: JSON.stringify({ productId }),
    }
  )
  if (!res.ok) throw new Error(await readErrorMessage(res))
  return (await res.json()) as BundleSlotOption
}

async function apiRemoveOption(
  fetchAuth: FetchFn,
  bundleId: string,
  slotId: string,
  optionProductId: string
): Promise<void> {
  const res = await fetchAuth(
    `/api/v1/bundles/${encodeURIComponent(bundleId)}/slots/${encodeURIComponent(slotId)}/options/${encodeURIComponent(optionProductId)}`,
    {
      method: "DELETE",
      headers: csrfHeaders("DELETE", false),
    }
  )
  if (!res.ok) throw new Error(await readErrorMessage(res))
}

// ─── Page ─────────────────────────────────────────────────────────────

export default function AdminBundlesPage() {
  const fetchAuth = useAuthorizedFetch()
  const [bundles, setBundles] = useState<Bundle[]>([])
  const [cats, setCats] = useState<{ id: string; name: string }[]>([])
  const [products, setProducts] = useState<ProductSummaryDTO[]>([])
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [editBundle, setEditBundle] = useState<Bundle | null>(null)

  const sorted = useMemo(() => {
    return [...bundles].sort((a, b) => a.name.localeCompare(b.name))
  }, [bundles])

  const refetch = useCallback(async () => {
    setError(null)
    try {
      const [br, cr, pr] = await Promise.all([
        fetchAuth(`/api/v1/bundles`),
        fetchAuth(`/api/v1/categories`),
        fetchAuth(`/api/v1/products?type=simple`),
      ])
      if (!br.ok) throw new Error(`Bundles: HTTP ${br.status}`)
      const bundleData = (await br.json()) as { items: Bundle[] }
      setBundles(bundleData.items || [])

      if (cr.ok) {
        const catData = (await cr.json()) as { items: { id: string; name: string }[] }
        setCats(catData.items || [])
      }
      if (pr.ok) {
        const prodData = (await pr.json()) as { items: ProductSummaryDTO[] }
        setProducts(prodData.items || [])
      }
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Fehler beim Laden")
    } finally {
      setLoaded(true)
    }
  }, [fetchAuth])

  useEffect(() => {
    refetch()
  }, [refetch])

  useEffect(() => {
    const onR = () => refetch()
    window.addEventListener("admin:refresh", onR)
    return () => window.removeEventListener("admin:refresh", onR)
  }, [refetch])

  const handleDelete = useCallback(
    async (bundleId: string) => {
      try {
        await apiDeleteBundle(fetchAuth, bundleId)
        await refetch()
      } catch (e: unknown) {
        setError(e instanceof Error ? e.message : "Löschen fehlgeschlagen")
      }
    },
    [fetchAuth, refetch]
  )

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">Bundles</h1>
        <Button size="sm" className="gap-1.5" onClick={() => setCreateOpen(true)}>
          <Plus className="size-4" />
          Neues Bundle
        </Button>
      </div>

      {!loaded ? (
        <div className="grid grid-cols-1 gap-5 sm:gap-3 md:grid-cols-2 xl:grid-cols-3 xl:gap-5">
          {Array.from({ length: 6 }).map((_, i) => (
            <Card key={i} className="rounded-xl border">
              <div className="p-3">
                <Skeleton className="aspect-[4/3] w-full rounded-lg" />
                <div className="mt-3 space-y-2">
                  <Skeleton className="h-4 w-2/3" />
                  <Skeleton className="h-4 w-1/3" />
                </div>
              </div>
            </Card>
          ))}
        </div>
      ) : sorted.length === 0 ? (
        <div className="text-muted-foreground py-12 text-center text-sm">
          Keine Bundles gefunden. Erstelle dein erstes Bundle.
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-5 sm:gap-3 md:grid-cols-2 xl:grid-cols-3 xl:gap-5">
          {sorted.map((bundle) => (
            <BundleCard
              key={bundle.id}
              bundle={bundle}
              products={products}
              fetchAuth={fetchAuth}
              onEdit={() => setEditBundle(bundle)}
              onDelete={() => handleDelete(bundle.id)}
              onRefetch={refetch}
              onError={setError}
              onUpdateSlots={(newSlots) =>
                setBundles((prev) => prev.map((b) => (b.id === bundle.id ? { ...b, slots: newSlots } : b)))
              }
            />
          ))}
        </div>
      )}

      {error && <div className="text-destructive text-sm">{error}</div>}

      <CreateBundleDialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        cats={cats}
        fetchAuth={fetchAuth}
        onCreated={refetch}
        onError={setError}
      />

      <EditBundleDialog
        bundle={editBundle}
        onClose={() => setEditBundle(null)}
        fetchAuth={fetchAuth}
        onSaved={refetch}
        onError={setError}
      />
    </div>
  )
}

// ─── BundleCard ───────────────────────────────────────────────────────

function BundleCard({
  bundle,
  products,
  fetchAuth,
  onEdit,
  onDelete,
  onRefetch,
  onError,
  onUpdateSlots,
}: {
  bundle: Bundle
  products: ProductSummaryDTO[]
  fetchAuth: FetchFn
  onEdit: () => void
  onDelete: () => void
  onRefetch: () => Promise<void>
  onError: (msg: string) => void
  onUpdateSlots: (slots: BundleSlot[]) => void
}) {
  const [open, setOpen] = useState(false)
  const slots = useMemo(() => [...(bundle.slots || [])].sort((a, b) => a.sequence - b.sequence), [bundle.slots])

  return (
    <Card className="gap-0 overflow-hidden rounded-[11px] p-0">
      <CardHeader className="p-2">
        <div className="relative aspect-video rounded-[11px] bg-[#cec9c6]">
          {bundle.image ? (
            <Image
              src={bundle.image}
              alt={"Bundlebild von " + bundle.name}
              fill
              sizes="(max-width: 768px) 100vw, (max-width: 1280px) 50vw, 33vw"
              quality={90}
              className="h-full w-full rounded-[11px] object-cover"
            />
          ) : (
            <div className="absolute inset-0 flex items-center justify-center text-zinc-500">Kein Bild</div>
          )}
          {!bundle.isActive && (
            <div className="absolute inset-0 z-10 grid place-items-center rounded-[11px] bg-black/55">
              <span className="rounded-full bg-zinc-700 px-3 py-1 text-sm font-medium text-white">Nicht aktiv</span>
            </div>
          )}
        </div>
      </CardHeader>

      <CardContent className="space-y-3 px-3 pt-0 pb-3">
        <div className="flex items-center justify-between">
          <div>
            <h3 className="font-family-secondary text-lg font-medium">{bundle.name}</h3>
            <p className="font-family-secondary text-muted-foreground text-sm">{formatPriceLabel(bundle.priceCents)}</p>
          </div>
          <div className="flex items-center gap-1">
            <Button
              size="icon"
              variant="ghost"
              onClick={onEdit}
              aria-label={`Bundle ${bundle.name} bearbeiten`}
              className="size-9"
            >
              <Pencil className="size-4" />
            </Button>
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button
                  size="icon"
                  variant="ghost"
                  aria-label={`Bundle ${bundle.name} löschen`}
                  className="text-destructive hover:text-destructive size-9"
                >
                  <Trash2 className="size-4" />
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Bundle löschen?</AlertDialogTitle>
                  <AlertDialogDescription>
                    &laquo;{bundle.name}&raquo; wird dauerhaft gelöscht inkl. aller Slots und Optionen.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Abbrechen</AlertDialogCancel>
                  <AlertDialogAction onClick={onDelete}>Löschen</AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </div>
        </div>

        <Collapsible open={open} onOpenChange={setOpen}>
          <CollapsibleTrigger asChild>
            <Button variant="ghost" size="sm" className="text-muted-foreground -ml-2 gap-1.5 text-xs">
              {open ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
              {slots.length} {slots.length === 1 ? "Slot" : "Slots"}
            </Button>
          </CollapsibleTrigger>
          <CollapsibleContent>
            <SlotsSection
              bundleId={bundle.id}
              slots={slots}
              products={products}
              fetchAuth={fetchAuth}
              onRefetch={onRefetch}
              onError={onError}
              onUpdateSlots={onUpdateSlots}
            />
          </CollapsibleContent>
        </Collapsible>
      </CardContent>
    </Card>
  )
}

// ─── SlotsSection ─────────────────────────────────────────────────────

function SlotsSection({
  bundleId,
  slots,
  products,
  fetchAuth,
  onRefetch,
  onError,
  onUpdateSlots,
}: {
  bundleId: string
  slots: BundleSlot[]
  products: ProductSummaryDTO[]
  fetchAuth: FetchFn
  onRefetch: () => Promise<void>
  onError: (msg: string) => void
  onUpdateSlots: (slots: BundleSlot[]) => void
}) {
  const [newSlotName, setNewSlotName] = useState("")
  const [busy, setBusy] = useState(false)

  const handleAddSlot = async () => {
    const name = newSlotName.trim()
    if (!name || busy) return
    setBusy(true)
    try {
      await apiCreateSlot(fetchAuth, bundleId, name)
      setNewSlotName("")
      await onRefetch()
    } catch (e: unknown) {
      onError(e instanceof Error ? e.message : "Slot erstellen fehlgeschlagen")
    } finally {
      setBusy(false)
    }
  }

  const [reordering, setReordering] = useState(false)

  const handleMoveSlot = async (slotId: string, direction: "up" | "down") => {
    if (reordering) return
    const idx = slots.findIndex((s) => s.id === slotId)
    if (idx < 0) return
    const swapIdx = direction === "up" ? idx - 1 : idx + 1
    if (swapIdx < 0 || swapIdx >= slots.length) return

    const reordered = [...slots]
    ;[reordered[idx], reordered[swapIdx]] = [reordered[swapIdx]!, reordered[idx]!]
    const normalizedSlots = reordered.map((s, i) => ({ ...s, sequence: i }))

    onUpdateSlots(normalizedSlots)

    const positions: Record<string, number> = {}
    for (const s of normalizedSlots) positions[s.id] = s.sequence

    setReordering(true)
    try {
      await apiReorderSlots(fetchAuth, bundleId, positions)
    } catch (e: unknown) {
      await onRefetch()
      onError(e instanceof Error ? e.message : "Reihenfolge ändern fehlgeschlagen")
    } finally {
      setReordering(false)
    }
  }

  const handleDeleteSlot = async (slotId: string) => {
    try {
      await apiDeleteSlot(fetchAuth, bundleId, slotId)
      await onRefetch()
    } catch (e: unknown) {
      onError(e instanceof Error ? e.message : "Slot löschen fehlgeschlagen")
    }
  }

  return (
    <div className="mt-2 space-y-3">
      {slots.map((slot, idx) => (
        <SlotRow
          key={slot.id}
          bundleId={bundleId}
          slot={slot}
          products={products}
          fetchAuth={fetchAuth}
          onRefetch={onRefetch}
          onError={onError}
          isFirst={idx === 0}
          isLast={idx === slots.length - 1}
          onMoveUp={() => handleMoveSlot(slot.id, "up")}
          onMoveDown={() => handleMoveSlot(slot.id, "down")}
          onDelete={() => handleDeleteSlot(slot.id)}
        />
      ))}

      <div className="flex items-center gap-2">
        <Input
          value={newSlotName}
          onChange={(e) => setNewSlotName(e.target.value)}
          placeholder="Neuer Slot"
          maxLength={20}
          className="h-8 flex-1"
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault()
              handleAddSlot()
            }
          }}
        />
        <Button
          size="sm"
          variant="outline"
          className="h-8 gap-1"
          disabled={!newSlotName.trim() || busy}
          onClick={handleAddSlot}
        >
          <Plus className="size-3.5" />
          Slot
        </Button>
      </div>
    </div>
  )
}

// ─── SlotRow ──────────────────────────────────────────────────────────

function SlotRow({
  bundleId,
  slot,
  products,
  fetchAuth,
  onRefetch,
  onError,
  isFirst,
  isLast,
  onMoveUp,
  onMoveDown,
  onDelete,
}: {
  bundleId: string
  slot: BundleSlot
  products: ProductSummaryDTO[]
  fetchAuth: FetchFn
  onRefetch: () => Promise<void>
  onError: (msg: string) => void
  isFirst: boolean
  isLast: boolean
  onMoveUp: () => void
  onMoveDown: () => void
  onDelete: () => void
}) {
  const [renaming, setRenaming] = useState(false)
  const [nameInput, setNameInput] = useState(slot.name)

  const handleRename = async () => {
    const name = nameInput.trim()
    if (!name || name === slot.name) {
      setRenaming(false)
      setNameInput(slot.name)
      return
    }
    try {
      await apiUpdateSlot(fetchAuth, bundleId, slot.id, name)
      setRenaming(false)
      await onRefetch()
    } catch (e: unknown) {
      onError(e instanceof Error ? e.message : "Umbenennen fehlgeschlagen")
    }
  }

  const assignedIds = useMemo(() => new Set((slot.options || []).map((o) => o.optionProductId)), [slot.options])
  const availableProducts = useMemo(() => products.filter((p) => !assignedIds.has(p.id)), [products, assignedIds])

  const handleAddOption = async (productId: string) => {
    try {
      await apiAddOption(fetchAuth, bundleId, slot.id, productId)
      await onRefetch()
    } catch (e: unknown) {
      onError(e instanceof Error ? e.message : "Option hinzufügen fehlgeschlagen")
    }
  }

  const handleRemoveOption = async (optionProductId: string) => {
    try {
      await apiRemoveOption(fetchAuth, bundleId, slot.id, optionProductId)
      await onRefetch()
    } catch (e: unknown) {
      onError(e instanceof Error ? e.message : "Option entfernen fehlgeschlagen")
    }
  }

  return (
    <div className="border-border rounded-lg border p-2">
      <div className="flex items-center gap-1">
        {renaming ? (
          <Input
            value={nameInput}
            onChange={(e) => setNameInput(e.target.value)}
            maxLength={20}
            className="h-7 flex-1 text-sm"
            autoFocus
            onBlur={handleRename}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault()
                handleRename()
              }
              if (e.key === "Escape") {
                setRenaming(false)
                setNameInput(slot.name)
              }
            }}
          />
        ) : (
          <button
            className="hover:bg-accent flex items-center gap-1 rounded px-1.5 py-0.5 text-sm font-medium"
            onClick={() => {
              setNameInput(slot.name)
              setRenaming(true)
            }}
          >
            {slot.name}
            <Pencil className="text-muted-foreground size-3" />
          </button>
        )}
        <div className="ml-auto flex items-center gap-0.5">
          <Button size="icon" variant="ghost" className="size-7" disabled={isFirst} onClick={onMoveUp}>
            <ArrowUp className="size-3.5" />
          </Button>
          <Button size="icon" variant="ghost" className="size-7" disabled={isLast} onClick={onMoveDown}>
            <ArrowDown className="size-3.5" />
          </Button>
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button size="icon" variant="ghost" className="text-destructive hover:text-destructive size-7">
                <Trash2 className="size-3.5" />
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Slot löschen?</AlertDialogTitle>
                <AlertDialogDescription>
                  Slot &laquo;{slot.name}&raquo; und alle zugehörigen Optionen werden gelöscht.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Abbrechen</AlertDialogCancel>
                <AlertDialogAction onClick={onDelete}>Löschen</AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </div>
      </div>

      <div className="mt-2 flex flex-wrap gap-1.5">
        {(slot.options || []).map((opt) => (
          <Badge key={opt.optionProductId} variant="secondary" className="gap-1 pr-1">
            {opt.optionProduct?.name ?? opt.optionProductId}
            <button
              className="hover:bg-destructive/20 rounded p-0.5"
              onClick={() => handleRemoveOption(opt.optionProductId)}
              aria-label={`Option ${opt.optionProduct?.name ?? ""} entfernen`}
            >
              <X className="size-3" />
            </button>
          </Badge>
        ))}
      </div>

      {availableProducts.length > 0 && (
        <div className="mt-2">
          <Select value="" onValueChange={handleAddOption}>
            <SelectTrigger className="h-8 text-xs">
              <SelectValue placeholder="Produkt hinzufügen…" />
            </SelectTrigger>
            <SelectContent>
              {availableProducts.map((p) => (
                <SelectItem key={p.id} value={p.id}>
                  {p.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}
    </div>
  )
}

// ─── CreateBundleDialog ───────────────────────────────────────────────

function CreateBundleDialog({
  open,
  onClose,
  cats,
  fetchAuth,
  onCreated,
  onError,
}: {
  open: boolean
  onClose: () => void
  cats: { id: string; name: string }[]
  fetchAuth: FetchFn
  onCreated: () => Promise<void>
  onError: (msg: string) => void
}) {
  const [name, setName] = useState("")
  const [categoryId, setCategoryId] = useState<string>("")
  const [priceInput, setPriceInput] = useState("")
  const [image, setImage] = useState("")
  const [description, setDescription] = useState("")
  const [saving, setSaving] = useState(false)
  const [errors, setErrors] = useState<Record<string, string>>({})

  useEffect(() => {
    if (open) {
      setName("")
      setCategoryId("")
      setPriceInput("")
      setImage("")
      setDescription("")
      setErrors({})
    }
  }, [open])

  function validate(): boolean {
    const errs: Record<string, string> = {}
    if (!name.trim()) errs.name = "Name erforderlich"
    if (!categoryId) errs.category = "Kategorie erforderlich"
    const cents = parsePriceInputToCents(priceInput)
    if (cents === null || cents < 0) errs.price = "Preis ungültig"
    setErrors(errs)
    return Object.keys(errs).length === 0
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!validate()) return
    setSaving(true)
    try {
      const cents = parsePriceInputToCents(priceInput) ?? 0
      const trimmedDescription = description.trim()
      await apiCreateBundle(fetchAuth, {
        categoryId,
        name: name.trim(),
        priceCents: cents,
        ...(image.trim() ? { image: image.trim() } : {}),
        ...(trimmedDescription ? { description: trimmedDescription } : {}),
      })
      onClose()
      await onCreated()
    } catch (e: unknown) {
      onError(e instanceof Error ? e.message : "Erstellen fehlgeschlagen")
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) onClose()
      }}
    >
      <DialogContent className="max-w-[520px] rounded-xl">
        <DialogHeader>
          <DialogTitle className="text-sm tracking-wider">NEUES BUNDLE</DialogTitle>
        </DialogHeader>
        <form className="space-y-4" onSubmit={handleSubmit}>
          <div className="space-y-2">
            <Label htmlFor="create-name">Name</Label>
            <Input
              id="create-name"
              value={name}
              onChange={(e) => {
                setName(e.target.value)
                setErrors((prev) => {
                  const { name: _, ...rest } = prev
                  return rest
                })
              }}
              maxLength={20}
              aria-invalid={!!errors.name}
            />
            {errors.name && <p className="text-destructive text-xs">{errors.name}</p>}
          </div>
          <div className="space-y-2">
            <Label>Kategorie</Label>
            <Select
              value={categoryId}
              onValueChange={(v) => {
                setCategoryId(v)
                setErrors((prev) => {
                  const { category: _, ...rest } = prev
                  return rest
                })
              }}
            >
              <SelectTrigger aria-invalid={!!errors.category}>
                <SelectValue placeholder="Kategorie wählen" />
              </SelectTrigger>
              <SelectContent>
                {cats.map((c) => (
                  <SelectItem key={c.id} value={c.id}>
                    {c.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {errors.category && <p className="text-destructive text-xs">{errors.category}</p>}
          </div>
          <div className="space-y-2">
            <Label htmlFor="create-price">Preis (CHF)</Label>
            <Input
              id="create-price"
              inputMode="decimal"
              value={priceInput}
              onChange={(e) => {
                setPriceInput(e.target.value)
                setErrors((prev) => {
                  const { price: _, ...rest } = prev
                  return rest
                })
              }}
              placeholder="z.B. 12.50"
              aria-invalid={!!errors.price}
            />
            {errors.price && <p className="text-destructive text-xs">{errors.price}</p>}
          </div>
          <div className="space-y-2">
            <Label htmlFor="create-image">Bild URL (optional)</Label>
            <Input
              id="create-image"
              value={image}
              onChange={(e) => setImage(e.target.value)}
              placeholder="https://..."
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="create-description">Beschreibung (optional)</Label>
            <Textarea
              id="create-description"
              value={description}
              maxLength={500}
              rows={3}
              placeholder="Optional: Details, Hinweise…"
              onChange={(e) => setDescription(e.target.value)}
            />
            <p className="text-muted-foreground text-xs">{description.length}/500</p>
          </div>
          <div className="flex items-center justify-end gap-2 pt-2">
            <Button type="button" variant="outline" onClick={onClose} disabled={saving}>
              Abbrechen
            </Button>
            <Button type="submit" disabled={saving}>
              {saving && <RefreshCw className="size-4 animate-spin" />}
              Erstellen
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  )
}

// ─── EditBundleDialog ─────────────────────────────────────────────────

function EditBundleDialog({
  bundle,
  onClose,
  fetchAuth,
  onSaved,
  onError,
}: {
  bundle: Bundle | null
  onClose: () => void
  fetchAuth: FetchFn
  onSaved: () => Promise<void>
  onError: (msg: string) => void
}) {
  const [name, setName] = useState("")
  const [priceInput, setPriceInput] = useState("")
  const [image, setImage] = useState<string | null>(null)
  const [description, setDescription] = useState("")
  const [isActive, setIsActive] = useState(true)
  const [saving, setSaving] = useState(false)
  const [fieldErrors, setFieldErrors] = useState<Partial<Record<"name" | "price", string>>>({})

  useEffect(() => {
    if (bundle) {
      setName(bundle.name)
      setPriceInput((bundle.priceCents / 100).toString())
      setImage(bundle.image)
      setDescription(bundle.description ?? "")
      setIsActive(bundle.isActive)
      setFieldErrors({})
    }
  }, [bundle])

  const dirty = useMemo(() => {
    if (!bundle) return false
    const cents = parsePriceInputToCents(priceInput)
    return (
      name.trim() !== bundle.name ||
      (cents !== null && cents !== bundle.priceCents) ||
      description.trim() !== (bundle.description ?? "") ||
      isActive !== bundle.isActive
    )
  }, [bundle, name, priceInput, description, isActive])

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!bundle) return
    const errs: Partial<Record<"name" | "price", string>> = {}
    if (!name.trim()) errs.name = "Name ist erforderlich."
    const cents = parsePriceInputToCents(priceInput)
    if (cents === null || cents < 0) errs.price = "Preis muss 0 oder höher sein."
    setFieldErrors(errs)
    if (Object.keys(errs).length > 0) return
    if (!dirty) return
    setSaving(true)
    try {
      const patch: Parameters<typeof apiUpdateBundle>[2] = {}
      if (name.trim() !== bundle.name) patch.name = name.trim()
      if (cents !== null && cents !== bundle.priceCents) patch.priceCents = cents
      const trimmedDescription = description.trim()
      if (trimmedDescription !== (bundle.description ?? "")) {
        patch.description = trimmedDescription || null
      }
      if (isActive !== bundle.isActive) patch.isActive = isActive

      await apiUpdateBundle(fetchAuth, bundle.id, patch)
      onClose()
      await onSaved()
    } catch (e: unknown) {
      onError(e instanceof Error ? e.message : "Speichern fehlgeschlagen")
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog
      open={!!bundle}
      onOpenChange={(o) => {
        if (!o) onClose()
      }}
    >
      <DialogContent className="max-w-[520px] rounded-xl">
        <DialogHeader>
          <DialogTitle className="text-sm tracking-wider">BEARBEITEN</DialogTitle>
        </DialogHeader>
        {bundle && (
          <form className="space-y-4" onSubmit={handleSubmit}>
            <div className="space-y-2">
              <Label>Bild</Label>
              <ImageUpload
                currentImageUrl={image}
                onUpload={async (file) => {
                  try {
                    const url = await apiUploadBundleImage(fetchAuth, bundle.id, file)
                    setImage(url)
                    await onSaved()
                    return url
                  } catch (e: unknown) {
                    onError(e instanceof Error ? e.message : "Bild-Upload fehlgeschlagen")
                    throw e
                  }
                }}
                onRemove={async () => {
                  try {
                    await apiDeleteBundleImage(fetchAuth, bundle.id)
                    setImage(null)
                    await onSaved()
                  } catch (e: unknown) {
                    onError(e instanceof Error ? e.message : "Bild löschen fehlgeschlagen")
                    throw e
                  }
                }}
                disabled={saving}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-name">Name</Label>
              <Input
                id="edit-name"
                value={name}
                onChange={(e) => {
                  setName(e.target.value)
                  if (fieldErrors.name)
                    setFieldErrors((p) => {
                      const { name: _, ...rest } = p
                      return rest
                    })
                }}
                maxLength={20}
                aria-invalid={!!fieldErrors.name}
              />
              {fieldErrors.name && <p className="text-destructive text-xs">{fieldErrors.name}</p>}
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-price">Preis (CHF)</Label>
              <Input
                id="edit-price"
                inputMode="decimal"
                value={priceInput}
                onChange={(e) => {
                  setPriceInput(e.target.value)
                  if (fieldErrors.price)
                    setFieldErrors((p) => {
                      const { price: _, ...rest } = p
                      return rest
                    })
                }}
                aria-invalid={!!fieldErrors.price}
              />
              {fieldErrors.price && <p className="text-destructive text-xs">{fieldErrors.price}</p>}
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-description">Beschreibung</Label>
              <Textarea
                id="edit-description"
                value={description}
                maxLength={500}
                rows={3}
                placeholder="Optional: Details, Hinweise…"
                onChange={(e) => setDescription(e.target.value)}
              />
              <p className="text-muted-foreground text-xs">{description.length}/500</p>
            </div>
            <div className="flex items-center justify-between gap-3">
              <Label htmlFor="edit-active">Aktiv</Label>
              <Switch id="edit-active" checked={isActive} onCheckedChange={setIsActive} />
            </div>
            <div className="flex items-center justify-end gap-2 pt-2">
              <Button type="button" variant="outline" onClick={onClose} disabled={saving}>
                Abbrechen
              </Button>
              <Button type="submit" disabled={!dirty || saving}>
                {saving && <RefreshCw className="size-4 animate-spin" />}
                Speichern
              </Button>
            </div>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}
