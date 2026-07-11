import type { PluginManifest } from "@/plugins/registry"

// No slots: artist-voucher renders as a full admin route (app/admin/artist-voucher).
const manifest: PluginManifest = {
  key: "artist-voucher",
  name: "Artist Voucher",
  slots: {},
}

export default manifest
