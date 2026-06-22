import type { Metadata } from "next"
import { PlatformAdminGuard } from "@/components/platform-admin/guard"
import { PlatformAdminHeader } from "@/components/platform-admin/header"

export const metadata: Metadata = {
  title: "Platform Admin",
  description: "Comanda platform administration.",
}

export default function PlatformAdminLayout({ children }: { children: React.ReactNode }) {
  return (
    <PlatformAdminGuard>
      <div className="flex min-h-screen flex-col">
        <PlatformAdminHeader />
        <main className="mx-auto w-full max-w-5xl flex-1 px-4 py-8">{children}</main>
      </div>
    </PlatformAdminGuard>
  )
}
