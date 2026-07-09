"use client"
import { Minus, Plus } from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import { DepotInfo } from "@/components/admin/depot-info"
import { ImageUpload } from "@/components/admin/image-upload"
import { ProductImageCrop } from "@/components/admin/product-image-crop"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { useAuthorizedFetch } from "@/hooks/use-authorized-fetch"

import { getCSRFToken } from "@/lib/csrf"
import { readApiError, readErrorMessage } from "@/lib/http"
import type { Jeton, PosFulfillmentMode } from "@/types/jeton"

type ProductVariant = {
  id?: string
  value: string
  priceAdjustmentCents: number
  isActive: boolean
  sequence: number
  stock?: number | null
}

type Product = {
  id: string
  name: string
  priceCents: number
  depositCents?: number
  isActive: boolean
  image?: string | null
  description?: string | null
  category?: { id: string; name: string }
  stock?: number | null
  jeton?: Jeton
  type: "simple" | "bundle"
  variantLabel?: string | null
  variants?: ProductVariant[]
}
type Category = { id: string; name: string; isActive: boolean; position: number }
const NO_JETON_VALUE = "__none__"

export default function AdminProductsPage() {
  const fetchAuth = useAuthorizedFetch()
  const [items, setItems] = useState<Product[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [cats, setCats] = useState<Category[]>([])
  const [jetons, setJetons] = useState<Jeton[]>([])
  const [posMode, setPosMode] = useState<PosFulfillmentMode>("QR_CODE")
  const [createOpen, setCreateOpen] = useState(false)
  const [successMsg, setSuccessMsg] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    ;(async () => {
      try {
        const res = await fetchAuth(`/api/v1/products`)
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const data = (await res.json()) as { items: Product[]; count: number }
        if (cancelled) return
        setItems(data.items)
      } catch (e: unknown) {
        const msg = e instanceof Error ? e.message : "Produkte laden fehlgeschlagen"
        if (!cancelled) setError(msg)
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [fetchAuth])

  useEffect(() => {
    ;(async () => {
      try {
        const res = await fetchAuth(`/api/v1/jetons`)
        if (res.ok) {
          const j = (await res.json()) as { items?: Jeton[] }
          setJetons(j.items || [])
        }
      } catch {
        // ignore jeton load errors here
      }
    })()
  }, [fetchAuth])

  useEffect(() => {
    ;(async () => {
      try {
        const res = await fetchAuth(`/api/v1/settings`)
        if (res.ok) {
          const j = (await res.json()) as { posMode?: PosFulfillmentMode }
          setPosMode(j.posMode || "QR_CODE")
        }
      } catch {
        // ignore
      }
    })()
  }, [fetchAuth])

  // Load categories once
  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const cr = await fetchAuth(`/api/v1/categories`)
        if (cr.ok) {
          const d = (await cr.json()) as { items: Category[] }
          if (!cancelled) setCats(d.items || [])
        }
      } catch {}
    })()
    return () => {
      cancelled = true
    }
  }, [fetchAuth])

  async function updatePrice(id: string, priceCents: number) {
    const csrf = getCSRFToken()
    const res = await fetchAuth(`/api/v1/products/${encodeURIComponent(id)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
      body: JSON.stringify({ priceCents }),
    })
    if (!res.ok) throw new Error(await readErrorMessage(res))
  }
  async function updateDeposit(id: string, depositCents: number) {
    const csrf = getCSRFToken()
    const res = await fetchAuth(`/api/v1/products/${encodeURIComponent(id)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
      body: JSON.stringify({ depositCents }),
    })
    if (!res.ok) throw new Error(await readErrorMessage(res))
  }
  async function updateName(id: string, name: string) {
    const csrf = getCSRFToken()
    const res = await fetchAuth(`/api/v1/products/${encodeURIComponent(id)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
      body: JSON.stringify({ name }),
    })
    if (!res.ok) throw new Error(await readErrorMessage(res))
  }
  async function updateDescription(id: string, description: string | null) {
    const csrf = getCSRFToken()
    const res = await fetchAuth(`/api/v1/products/${encodeURIComponent(id)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
      body: JSON.stringify({ description: description ?? "" }),
    })
    if (!res.ok) throw new Error(await readErrorMessage(res))
  }
  async function moveCategory(id: string, categoryId: string) {
    const csrf = getCSRFToken()
    const res = await fetchAuth(`/api/v1/products/${encodeURIComponent(id)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
      body: JSON.stringify({ categoryId }),
    })
    if (!res.ok) throw new Error("Kategorie verschieben fehlgeschlagen")
  }

  async function setActive(id: string, isActive: boolean) {
    const csrf = getCSRFToken()
    const res = await fetchAuth(`/api/v1/products/${encodeURIComponent(id)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
      body: JSON.stringify({ isActive }),
    })
    if (!res.ok) {
      const err = await readApiError(res)
      if (err.raw === "jeton_required") throw new Error("Bitte zuerst einen Jeton zuweisen.")
      throw new Error(err.messageDe)
    }
  }

  async function deleteHard(id: string) {
    const csrf = getCSRFToken()
    const res = await fetchAuth(`/api/v1/products/${encodeURIComponent(id)}`, {
      method: "DELETE",
      headers: { "X-CSRF": csrf || "" },
    })
    if (!res.ok) throw new Error("Produkt löschen fehlgeschlagen")
  }

  async function adjustInventory(id: string, delta: number, variantId?: string) {
    const csrf = getCSRFToken()
    const body: Record<string, unknown> = { delta, reason: "manual_adjust" }
    if (variantId) body.variantId = variantId
    const res = await fetchAuth(`/api/v1/products/${encodeURIComponent(id)}/inventory`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
      body: JSON.stringify(body),
    })
    if (!res.ok) throw new Error(await readErrorMessage(res))
  }

  async function updateVariants(
    id: string,
    variantLabel: string | null,
    variants: ProductVariant[]
  ): Promise<ProductVariant[]> {
    const csrf = getCSRFToken()
    const res = await fetchAuth(`/api/v1/products/${encodeURIComponent(id)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
      body: JSON.stringify({
        variantLabel: variantLabel ?? "",
        variants: variants.map((v, idx) => ({
          id: v.id,
          value: v.value,
          priceAdjustmentCents: v.priceAdjustmentCents,
          isActive: v.isActive,
          sequence: idx + 1,
        })),
      }),
    })
    if (!res.ok) throw new Error(await readErrorMessage(res))
    const data = (await res.json()) as { variants?: ProductVariant[] }
    return data.variants ?? []
  }

  async function updateJeton(id: string, jetonId: string | null) {
    const csrf = getCSRFToken()
    const res = await fetchAuth(`/api/v1/products/${encodeURIComponent(id)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
      body: JSON.stringify({ jetonId }),
    })
    if (!res.ok) {
      const err = await readApiError(res)
      if (err.raw === "jeton_required") throw new Error("Im Jeton-Modus ist ein Jeton Pflicht.")
      throw new Error(err.messageDe)
    }
  }

  async function uploadProductImage(id: string, file: File): Promise<string> {
    const csrf = getCSRFToken()
    const formData = new FormData()
    formData.append("file", file)
    const res = await fetchAuth(`/api/v1/products/${encodeURIComponent(id)}/image`, {
      method: "POST",
      headers: { "X-CSRF": csrf || "" },
      body: formData,
    })
    if (!res.ok) throw new Error(await readErrorMessage(res))
    const data = (await res.json()) as { imageUrl: string }
    return data.imageUrl
  }

  async function deleteProductImage(id: string): Promise<void> {
    const csrf = getCSRFToken()
    const res = await fetchAuth(`/api/v1/products/${encodeURIComponent(id)}/image`, {
      method: "DELETE",
      headers: { "X-CSRF": csrf || "" },
    })
    if (!res.ok) throw new Error(await readErrorMessage(res))
  }

  async function createProduct(payload: {
    name: string
    priceCents: number
    depositCents?: number
    categoryId: string
    type: "simple" | "bundle"
    isActive: boolean
    image?: string | null
    description?: string | null
    jetonId?: string | null
  }) {
    const csrf = getCSRFToken()
    if (payload.type === "bundle") {
      const res = await fetchAuth(`/api/v1/bundles`, {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
        body: JSON.stringify({
          name: payload.name,
          priceCents: payload.priceCents,
          categoryId: payload.categoryId,
          image: payload.image,
          isActive: payload.isActive,
        }),
      })
      if (!res.ok) throw new Error(await readErrorMessage(res))
      const created = (await res.json()) as Partial<Product> & { id?: string }
      if (payload.description && created.id) {
        await updateDescription(created.id, payload.description)
        created.description = payload.description
      }
      return created
    }
    const res = await fetchAuth(`/api/v1/products`, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-CSRF": csrf || "" },
      body: JSON.stringify({
        name: payload.name,
        priceCents: payload.priceCents,
        depositCents: payload.depositCents ?? 0,
        categoryId: payload.categoryId,
        type: payload.type,
        isActive: payload.isActive,
        image: payload.image,
        description: payload.description ?? undefined,
        jetonId: payload.jetonId,
      }),
    })
    if (!res.ok) throw new Error(await readErrorMessage(res))
    return (await res.json()) as Partial<Product> & { id?: string }
  }

  const catPositionMap = useMemo(() => {
    const m = new Map<string, number>()
    for (const c of cats) m.set(c.id, c.position)
    return m
  }, [cats])

  const sortedItems = useMemo(
    () =>
      [...items].sort((a, b) => {
        const posA = a.category ? (catPositionMap.get(a.category.id) ?? Infinity) : Infinity
        const posB = b.category ? (catPositionMap.get(b.category.id) ?? Infinity) : Infinity
        if (posA !== posB) return posA - posB
        return a.name.localeCompare(b.name, "de")
      }),
    [items, catPositionMap]
  )

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-2">
        <h1 className="text-xl font-semibold">Produkte</h1>
        <Button onClick={() => setCreateOpen(true)}>Produkt hinzufügen</Button>
      </div>
      {error && <div className="text-sm text-red-600">{error}</div>}
      {successMsg && (
        <div role="status" className="rounded bg-green-100 px-3 py-2 text-green-800">
          {successMsg}
        </div>
      )}
      <CreateProductCard
        open={createOpen}
        onOpenChange={setCreateOpen}
        categories={cats}
        jetons={jetons}
        posMode={posMode}
        onError={(msg) => setError(msg)}
        onCreated={(p) => {
          setItems((prev) => [p, ...prev])
          setSuccessMsg("Produkt erstellt")
          setTimeout(() => setSuccessMsg(null), 3000)
        }}
        createProduct={createProduct}
        adjustInventory={adjustInventory}
        updateJeton={updateJeton}
        uploadImage={uploadProductImage}
      />
      {!loading && sortedItems.length === 0 ? (
        <div className="text-muted-foreground py-12 text-center text-sm">
          Keine Produkte gefunden. Erstelle dein erstes Produkt.
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2 lg:grid-cols-3">
          {sortedItems.map((p) => (
            <ProductCardEditable
              key={p.id}
              product={p}
              categories={cats}
              jetons={jetons}
              posMode={posMode}
              onError={(msg) => setError(msg)}
              onUpdated={(next) => {
                setItems((prev) => prev.map((it) => (it.id === next.id ? next : it)))
              }}
              onDelete={async (id) => {
                await deleteHard(id)
                setItems((prev) => prev.filter((it) => it.id !== id))
              }}
              updatePrice={updatePrice}
              updateDeposit={updateDeposit}
              updateName={updateName}
              updateDescription={updateDescription}
              moveCategory={moveCategory}
              setActive={setActive}
              updateJeton={updateJeton}
              adjustInventory={adjustInventory}
              updateVariants={updateVariants}
              uploadImage={uploadProductImage}
              deleteImage={deleteProductImage}
            />
          ))}
        </div>
      )}
    </div>
  )
}

type ProductCardProps = {
  product: Product
  categories: Category[]
  jetons: Jeton[]
  posMode: PosFulfillmentMode
  onUpdated: (p: Product) => void
  onDelete: (id: string) => Promise<void>
  onError: (msg: string) => void
  updatePrice: (id: string, priceCents: number) => Promise<void>
  updateDeposit: (id: string, depositCents: number) => Promise<void>
  updateName: (id: string, name: string) => Promise<void>
  updateDescription: (id: string, description: string | null) => Promise<void>
  moveCategory: (id: string, categoryId: string) => Promise<void>
  setActive: (id: string, isActive: boolean) => Promise<void>
  updateJeton: (id: string, jetonId: string | null) => Promise<void>
  adjustInventory: (id: string, delta: number, variantId?: string) => Promise<void>
  updateVariants: (id: string, variantLabel: string | null, variants: ProductVariant[]) => Promise<ProductVariant[]>
  uploadImage: (id: string, file: File) => Promise<string>
  deleteImage: (id: string) => Promise<void>
}

function parseAdjustment(input: string): number | null {
  const norm = input.replace(",", ".").trim()
  if (!norm) return 0
  const val = Number(norm)
  if (!isFinite(val) || Number.isNaN(val)) return null
  return Math.round(val * 100)
}

function ProductCardEditable({
  product,
  categories,
  jetons,
  posMode,
  onUpdated,
  onDelete,
  onError,
  updatePrice,
  updateDeposit,
  updateName,
  updateDescription,
  moveCategory,
  setActive,
  updateJeton,
  adjustInventory,
  updateVariants,
  uploadImage,
  deleteImage,
}: ProductCardProps) {
  const [name, setName] = useState(product.name)
  const initialPrice = (product.priceCents / 100).toFixed(2)
  const [priceInput, setPriceInput] = useState(initialPrice)
  const initialDeposit = ((product.depositCents ?? 0) / 100).toFixed(2)
  const [depositInput, setDepositInput] = useState(initialDeposit)
  const [description, setDescription] = useState(product.description ?? "")
  const [categoryId, setCategoryId] = useState(product.category?.id ?? "")
  const [jetonId, setJetonId] = useState(product.jeton?.id ?? "")
  const [isActive, setIsActiveLocal] = useState(product.isActive)
  const [deltaInput, setDeltaInput] = useState("")
  const [status, setStatus] = useState<"idle" | "saving" | "saved" | "error">("idle")
  const [fieldError, setFieldError] = useState<string | null>(null)
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const categoryLabelId = `category-label-${product.id}`
  const jetonLabelId = `jeton-label-${product.id}`

  const [variantLabel, setVariantLabel] = useState(product.variantLabel ?? "")
  const [variants, setVariants] = useState<(ProductVariant & { adjInput: string; varDeltaInput: string })[]>(() =>
    (product.variants ?? []).map((v) => ({
      ...v,
      adjInput: v.priceAdjustmentCents === 0 ? "" : (v.priceAdjustmentCents / 100).toFixed(2),
      varDeltaInput: "",
    }))
  )

  useEffect(() => {
    setName(product.name)
    setPriceInput((product.priceCents / 100).toFixed(2))
    setDepositInput(((product.depositCents ?? 0) / 100).toFixed(2))
    setDescription(product.description ?? "")
    setCategoryId(product.category?.id ?? "")
    setJetonId(product.jeton?.id ?? "")
    setIsActiveLocal(product.isActive)
    setDeltaInput("")
    setVariantLabel(product.variantLabel ?? "")
    setVariants(
      (product.variants ?? []).map((v) => ({
        ...v,
        adjInput: v.priceAdjustmentCents === 0 ? "" : (v.priceAdjustmentCents / 100).toFixed(2),
        varDeltaInput: "",
      }))
    )
    setStatus("idle")
    setFieldError(null)
  }, [product])

  const currentStockValue = typeof product.stock === "number" ? product.stock : 0
  const currentStockLabel = typeof product.stock === "number" ? product.stock : "–"
  const parsedPrice = parsePrice(priceInput)
  const parsedDeposit = parsePrice(depositInput)
  const trimmedName = name.trim()
  const parsedDelta = deltaInput.trim() === "" ? 0 : parseInt(deltaInput, 10)
  const deltaValid = deltaInput.trim() === "" || !isNaN(parsedDelta)
  const newStock = currentStockValue + (deltaValid ? parsedDelta : 0)
  const jetonRequired = product.type === "simple" && posMode !== "QR_CODE" && isActive && !jetonId

  const priceDirty = parsedPrice != null ? parsedPrice !== product.priceCents : priceInput !== initialPrice
  const depositDirty =
    parsedDeposit != null ? parsedDeposit !== (product.depositCents ?? 0) : depositInput !== initialDeposit
  const deltaDirty = deltaInput.trim() !== "" && parsedDelta !== 0
  const trimmedDescription = description.trim()
  const descriptionDirty = trimmedDescription !== (product.description ?? "")

  const variantsHaveErrors = variants.some((v) => parseAdjustment(v.adjInput) === null || !v.value.trim())
  const variantsDirty =
    product.type === "simple" &&
    (variantLabel !== (product.variantLabel ?? "") ||
      variants.length !== (product.variants ?? []).length ||
      variants.some((v, i) => {
        const orig = (product.variants ?? [])[i]
        if (!orig) return true
        const adj = parseAdjustment(v.adjInput)
        return (
          v.id !== orig.id ||
          v.value.trim() !== orig.value ||
          adj !== orig.priceAdjustmentCents ||
          v.isActive !== orig.isActive
        )
      }))
  const variantDeltas = variants.filter((v) => v.varDeltaInput.trim() !== "" && parseInt(v.varDeltaInput, 10) !== 0)
  const variantDeltasDirty = variantDeltas.length > 0

  const dirty =
    trimmedName !== product.name ||
    priceDirty ||
    (product.type === "simple" && depositDirty) ||
    descriptionDirty ||
    categoryId !== (product.category?.id ?? "") ||
    jetonId !== (product.jeton?.id ?? "") ||
    isActive !== product.isActive ||
    deltaDirty ||
    variantsDirty ||
    variantDeltasDirty

  const valid =
    trimmedName.length > 0 &&
    parsedPrice != null &&
    parsedPrice >= 0 &&
    (product.type !== "simple" || (parsedDeposit != null && parsedDeposit >= 0)) &&
    deltaValid &&
    newStock >= 0 &&
    (!jetonRequired || !!jetonId) &&
    !variantsHaveErrors

  function onPriceBlur() {
    // Normalize to two decimals if valid
    if (parsedPrice != null) {
      setPriceInput((parsedPrice / 100).toFixed(2))
    }
  }

  function onDepositBlur() {
    if (parsedDeposit != null) {
      setDepositInput((parsedDeposit / 100).toFixed(2))
    }
  }

  async function handleSave() {
    setFieldError(null)
    if (!valid) {
      setStatus("error")
      setFieldError("Bitte Eingaben prüfen.")
      return
    }
    if (jetonRequired) {
      setStatus("error")
      setFieldError("Im Jeton-Modus benötigen aktive Produkte einen Jeton.")
      onError("Im Jeton-Modus benötigen aktive Produkte einen Jeton.")
      return
    }
    setStatus("saving")
    try {
      const updates: Array<Promise<void>> = []
      if (trimmedName !== product.name) updates.push(updateName(product.id, trimmedName))
      if (parsedPrice != null && parsedPrice !== product.priceCents) updates.push(updatePrice(product.id, parsedPrice))
      if (product.type === "simple" && parsedDeposit != null && parsedDeposit !== (product.depositCents ?? 0))
        updates.push(updateDeposit(product.id, parsedDeposit))
      if (descriptionDirty) updates.push(updateDescription(product.id, trimmedDescription || null))
      if (categoryId !== (product.category?.id ?? "")) updates.push(moveCategory(product.id, categoryId))
      if (jetonId !== (product.jeton?.id ?? "")) updates.push(updateJeton(product.id, jetonId || null))
      if (isActive !== product.isActive) {
        if (product.type === "simple" && posMode !== "QR_CODE" && isActive && !jetonId) {
          throw new Error("Im Jeton-Modus benötigen aktive Produkte einen Jeton.")
        }
        updates.push(setActive(product.id, isActive))
      }
      if (deltaDirty) updates.push(adjustInventory(product.id, parsedDelta))
      await Promise.all(updates)

      // The server response carries new variants' ids; without them a second
      // save would re-create instead of update.
      let updatedVariants = product.variants
      if (variantsDirty) {
        const saved = await updateVariants(
          product.id,
          variantLabel.trim() || null,
          variants.map((v, idx) => ({
            id: v.id,
            value: v.value.trim(),
            priceAdjustmentCents: parseAdjustment(v.adjInput) ?? 0,
            isActive: v.isActive,
            sequence: idx + 1,
          }))
        )
        const stockById = new Map(variants.filter((v) => v.id).map((v) => [v.id, v.stock]))
        updatedVariants = saved.map((v) => ({ ...v, stock: stockById.get(v.id) ?? v.stock }))
      }

      for (const v of variants) {
        const varDelta = v.varDeltaInput.trim() === "" ? 0 : parseInt(v.varDeltaInput, 10)
        if (!isNaN(varDelta) && varDelta !== 0 && v.id) {
          await adjustInventory(product.id, varDelta, v.id)
          updatedVariants = updatedVariants?.map((uv) =>
            uv.id === v.id && typeof uv.stock === "number" ? { ...uv, stock: uv.stock + varDelta } : uv
          )
        }
      }

      const updated: Product = {
        ...product,
        name: trimmedName,
        priceCents: parsedPrice ?? product.priceCents,
        depositCents:
          product.type === "simple" ? (parsedDeposit ?? product.depositCents ?? 0) : (product.depositCents ?? 0),
        description: descriptionDirty ? trimmedDescription || null : product.description,
        category: categoryId
          ? { id: categoryId, name: categories.find((c) => c.id === categoryId)?.name ?? product.category?.name ?? "" }
          : undefined,
        jeton: jetonId ? jetons.find((j) => j.id === jetonId) : undefined,
        isActive,
        stock: typeof product.stock === "number" ? product.stock + parsedDelta : product.stock,
        variantLabel: variantLabel.trim() || null,
        variants: updatedVariants,
      }
      setDeltaInput("")
      setVariants(
        (updatedVariants ?? []).map((v) => ({
          ...v,
          adjInput: v.priceAdjustmentCents === 0 ? "" : (v.priceAdjustmentCents / 100).toFixed(2),
          varDeltaInput: "",
        }))
      )
      setStatus("saved")
      onUpdated(updated)
      setTimeout(() => setStatus("idle"), 2000)
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Speichern fehlgeschlagen"
      setStatus("error")
      setFieldError(msg)
      onError(msg)
    }
  }

  const savingDisabled = !dirty || !valid || status === "saving"

  return (
    <>
      <Card className="border-border flex h-full flex-col gap-0 overflow-hidden rounded-[11px] border p-0 pb-3">
        <CardHeader className="p-2">
          <div className="relative">
            <ImageUpload
              currentImageUrl={product.image}
              onUpload={async (file) => {
                const url = await uploadImage(product.id, file)
                onUpdated({ ...product, image: url })
                return url
              }}
              onRemove={async () => {
                await deleteImage(product.id)
                onUpdated({ ...product, image: null })
              }}
              disabled={status === "saving"}
            />
            {!isActive && (
              <div className="pointer-events-none absolute inset-x-0 top-0 z-10 grid aspect-video place-items-center rounded-[11px] bg-black/55">
                <span className="rounded-full bg-zinc-700 px-3 py-1 text-sm font-medium text-white">Nicht aktiv</span>
              </div>
            )}
          </div>
        </CardHeader>
        <CardContent className="flex flex-1 flex-col gap-4 px-3">
          <div className="space-y-1">
            <Label className="sr-only" htmlFor={`name-${product.id}`}>
              Produktname
            </Label>
            <Input
              id={`name-${product.id}`}
              value={name}
              placeholder="Produktname"
              onChange={(e) => {
                setName(e.target.value)
                setStatus("idle")
                setFieldError(null)
              }}
            />
          </div>

          <div className="space-y-1">
            <Label htmlFor={`description-${product.id}`}>Beschreibung</Label>
            <Textarea
              id={`description-${product.id}`}
              value={description}
              maxLength={500}
              rows={3}
              placeholder="Optional: Details, Hinweise…"
              onChange={(e) => {
                setDescription(e.target.value)
                setStatus("idle")
                setFieldError(null)
              }}
            />
            <p className="text-muted-foreground text-xs">{description.length}/500</p>
          </div>

          <div className="space-y-1">
            <Label htmlFor={`price-${product.id}`}>Preis (CHF)</Label>
            <Input
              id={`price-${product.id}`}
              inputMode="decimal"
              value={priceInput}
              onChange={(e) => {
                setPriceInput(e.target.value)
                setStatus("idle")
                setFieldError(null)
              }}
              onBlur={onPriceBlur}
              aria-invalid={parsedPrice == null || parsedPrice < 0}
            />
            {(parsedPrice == null || parsedPrice < 0) && (
              <p className="text-destructive text-xs">Preis muss 0 oder höher sein.</p>
            )}
          </div>

          {product.type === "simple" && (
            <div className="space-y-1">
              <Label htmlFor={`deposit-${product.id}`}>Depot (CHF)</Label>
              <div className="flex items-center gap-2">
                <Input
                  id={`deposit-${product.id}`}
                  inputMode="decimal"
                  value={depositInput}
                  onChange={(e) => {
                    setDepositInput(e.target.value)
                    setStatus("idle")
                    setFieldError(null)
                  }}
                  onBlur={onDepositBlur}
                  aria-invalid={parsedDeposit == null || parsedDeposit < 0}
                  className="flex-1"
                />
                <DepotInfo />
              </div>
              {(parsedDeposit == null || parsedDeposit < 0) && (
                <p className="text-destructive text-xs">Depot muss 0 oder höher sein.</p>
              )}
            </div>
          )}

          <div className="space-y-1">
            <Label id={categoryLabelId} htmlFor={`category-${product.id}`}>
              Kategorie
            </Label>
            <Select
              value={categoryId || undefined}
              onValueChange={(cid) => {
                setCategoryId(cid)
                setStatus("idle")
                setFieldError(null)
              }}
            >
              <SelectTrigger id={`category-${product.id}`} className="h-10" aria-labelledby={categoryLabelId}>
                <SelectValue placeholder="Kategorie wählen" />
              </SelectTrigger>
              <SelectContent>
                {categories.map((c) => (
                  <SelectItem key={c.id} value={c.id}>
                    {c.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {posMode !== "QR_CODE" && (
            <div className="space-y-1">
              <Label id={jetonLabelId} htmlFor={`jeton-${product.id}`}>
                Jeton
              </Label>
              <Select
                value={jetonId || NO_JETON_VALUE}
                onValueChange={(jid) => {
                  setJetonId(jid === NO_JETON_VALUE ? "" : jid)
                  setStatus("idle")
                  setFieldError(null)
                }}
              >
                <SelectTrigger id={`jeton-${product.id}`} className="h-10" aria-labelledby={jetonLabelId}>
                  <SelectValue placeholder="Jeton wählen" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NO_JETON_VALUE}>Kein Jeton</SelectItem>
                  {jetons.map((j) => (
                    <SelectItem key={j.id} value={j.id}>
                      <span
                        aria-hidden
                        className="mr-2 inline-block h-3 w-3 rounded-full align-middle"
                        style={{ backgroundColor: j.color }}
                      />
                      {j.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {jetonRequired && <p className="text-destructive text-xs">Jeton ist Pflicht im Jeton-Modus.</p>}
            </div>
          )}
          {product.type === "simple" && (
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <Label className="text-sm font-medium">Varianten</Label>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="h-7 text-xs"
                  onClick={() => {
                    setVariants((prev) => [
                      ...prev,
                      {
                        value: "",
                        priceAdjustmentCents: 0,
                        isActive: true,
                        sequence: prev.length + 1,
                        adjInput: "",
                        varDeltaInput: "",
                      },
                    ])
                    setStatus("idle")
                    setFieldError(null)
                  }}
                >
                  <Plus className="h-3 w-3" />
                  Variante
                </Button>
              </div>

              {variants.length > 0 && (
                <div className="space-y-1">
                  <Label htmlFor={`variant-label-${product.id}`} className="text-xs">
                    Bezeichnung (z. B. Grösse)
                  </Label>
                  <Input
                    id={`variant-label-${product.id}`}
                    value={variantLabel}
                    placeholder="Variante"
                    maxLength={20}
                    onChange={(e) => {
                      setVariantLabel(e.target.value)
                      setStatus("idle")
                      setFieldError(null)
                    }}
                  />
                </div>
              )}

              {variants.map((v, idx) => {
                const adj = parseAdjustment(v.adjInput)
                return (
                  <div key={idx} className="space-y-2 rounded-lg border p-2">
                    <div className="flex items-center gap-2">
                      <Input
                        value={v.value}
                        placeholder="Wert (z. B. M)"
                        maxLength={20}
                        className="flex-1"
                        onChange={(e) => {
                          setVariants((prev) => prev.map((x, i) => (i === idx ? { ...x, value: e.target.value } : x)))
                          setStatus("idle")
                          setFieldError(null)
                        }}
                      />
                      <Input
                        value={v.adjInput}
                        placeholder="Aufpreis"
                        inputMode="decimal"
                        className="w-24"
                        aria-label="Aufpreis (CHF)"
                        onChange={(e) => {
                          setVariants((prev) =>
                            prev.map((x, i) => (i === idx ? { ...x, adjInput: e.target.value } : x))
                          )
                          setStatus("idle")
                          setFieldError(null)
                        }}
                      />
                      <div className="flex shrink-0 items-center gap-1">
                        <Label htmlFor={`var-active-${product.id}-${idx}`} className="sr-only">
                          Aktiv
                        </Label>
                        <Switch
                          id={`var-active-${product.id}-${idx}`}
                          checked={v.isActive}
                          onCheckedChange={(checked) => {
                            setVariants((prev) => prev.map((x, i) => (i === idx ? { ...x, isActive: checked } : x)))
                            setStatus("idle")
                            setFieldError(null)
                          }}
                          aria-label="Variante aktiv"
                        />
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="h-8 w-8 shrink-0"
                          onClick={() => {
                            setVariants((prev) => prev.filter((_, i) => i !== idx))
                            setStatus("idle")
                            setFieldError(null)
                          }}
                          aria-label="Variante entfernen"
                        >
                          <Minus className="h-4 w-4" />
                        </Button>
                      </div>
                    </div>
                    {adj === null && v.adjInput.trim() !== "" && (
                      <p className="text-destructive text-xs">Ungültiger Aufpreis.</p>
                    )}
                    <div className="flex items-center gap-2 text-xs">
                      <span className="text-muted-foreground shrink-0">
                        Lager:{" "}
                        <span className="text-foreground rounded border px-1.5 py-0.5">
                          {typeof v.stock === "number" ? v.stock : "–"}
                        </span>
                      </span>
                      {v.id && (
                        <div className="flex items-center gap-1">
                          <Button
                            type="button"
                            variant="outline"
                            size="icon"
                            className="h-6 w-6"
                            onClick={() => {
                              const cur = v.varDeltaInput.trim() === "" ? 0 : parseInt(v.varDeltaInput, 10) || 0
                              setVariants((prev) =>
                                prev.map((x, i) => (i === idx ? { ...x, varDeltaInput: String(cur - 1) } : x))
                              )
                            }}
                            aria-label="Variante Bestand verringern"
                          >
                            <Minus className="h-3 w-3" />
                          </Button>
                          <Input
                            type="text"
                            inputMode="numeric"
                            value={v.varDeltaInput}
                            placeholder="0"
                            className="h-6 w-14 [appearance:textfield] text-center text-xs [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
                            onChange={(e) => {
                              setVariants((prev) =>
                                prev.map((x, i) => (i === idx ? { ...x, varDeltaInput: e.target.value } : x))
                              )
                              setStatus("idle")
                              setFieldError(null)
                            }}
                            aria-label="Variante Bestand anpassen"
                          />
                          <Button
                            type="button"
                            variant="outline"
                            size="icon"
                            className="h-6 w-6"
                            onClick={() => {
                              const cur = v.varDeltaInput.trim() === "" ? 0 : parseInt(v.varDeltaInput, 10) || 0
                              setVariants((prev) =>
                                prev.map((x, i) => (i === idx ? { ...x, varDeltaInput: String(cur + 1) } : x))
                              )
                            }}
                            aria-label="Variante Bestand erhöhen"
                          >
                            <Plus className="h-3 w-3" />
                          </Button>
                        </div>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          )}

          {product.type === "simple" && variants.length === 0 && (
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2 md:items-end">
              <div className="space-y-1">
                <Label>Lager</Label>
                <div className="text-muted-foreground text-sm">
                  Aktuell: <span className="text-foreground rounded-md border px-2 py-1">{currentStockLabel}</span>
                </div>
              </div>
              <div className="space-y-1">
                <Label htmlFor={`stock-delta-${product.id}`}>Anpassen</Label>
                <div className="flex items-center gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    onClick={() => {
                      const current = deltaInput.trim() === "" ? 0 : parseInt(deltaInput, 10) || 0
                      setDeltaInput(String(current - 1))
                      setStatus("idle")
                      setFieldError(null)
                    }}
                    aria-label="Bestand verringern"
                  >
                    <Minus className="h-4 w-4" />
                  </Button>
                  <Input
                    id={`stock-delta-${product.id}`}
                    type="text"
                    inputMode="numeric"
                    value={deltaInput}
                    placeholder="0"
                    onChange={(e) => {
                      setDeltaInput(e.target.value)
                      setStatus("idle")
                      setFieldError(null)
                    }}
                    className="w-full [appearance:textfield] text-center [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
                    aria-label="Bestand ändern"
                  />
                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    onClick={() => {
                      const current = deltaInput.trim() === "" ? 0 : parseInt(deltaInput, 10) || 0
                      setDeltaInput(String(current + 1))
                      setStatus("idle")
                      setFieldError(null)
                    }}
                    aria-label="Bestand erhöhen"
                  >
                    <Plus className="h-4 w-4" />
                  </Button>
                </div>
                {!deltaValid && <p className="text-destructive text-xs">Bitte eine gültige Zahl eingeben.</p>}
                {deltaValid && newStock < 0 && (
                  <p className="text-destructive text-xs">Bestand darf nicht negativ sein.</p>
                )}
              </div>
            </div>
          )}
          <div className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-2">
              <Label htmlFor={`active-${product.id}`} className="text-sm font-medium">
                Aktiv
              </Label>
              <Switch
                id={`active-${product.id}`}
                checked={isActive}
                onCheckedChange={(checked) => {
                  setIsActiveLocal(checked)
                  setStatus("idle")
                  setFieldError(null)
                }}
                aria-label="Aktiv"
              />
            </div>
          </div>

          <div className="mt-auto flex flex-col gap-2 pt-3 text-sm md:flex-row md:items-center md:justify-between">
            {fieldError && <p className="text-destructive text-xs">{fieldError}</p>}
            <div className="flex flex-col gap-2 md:ml-auto md:flex-row">
              <Button
                onClick={() => setShowDeleteConfirm(true)}
                variant="secondary"
                className="text-destructive w-full md:w-auto"
              >
                Löschen
              </Button>
              <Button onClick={handleSave} disabled={savingDisabled} className="w-full md:w-auto">
                Speichern
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      <AlertDialog open={showDeleteConfirm} onOpenChange={setShowDeleteConfirm}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Produkt löschen?</AlertDialogTitle>
            <AlertDialogDescription>Diese Aktion kann nicht rückgängig gemacht werden.</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Abbrechen</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={async () => {
                try {
                  await onDelete(product.id)
                  setShowDeleteConfirm(false)
                } catch (e: unknown) {
                  const msg = e instanceof Error ? e.message : "Löschen fehlgeschlagen"
                  onError(msg)
                }
              }}
            >
              Löschen
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function parsePrice(input: string): number | null {
  const norm = input.replace(",", ".").trim()
  if (!norm) return null
  const val = Number(norm)
  if (!isFinite(val) || Number.isNaN(val)) return null
  return Math.round(val * 100)
}

type CreateProductCardProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  categories: Category[]
  jetons: Jeton[]
  posMode: PosFulfillmentMode
  onCreated: (p: Product) => void
  onError: (msg: string) => void
  createProduct: (payload: {
    name: string
    priceCents: number
    depositCents?: number
    categoryId: string
    type: "simple" | "bundle"
    isActive: boolean
    image?: string | null
    description?: string | null
    jetonId?: string | null
  }) => Promise<Partial<Product> | null>
  adjustInventory: (id: string, delta: number) => Promise<void>
  updateJeton: (id: string, jetonId: string | null) => Promise<void>
  uploadImage: (id: string, file: File) => Promise<string>
}

function CreateProductCard({
  open,
  onOpenChange,
  categories,
  jetons,
  posMode,
  onCreated,
  onError,
  createProduct,
  adjustInventory,
  updateJeton,
  uploadImage,
}: CreateProductCardProps) {
  const [saving, setSaving] = useState(false)
  const [submitted, setSubmitted] = useState(false)
  const [name, setName] = useState("")
  const [priceInput, setPriceInput] = useState("")
  const [depositInput, setDepositInput] = useState("")
  const [categoryId, setCategoryId] = useState<string>("")
  const [type, setType] = useState<"simple" | "bundle">("simple")
  const [jetonId, setJetonId] = useState<string>("")
  const [isActive, setIsActive] = useState(true)
  const [initialStock, setInitialStock] = useState<number | "">(0)
  const [error, setError] = useState<string | null>(null)
  const [imageFile, setImageFile] = useState<File | null>(null)
  const [description, setDescription] = useState<string>("")
  const [step, setStep] = useState<1 | 2>(1)

  const parsedPrice = parsePrice(priceInput)
  const parsedDeposit = depositInput.trim() === "" ? 0 : parsePrice(depositInput)
  const depositValid = parsedDeposit != null && parsedDeposit >= 0
  const trimmedName = name.trim()
  const jetonRequired = type === "simple" && posMode !== "QR_CODE" && isActive && !jetonId
  const stockValid = initialStock === "" || initialStock >= 0
  const valid =
    trimmedName.length > 0 &&
    parsedPrice != null &&
    parsedPrice >= 0 &&
    depositValid &&
    !!categoryId &&
    !jetonRequired &&
    stockValid &&
    (type === "simple" || type === "bundle")

  const reset = () => {
    setName("")
    setPriceInput("")
    setDepositInput("")
    setCategoryId("")
    setType("simple")
    setJetonId("")
    setIsActive(true)
    setInitialStock(0)
    setError(null)
    setImageFile(null)
    setDescription("")
    setSubmitted(false)
    setStep(1)
  }

  const handleCreate = async () => {
    setSubmitted(true)
    if (!valid) {
      setError("Bitte alle Pflichtfelder ausfüllen.")
      return
    }
    setSaving(true)
    setError(null)
    try {
      const stockValue = initialStock === "" ? 0 : initialStock
      const trimmedDescription = description.trim()
      const res = await createProduct({
        name: trimmedName,
        priceCents: parsedPrice ?? 0,
        depositCents: type === "simple" ? (parsedDeposit ?? 0) : 0,
        categoryId,
        type,
        isActive,
        image: null,
        description: trimmedDescription || null,
        jetonId: jetonId || undefined,
      })
      const newId = res?.id ?? crypto.randomUUID?.() ?? `${Date.now()}`
      const category = categories.find((c) => c.id === categoryId)
      const created: Product = {
        id: newId,
        name: trimmedName,
        priceCents: parsedPrice ?? 0,
        depositCents: type === "simple" ? (parsedDeposit ?? 0) : 0,
        isActive,
        category: category ? { id: categoryId, name: category.name } : undefined,
        jeton: jetonId ? jetons.find((j) => j.id === jetonId) : undefined,
        stock: stockValue,
        type,
        image: null,
        description: trimmedDescription || null,
      }
      if (imageFile && res?.id) {
        try {
          created.image = await uploadImage(res.id, imageFile)
        } catch {
          onError("Produkt erstellt, aber das Bild konnte nicht hochgeladen werden.")
        }
      }
      if (stockValue !== 0 && res?.id) {
        await adjustInventory(res.id, stockValue)
        created.stock = stockValue
      }
      if (jetonId && res?.id) {
        try {
          await updateJeton(res.id, jetonId)
          created.jeton = jetons.find((j) => j.id === jetonId)
        } catch {
          // ignore jeton assignment error for now; surfaced via onError already
        }
      } else if (!jetonId && res?.id && posMode !== "QR_CODE" && isActive && type === "simple") {
        setError("Im Jeton-Modus benötigen aktive Produkte einen Jeton.")
      }
      onCreated(created)
      onOpenChange(false)
      reset()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Erstellen fehlgeschlagen"
      setError(msg)
      onError(msg)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Neues Produkt anlegen</DialogTitle>
          <DialogDescription>
            {step === 1 ? "Schritt 1 von 2 · Details" : "Schritt 2 von 2 · Produktbild"}
          </DialogDescription>
        </DialogHeader>
        {step === 1 && (
          <div className="space-y-3">
            <div className="space-y-1">
              <Label htmlFor="new-name">Produktname</Label>
              <Input
                id="new-name"
                value={name}
                placeholder="Produktname"
                maxLength={20}
                onChange={(e) => setName(e.target.value)}
                autoFocus
                aria-invalid={submitted && !trimmedName}
              />
              {submitted && !trimmedName && <p className="text-destructive text-xs">Name ist erforderlich.</p>}
            </div>
            <div className="space-y-1">
              <Label htmlFor="new-price">Preis (CHF)</Label>
              <Input
                id="new-price"
                inputMode="decimal"
                value={priceInput}
                onChange={(e) => setPriceInput(e.target.value)}
                aria-invalid={submitted && (parsedPrice == null || parsedPrice < 0)}
              />
              {submitted && (parsedPrice == null || parsedPrice < 0) && (
                <p className="text-destructive text-xs">Preis muss 0 oder höher sein.</p>
              )}
            </div>
            {type === "simple" && (
              <div className="space-y-1">
                <Label htmlFor="new-deposit">Depot (CHF)</Label>
                <div className="flex items-center gap-2">
                  <Input
                    id="new-deposit"
                    inputMode="decimal"
                    value={depositInput}
                    onChange={(e) => setDepositInput(e.target.value)}
                    placeholder="0.00"
                    aria-invalid={submitted && !depositValid}
                    className="flex-1"
                  />
                  <DepotInfo />
                </div>
                {submitted && !depositValid && (
                  <p className="text-destructive text-xs">Depot muss 0 oder höher sein.</p>
                )}
              </div>
            )}
            <div className="space-y-1">
              <Label htmlFor="new-category">Kategorie</Label>
              <Select value={categoryId || undefined} onValueChange={setCategoryId}>
                <SelectTrigger id="new-category">
                  <SelectValue placeholder="Kategorie wählen" />
                </SelectTrigger>
                <SelectContent>
                  {categories.map((c) => (
                    <SelectItem key={c.id} value={c.id}>
                      {c.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label htmlFor="new-type">Typ</Label>
              <Select value={type} onValueChange={(v) => setType(v as "simple" | "bundle")}>
                <SelectTrigger id="new-type">
                  <SelectValue placeholder="Typ wählen" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="bundle">Bundle</SelectItem>
                  <SelectItem value="simple">Einfaches Produkt</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label htmlFor="new-description">Beschreibung (optional)</Label>
              <Textarea
                id="new-description"
                value={description}
                maxLength={500}
                rows={3}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="Optional: Details, Hinweise…"
              />
              <p className="text-muted-foreground text-xs">{description.length}/500</p>
            </div>
            {posMode !== "QR_CODE" && (
              <div className="space-y-1">
                <Label htmlFor="new-jeton">Jeton</Label>
                <Select
                  value={jetonId || NO_JETON_VALUE}
                  onValueChange={(v) => setJetonId(v === NO_JETON_VALUE ? "" : v)}
                >
                  <SelectTrigger id="new-jeton">
                    <SelectValue placeholder="Jeton wählen" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NO_JETON_VALUE}>Kein Jeton</SelectItem>
                    {jetons.map((j) => (
                      <SelectItem key={j.id} value={j.id}>
                        <span
                          aria-hidden
                          className="mr-2 inline-block h-3 w-3 rounded-full align-middle"
                          style={{ backgroundColor: j.color }}
                        />
                        {j.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {submitted && jetonRequired && (
                  <p className="text-destructive text-xs">Jeton ist Pflicht im Jeton-Modus.</p>
                )}
              </div>
            )}
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Label htmlFor="new-active" className="text-sm font-medium">
                  Aktiv
                </Label>
                <Switch id="new-active" checked={isActive} onCheckedChange={setIsActive} aria-label="Aktiv" />
              </div>
              <div className="space-y-1 text-right">
                <Label htmlFor="new-stock" className="text-sm">
                  Start-Lager
                </Label>
                <Input
                  id="new-stock"
                  type="number"
                  value={initialStock}
                  onChange={(e) => {
                    const raw = e.target.value
                    setInitialStock(raw === "" ? "" : parseInt(raw, 10))
                  }}
                  className="w-28"
                />
              </div>
            </div>
            {submitted && !stockValid && <p className="text-destructive text-xs">Bestand darf nicht negativ sein.</p>}
          </div>
        )}
        {step === 2 && (
          <div className="space-y-2">
            <Label htmlFor="new-image">Produktbild (optional)</Label>
            <ProductImageCrop onChange={setImageFile} />
          </div>
        )}
        {error && <p className="text-destructive text-sm">{error}</p>}
        <DialogFooter className="mt-2">
          {step === 1 ? (
            <>
              <Button
                variant="outline"
                onClick={() => {
                  onOpenChange(false)
                  reset()
                }}
              >
                Abbrechen
              </Button>
              <Button
                onClick={() => {
                  setSubmitted(true)
                  if (valid) setStep(2)
                }}
              >
                Weiter
              </Button>
            </>
          ) : (
            <>
              <Button variant="outline" onClick={() => setStep(1)} disabled={saving}>
                Zurück
              </Button>
              <Button onClick={handleCreate} disabled={saving || !valid}>
                {saving ? "Speichern…" : "Erstellen"}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// CSRF helper now centralized in lib/csrf
