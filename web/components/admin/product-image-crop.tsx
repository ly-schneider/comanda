"use client"

import { RotateCcw, RotateCw, Upload, ZoomIn, ZoomOut } from "lucide-react"
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

type Props = {
  // Fires with the cropped 1:1 WebP whenever the crop settles, or null when cleared.
  onChange: (file: File | null) => void
}

export function ProductImageCrop({ onChange }: Props) {
  const [src, setSrc] = useState<string | null>(null)
  const [crop, setCrop] = useState({ x: 0, y: 0 })
  const [zoom, setZoom] = useState(1)
  const [rotation, setRotation] = useState(0)
  const [area, setArea] = useState<CropArea | null>(null)
  const [preview, setPreview] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  // Keep callback and revocable URLs in refs so the crop effect stays stable.
  const onChangeRef = useRef(onChange)
  onChangeRef.current = onChange
  const srcRef = useRef<string | null>(null)
  const previewRef = useRef<string | null>(null)

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

  const clear = () => {
    if (srcRef.current) URL.revokeObjectURL(srcRef.current)
    if (previewRef.current) URL.revokeObjectURL(previewRef.current)
    srcRef.current = null
    previewRef.current = null
    setSrc(null)
    setArea(null)
    setPreview(null)
    setError(null)
    onChangeRef.current(null)
    if (inputRef.current) inputRef.current.value = ""
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
        onChangeRef.current(file)
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

  if (!src) {
    return (
      <div className="space-y-1">
        <div
          role="button"
          tabIndex={0}
          onClick={() => inputRef.current?.click()}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === " ") {
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
          className="border-border text-muted-foreground hover:bg-muted/50 flex aspect-square w-full cursor-pointer flex-col items-center justify-center gap-2 rounded-[11px] border border-dashed"
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
      <div className="bg-muted relative aspect-square w-full overflow-hidden rounded-[11px]">
        <Cropper
          image={src}
          crop={crop}
          zoom={zoom}
          rotation={rotation}
          aspect={1}
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
          So erscheint das Bild im Shop und Checkout (1:1). Achte darauf, dass alles Wichtige im Quadrat liegt.
        </p>
        <Button type="button" variant="ghost" size="sm" className="shrink-0" onClick={clear}>
          Anderes Foto
        </Button>
      </div>

      {error && <p className="text-destructive text-xs">{error}</p>}
    </div>
  )
}
