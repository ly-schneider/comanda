import dynamic from "next/dynamic"
import type { PluginManifest } from "@/plugins/registry"

const manifest: PluginManifest = {
  key: "stationqr",
  name: "Station QR",
  slots: {
    "admin.stations.actions": dynamic(() => import("./admin-stations-action")),
    "device.pairing.actions": dynamic(() => import("./pairing-scan-action")),
  },
}

export default manifest
