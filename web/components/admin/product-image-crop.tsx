"use client"

import { Loader2, RotateCcw, RotateCw, Trash2, Upload, ZoomIn, ZoomOut } from "lucide-react"
import NextImage from "next/image"
import { type ComponentType, useEffect, useRef, useState } from "react"
import CropperBase, { type Area, type CropperProps } from "react-easy-crop"
import { Button } from "@/components/ui/button"
import { Slider } from "@/components/ui/slider"
import { type CropArea, getCroppedBlob } from "@/lib/crop-image"

// react-easy-crop ships a class component whose types don't line up with React 19's
// JSX typing; alias it through ComponentType so it can be used as a JSX element.
const Cropper = CropperBase as unknown as ComponentType<Partial<CropperProps>>

const ACCEPTED_TYPES = "image/jpeg,image/png,image/webp"
const MAX_SIZE = 5 * 1024 * 1024 // 5 MB
const DEFAULT_ASPECT = 16 / 9

type Props = {
  aspect?: number
  disabled?: boolean
  // Deferred mode (e.g. two-step create dialogs): fires with the cropped WebP
  // whenever the crop settles, or null when cleared. The parent uploads it later.
  onChange?: (file: File | null) => void
  // Immediate mode (edit flows): shows the persisted image and only uploads once
  // the admin confirms the crop.
  currentImageUrl?: string | null
  onUpload?: (file: File) => Promise<string>
  onRemove?: () => Promise<void>
}

export function ProductImageCrop({
  aspect = DEFAULT_ASPECT,
  disabled,
  onChange,
  currentImageUrl,
  onUpload,
  onRemove,
}: Props) {
  const immediate = !!onUpload
  const [src, setSrc] = useState<string | null>(null)
  const [crop, setCrop] = useState({ x: 0, y: 0 })
  const [zoom, setZoom] = useState(1)
  const [rotation, setRotation] = useState(0)
  const [area, setArea] = useState<CropArea | null>(null)
  const [preview, setPreview] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [removing, setRemoving] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  // Keep callbacks, the pending file, and revocable URLs in refs so the crop
  // effect stays stable across re-renders.
  const onChangeRef = useRef(onChange)
  onChangeRef.current = onChange
  const srcRef = useRef<string | null>(null)
  const previewRef = useRef<string | null>(null)
  const fileRef = useRef<File | null>(null)

  const busy = disabled || saving || removing

  const pickFile = (file: File) => {
    setError(null)
    if (!file.type.match(/^image\/(jpeg|png|webp)$/)) {
      setError("Nur JPEG, PNG oder WebP erlaubt.")
      return
    }
    if (file.size > MAX_SIZE) {
      setError("Datei darf max. 5 MB gross sein.")
      return
    }
    if (srcRef.current) URL.revokeObjectURL(srcRef.current)
    const url = URL.createObjectURL(file)
    srcRef.current = url
    setSrc(url)
    setCrop({ x: 0, y: 0 })
    setZoom(1)
    setRotation(0)
  }

  const resetCropState = () => {
    if (srcRef.current) URL.revokeObjectURL(srcRef.current)
    if (previewRef.current) URL.revokeObjectURL(previewRef.current)
    srcRef.current = null
    previewRef.current = null
    fileRef.current = null
    setSrc(null)
    setArea(null)
    setPreview(null)
    setError(null)
    if (inputRef.current) inputRef.current.value = ""
  }

  // Deferred mode: clears the picked image entirely. Immediate mode: cancels
  // the in-progress crop and falls back to the currently persisted image.
  const cancel = () => {
    resetCropState()
    if (!immediate) onChangeRef.current?.(null)
  }

  const handleRemove = async () => {
    if (!onRemove) return
    setError(null)
    setRemoving(true)
    try {
      await onRemove()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Löschen fehlgeschlagen")
    } finally {
      setRemoving(false)
    }
  }

  const confirmUpload = async () => {
    if (!onUpload || !fileRef.current) return
    setError(null)
    setSaving(true)
    try {
      await onUpload(fileRef.current)
      resetCropState()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Upload fehlgeschlagen")
    } finally {
      setSaving(false)
    }
  }

  useEffect(() => {
    if (!src || !area) return
    let cancelled = false
    ;(async () => {
      try {
        const blob = await getCroppedBlob(src, area, rotation)
        if (cancelled) return
        const file = new File([blob], "product.webp", { type: "image/webp" })
        const url = URL.createObjectURL(blob)
        if (previewRef.current) URL.revokeObjectURL(previewRef.current)
        previewRef.current = url
        setPreview(url)
        fileRef.current = file
        if (!immediate) onChangeRef.current?.(file)
      } catch {
        // ignore transient crop failures while dragging
      }
    })()
    return () => {
      cancelled = true
    }
  }, [src, area, rotation])

  useEffect(
    () => () => {
      if (srcRef.current) URL.revokeObjectURL(srcRef.current)
      if (previewRef.current) URL.revokeObjectURL(previewRef.current)
    },
    []
  )

  const boxStyle = { aspectRatio: aspect }

  // Immediate mode with a persisted image and nothing being edited right now:
  // show the current image plus replace/remove actions.
  if (immediate && !src && currentImageUrl) {
    return (
      <div className="space-y-2">
        <div
          role="button"
          tabIndex={0}
          className="bg-muted relative w-full cursor-pointer overflow-hidden rounded-[11px]"
          style={boxStyle}
          onClick={() => !busy && inputRef.current?.click()}
          onKeyDown={(e) => {
            if (!busy && (e.key === "Enter" || e.key === " ")) {
              e.preventDefault()
              inputRef.current?.click()
            }
          }}
          onDrop={(e) => {
            e.preventDefault()
            if (busy) return
            const f = e.dataTransfer.files?.[0]
            if (f) pickFile(f)
          }}
          onDragOver={(e) => e.preventDefault()}
          aria-label="Bild ersetzen"
        >
          <NextImage
            src={currentImageUrl}
            alt="Produktbild"
            fill
            sizes="(max-width: 768px) 100vw, (max-width: 1280px) 50vw, 25vw"
            quality={90}
            className="object-cover"
            unoptimized={
              currentImageUrl.startsWith("blob:") ||
              currentImageUrl.includes("localhost") ||
              currentImageUrl.includes("127.0.0.1")
            }
          />
        </div>
        <div className="flex gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="flex-1"
            onClick={() => inputRef.current?.click()}
            disabled={busy}
          >
            Bild ersetzen
          </Button>
          <Button type="button" variant="outline" size="sm" onClick={handleRemove} disabled={busy}>
            {removing ? <Loader2 className="mr-1 h-3 w-3 animate-spin" /> : <Trash2 className="mr-1 h-3 w-3" />}
            Entfernen
          </Button>
        </div>
        <input
          ref={inputRef}
          type="file"
          accept={ACCEPTED_TYPES}
          className="hidden"
          onChange={(e) => {
            const f = e.target.files?.[0]
            if (f) pickFile(f)
            if (inputRef.current) inputRef.current.value = ""
          }}
        />
        {error && <p className="text-destructive text-xs">{error}</p>}
      </div>
    )
  }

  if (!src) {
    return (
      <div className="space-y-1">
        <div
          role="button"
          tabIndex={0}
          onClick={() => !busy && inputRef.current?.click()}
          onKeyDown={(e) => {
            if (!busy && (e.key === "Enter" || e.key === " ")) {
              e.preventDefault()
              inputRef.current?.click()
            }
          }}
          onDrop={(e) => {
            e.preventDefault()
            const f = e.dataTransfer.files?.[0]
            if (f) pickFile(f)
          }}
          onDragOver={(e) => e.preventDefault()}
          className="border-border text-muted-foreground hover:bg-muted/50 flex w-full cursor-pointer flex-col items-center justify-center gap-2 rounded-[11px] border border-dashed"
          style={boxStyle}
        >
          <Upload className="size-6" />
          <span className="text-sm">Foto auswählen oder hierher ziehen</span>
          <span className="text-xs">JPEG, PNG oder WebP · max. 5 MB</span>
        </div>
        <input
          ref={inputRef}
          type="file"
          accept={ACCEPTED_TYPES}
          className="hidden"
          onChange={(e) => {
            const f = e.target.files?.[0]
            if (f) pickFile(f)
            if (inputRef.current) inputRef.current.value = ""
          }}
        />
        {error && <p className="text-destructive text-xs">{error}</p>}
      </div>
    )
  }

  return (
    <div className="space-y-3">
      <div className="bg-muted relative w-full overflow-hidden rounded-[11px]" style={boxStyle}>
        <Cropper
          image={src}
          crop={crop}
          zoom={zoom}
          rotation={rotation}
          aspect={aspect}
          showGrid
          onCropChange={setCrop}
          onZoomChange={setZoom}
          onRotationChange={setRotation}
          onCropComplete={(_croppedArea: Area, pixels: Area) => setArea(pixels)}
        />
      </div>

      <div className="flex items-center gap-3">
        <ZoomOut className="text-muted-foreground size-4 shrink-0" />
        <Slider
          min={1}
          max={3}
          step={0.01}
          value={[zoom]}
          onValueChange={([z]) => setZoom(z ?? 1)}
          aria-label="Zoom"
          className="flex-1"
        />
        <ZoomIn className="text-muted-foreground size-4 shrink-0" />
        <Button
          type="button"
          variant="outline"
          size="icon"
          className="size-9 shrink-0"
          onClick={() => setRotation((r) => (r - 90 + 360) % 360)}
          aria-label="Nach links drehen"
        >
          <RotateCcw className="size-4" />
        </Button>
        <Button
          type="button"
          variant="outline"
          size="icon"
          className="size-9 shrink-0"
          onClick={() => setRotation((r) => (r + 90) % 360)}
          aria-label="Nach rechts drehen"
        >
          <RotateCw className="size-4" />
        </Button>
      </div>

      <div className="flex items-center gap-3 rounded-[11px] border p-2">
        <div
          className="bg-muted size-14 shrink-0 rounded-lg bg-cover bg-center"
          style={preview ? { backgroundImage: `url(${preview})` } : undefined}
          aria-hidden
        />
        <p className="text-muted-foreground text-xs">
          So erscheint das Bild im Shop. Für kleine Vorschauen wird die Bildmitte quadratisch zugeschnitten.
        </p>
        {!immediate && (
          <Button type="button" variant="ghost" size="sm" className="shrink-0" onClick={cancel}>
            Anderes Foto
          </Button>
        )}
      </div>

      {immediate && (
        <div className="flex gap-2">
          <Button type="button" variant="outline" size="sm" className="flex-1" onClick={cancel} disabled={busy}>
            Abbrechen
          </Button>
          <Button type="button" size="sm" className="flex-1" onClick={confirmUpload} disabled={busy || !preview}>
            {saving && <Loader2 className="mr-1 h-3 w-3 animate-spin" />}
            Speichern
          </Button>
        </div>
      )}

      {error && <p className="text-destructive text-xs">{error}</p>}
    </div>
  )
}
