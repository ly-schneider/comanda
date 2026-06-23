import type { Metadata } from "next"
import { AccountShell } from "@/components/account/account-shell"

export const metadata: Metadata = {
  title: "Konto",
  description: "Verwalte Marke, Domains und Konto.",
}

export default function AccountLayout({ children }: { children: React.ReactNode }) {
  return <AccountShell>{children}</AccountShell>
}
