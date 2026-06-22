import type { Metadata } from "next"
import AuthFooter from "@/components/layout/auth-footer"

export const metadata: Metadata = {
  other: {
    "theme-color": "#FFFFFF",
    "msapplication-TileColor": "#FFFFFF",
    "apple-mobile-web-app-status-bar-style": "default",
  },
}

export default function AuthLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-screen flex-col overflow-x-hidden">
      <main className="flex-1">{children}</main>
      <AuthFooter />
    </div>
  )
}
