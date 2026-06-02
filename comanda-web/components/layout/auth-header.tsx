import Image from "next/image"
import Link from "next/link"

export default function AuthHeader() {
  return (
    <div className="mx-auto flex items-center justify-center">
      <Link href="/food" className="inline-flex items-center gap-3">
        <Image src="/assets/images/comanda-logo.png" alt="Comanda Logo" width={40} height={40} className="h-10 w-10" />
        <span className="text-lg font-semibold tracking-tight">Comanda</span>
      </Link>
    </div>
  )
}
