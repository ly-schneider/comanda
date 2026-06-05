"use client"

import Image from "next/image"
import Link from "next/link"
import { useBranding } from "@/contexts/branding-context"

export default function AuthHeader() {
  const { displayName, logoUrl } = useBranding()
  return (
    <div className="mx-auto flex items-center justify-center">
      <Link href="/food" className="inline-flex items-center gap-3">
        <Image
          src={logoUrl ?? "/assets/images/comanda-logo.png"}
          alt={`${displayName} Logo`}
          width={40}
          height={40}
          className="h-10 w-10"
        />
        <span className="text-lg font-semibold tracking-tight">{displayName}</span>
      </Link>
    </div>
  )
}
