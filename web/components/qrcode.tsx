"use client"

import Image from "next/image"
import { type QRCodeSegment, toDataURL } from "qrcode"
import { useEffect, useState } from "react"
import { base64UrlToBytes } from "@/lib/base64url"

type Props = {
  value: string
  size?: number
  className?: string
  // `value` is a base64url token rendered byte-mode: the pickup QR is a binary
  // v2 payload, so the QR must carry the raw bytes the station scans, not text.
  binary?: boolean
}

export default function QRCode({ value, size = 220, className, binary = false }: Props) {
  const [src, setSrc] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        let input: string | QRCodeSegment[] = value
        if (binary) {
          const bytes = base64UrlToBytes(value)
          if (!bytes) throw new Error("QR token is not valid base64url")
          input = [{ data: bytes, mode: "byte" }]
        }
        const url = await toDataURL(input, {
          width: size,
          margin: 1,
          color: {
            dark: "#000000",
            light: "#E9E7E6",
          },
        })
        if (!cancelled) setSrc(url)
      } catch (err) {
        console.error("Failed generating QR code", err)
        if (!cancelled) setSrc(null)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [value, size, binary])

  if (!src) return null
  return <Image src={src} alt="QR Code" width={size} height={size} className={className} unoptimized />
}
