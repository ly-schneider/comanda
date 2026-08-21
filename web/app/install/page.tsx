import { AlertTriangle, Download, KeyRound, ShieldCheck } from "lucide-react"
import type { Metadata } from "next"
import Image from "next/image"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { getBranding } from "@/lib/server/branding"

export const metadata: Metadata = {
  title: "POS-App installieren",
  // The link carries a download credential.
  robots: { index: false, follow: false },
}

export const dynamic = "force-dynamic"

type SearchParams = Promise<{ pkg?: string; channel?: string; t?: string }>

export default async function InstallPage({ searchParams }: { searchParams: SearchParams }) {
  const { pkg, channel, t } = await searchParams
  const branding = await getBranding()

  // Same-origin proxy rather than the API host, so BYO-domain tablets need no
  // extra CORS or DNS setup.
  const download = new URLSearchParams()
  if (pkg) download.set("pkg", pkg)
  download.set("channel", channel || "production")
  if (t) download.set("t", t)
  const downloadUrl = `/api/v1/android/download?${download.toString()}`

  return (
    <main className="mx-auto flex min-h-dvh w-full max-w-xl flex-col justify-center gap-6 p-6">
      <div className="flex flex-col items-center gap-3 text-center">
        {branding.logoUrl ? (
          <Image
            src={branding.logoUrl}
            alt={branding.displayName}
            width={160}
            height={64}
            className="h-16 w-auto object-contain"
            unoptimized
          />
        ) : null}
        <div>
          <h1 className="text-2xl font-semibold">POS-App installieren</h1>
          <p className="text-muted-foreground text-sm">
            Für {branding.displayName}
            {channel === "staging" ? " (Staging)" : ""}
          </p>
        </div>
      </div>

      {pkg ? (
        <Button asChild size="lg" className="w-full">
          <a href={downloadUrl} download>
            <Download className="mr-2 size-4" />
            App herunterladen
          </a>
        </Button>
      ) : (
        <Card className="border-destructive/40">
          <CardContent className="text-muted-foreground flex gap-3 pt-6 text-sm">
            <AlertTriangle className="text-destructive mt-0.5 size-4 shrink-0" />
            <p>
              Dieser Link ist unvollständig. Öffne den Installations-QR-Code nochmals in der Plattform-Verwaltung und
              scanne ihn neu.
            </p>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">So gehts</CardTitle>
        </CardHeader>
        <CardContent className="text-muted-foreground space-y-4 text-sm">
          <div className="flex gap-3">
            <Download className="mt-0.5 size-4 shrink-0" />
            <p>
              Auf «App herunterladen» tippen. Android fragt einmalig, ob dieser Browser Apps installieren darf —
              erlauben und danach auf die heruntergeladene Datei tippen.
            </p>
          </div>
          <div className="flex gap-3">
            <KeyRound className="mt-0.5 size-4 shrink-0" />
            <p>
              Beim ersten Start die Geräte-Einrichtung öffnen (lange oben links auf den Bildschirm drücken) und das
              SumUp-Konto anmelden.
            </p>
          </div>
          <div className="flex gap-3">
            <ShieldCheck className="mt-0.5 size-4 shrink-0" />
            <p>
              Mehrere Betriebe am selben Ort: jeden über seinen eigenen Link installieren. Die Apps laufen unabhängig
              nebeneinander, mit getrenntem SumUp-Konto und eigener Kasse.
            </p>
          </div>
        </CardContent>
      </Card>
    </main>
  )
}
