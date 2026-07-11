import dynamic from "next/dynamic"
import type { PluginManifest } from "@/plugins/registry"

const manifest: PluginManifest = {
  key: "artist-voucher",
  name: "Artist Voucher",
  slots: {
    "admin.settings.section": dynamic(() => import("./admin-settings")),
  },
}

export default manifest
