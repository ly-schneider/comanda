import { Suspense } from "react"
import AccountOverviewClient from "./account-overview-client"

export default function AccountOverviewPage() {
  return (
    <Suspense
      fallback={
        <div className="space-y-6">
          <p className="text-muted-foreground text-sm">Laden…</p>
        </div>
      }
    >
      <AccountOverviewClient />
    </Suspense>
  )
}
