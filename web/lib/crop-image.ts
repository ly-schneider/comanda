export interface CropArea {
  x: number
  y: number
  width: number
  height: number
}

function createImage(url: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.addEventListener("load", () => resolve(image))
    image.addEventListener("error", () => reject(new Error("Bild konnte nicht geladen werden")))
    image.src = url
  })
}

function radians(degrees: number): number {
  return (degrees * Math.PI) / 180
}

function rotatedBBox(width: number, height: number, rotation: number): { width: number; height: number } {
  return {
    width: Math.abs(Math.cos(rotation) * width) + Math.abs(Math.sin(rotation) * height),
    height: Math.abs(Math.sin(rotation) * width) + Math.abs(Math.cos(rotation) * height),
  }
}

// Renders the (optionally rotated) source, extracts the crop area and normalizes
// it to a square WebP — the 1:1 format used across the storefront and checkout.
export async function getCroppedBlob(imageSrc: string, crop: CropArea, rotation = 0, outputSize = 800): Promise<Blob> {
  const image = await createImage(imageSrc)
  const canvas = document.createElement("canvas")
  const ctx = canvas.getContext("2d")
  if (!ctx) throw new Error("Canvas nicht verfügbar")

  const rot = radians(rotation)
  const box = rotatedBBox(image.width, image.height, rot)
  canvas.width = box.width
  canvas.height = box.height
  ctx.translate(box.width / 2, box.height / 2)
  ctx.rotate(rot)
  ctx.drawImage(image, -image.width / 2, -image.height / 2)

  const out = document.createElement("canvas")
  const outCtx = out.getContext("2d")
  if (!outCtx) throw new Error("Canvas nicht verfügbar")
  out.width = outputSize
  out.height = outputSize
  outCtx.drawImage(canvas, crop.x, crop.y, crop.width, crop.height, 0, 0, outputSize, outputSize)

  return new Promise<Blob>((resolve, reject) => {
    out.toBlob(
      (blob) => (blob ? resolve(blob) : reject(new Error("Bild konnte nicht erstellt werden"))),
      "image/webp",
      0.9
    )
  })
}
