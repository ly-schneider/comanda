import Link from "next/link"
import { Button } from "@/components/ui/button"

// Minimal, informational landing for the platform host (comanda.leys.ch).
export function PlatformLanding() {
  return (
    <main className="mx-auto flex min-h-screen max-w-3xl flex-col items-center justify-center px-6 py-20 text-center">
      <p className="eyebrow mb-5">Comanda</p>
      <h1 className="font-primary text-4xl leading-[1.05] tracking-tight text-balance sm:text-6xl">
        Das Bestellsystem für Foodstände und Events.
      </h1>
      <p className="text-muted-foreground mt-6 max-w-xl text-lg text-pretty">
        Eine Plattform, dein Auftritt. Bestellung, Bezahlung und Abholung. Mit eigenem Logo, eigener Domain und eigenem
        Payrexx-Konto.
      </p>
      <div className="mt-10 flex flex-wrap items-center justify-center gap-3">
        <Button asChild size="lg" variant="primary">
          <Link href="/login">Anmelden</Link>
        </Button>
        <Button asChild size="lg" variant="outline">
          <a href="mailto:hello@leys.ch?subject=Comanda%20Zugang">Zugang anfragen</a>
        </Button>
      </div>
    </main>
  )
}
