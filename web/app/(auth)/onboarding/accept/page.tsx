import { Suspense } from "react"
import OnboardingAcceptClient from "./onboarding-accept-client"

export default function OnboardingAcceptPage() {
  return (
    <Suspense fallback={<div className="container mx-auto px-4 pt-24 pb-10">Prüfe Einladung…</div>}>
      <OnboardingAcceptClient />
    </Suspense>
  )
}
