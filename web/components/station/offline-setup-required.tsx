import { WifiOff } from "lucide-react"

// Pairing/setup need the network, so offline we show this instead of a code.
export function OfflineSetupRequired() {
  return (
    <div role="status" className="bg-card w-full rounded-2xl p-6">
      <div className="bg-destructive/10 text-destructive mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-full">
        <WifiOff className="h-5 w-5" aria-hidden />
      </div>
      <h2 className="font-primary text-center text-xl tracking-wide uppercase">Offline – Einrichtung nötig</h2>
      <p className="text-muted-foreground mt-1 text-center text-sm">
        Dieses Gerät ist noch nicht für den Offline-Betrieb eingerichtet. Verbinde dich mit dem Internet, um die
        Einrichtung abzuschliessen.
      </p>
    </div>
  )
}
