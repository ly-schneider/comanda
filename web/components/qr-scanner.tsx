"use client"

import { useEffect, useRef } from "react"
import { prepareZXingModule, readBarcodes } from "zxing-wasm/reader"

// Serve the reader wasm same-origin (vendored to /assets at build, SW-precached)
// so the offline station never hits a CDN. `window` guard keeps it out of SSR.
if (typeof window !== "undefined") {
  prepareZXingModule({
    overrides: {
      locateFile: (path: string, prefix: string) =>
        path.endsWith(".wasm") ? "/assets/zxing_reader.wasm" : `${prefix}${path}`,
    },
  })
}

export type QrScanResult = { text: string; bytes: Uint8Array }
export type QrScannerControls = { pause: () => void; resume: () => void }

type Props = {
  cameraId?: string
  fps?: number
  maxWidthRem?: number
  onResult: (result: QrScanResult) => void
  onReady?: (controls: QrScannerControls) => void
  onStartError?: (error: unknown) => void
  onStop?: () => void
}

export default function QrScanner({
  cameraId,
  fps = 10,
  maxWidthRem = 28,
  onResult,
  onReady,
  onStartError,
  onStop,
}: Props) {
  const videoRef = useRef<HTMLVideoElement | null>(null)
  const pausedRef = useRef(false)

  // Read callbacks through refs so a parent re-render never restarts the camera.
  const onResultRef = useRef(onResult)
  const onReadyRef = useRef(onReady)
  const onStartErrorRef = useRef(onStartError)
  const onStopRef = useRef(onStop)
  onResultRef.current = onResult
  onReadyRef.current = onReady
  onStartErrorRef.current = onStartError
  onStopRef.current = onStop

  useEffect(() => {
    let cancelled = false
    let stream: MediaStream | null = null
    let interval: ReturnType<typeof setInterval> | null = null
    let decoding = false
    const video = videoRef.current
    const canvas = document.createElement("canvas")
    const ctx = canvas.getContext("2d", { willReadFrequently: true })

    const stop = () => {
      if (interval) clearInterval(interval)
      interval = null
      stream?.getTracks().forEach((t) => t.stop())
      stream = null
    }

    const tick = async () => {
      if (cancelled || pausedRef.current || decoding || !video || !ctx) return
      if (video.readyState < 2 || video.videoWidth === 0) return
      decoding = true
      try {
        // Downscale the long edge; QR detection stays reliable and cheaper.
        const scale = Math.min(1, 640 / Math.max(video.videoWidth, video.videoHeight))
        const w = Math.max(1, Math.round(video.videoWidth * scale))
        const h = Math.max(1, Math.round(video.videoHeight * scale))
        if (canvas.width !== w) canvas.width = w
        if (canvas.height !== h) canvas.height = h
        ctx.drawImage(video, 0, 0, w, h)
        const image = ctx.getImageData(0, 0, w, h)
        const results = await readBarcodes(image, { formats: ["QRCode"], maxNumberOfSymbols: 1, tryHarder: true })
        if (cancelled || pausedRef.current) return
        const hit = results.find((r) => r.isValid && r.bytes.length > 0)
        if (hit) onResultRef.current({ text: hit.text, bytes: hit.bytes })
      } catch {
        // Transient frame/decoder error (incl. first-call wasm warm-up) — keep scanning.
      } finally {
        decoding = false
      }
    }

    void (async () => {
      try {
        stream = await navigator.mediaDevices.getUserMedia({
          video: cameraId ? { deviceId: { exact: cameraId } } : { facingMode: "environment" },
        })
        if (cancelled || !video) {
          stream.getTracks().forEach((t) => t.stop())
          return
        }
        video.srcObject = stream
        video.setAttribute("playsinline", "true")
        await video.play().catch(() => {})
        if (cancelled) {
          stop()
          return
        }
        pausedRef.current = false
        interval = setInterval(tick, Math.max(1, Math.round(1000 / fps)))
        onReadyRef.current?.({
          pause: () => {
            pausedRef.current = true
          },
          resume: () => {
            pausedRef.current = false
          },
        })
      } catch (error) {
        if (!cancelled) onStartErrorRef.current?.(error)
      }
    })()

    return () => {
      cancelled = true
      onStopRef.current?.()
      stop()
      if (video) video.srcObject = null
    }
  }, [cameraId, fps])

  return (
    <div className="mx-auto w-full overflow-hidden rounded-2xl bg-black" style={{ maxWidth: `${maxWidthRem}rem` }}>
      <video ref={videoRef} className="aspect-square w-full object-cover" muted playsInline />
    </div>
  )
}
