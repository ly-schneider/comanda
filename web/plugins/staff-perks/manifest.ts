import type { PluginManifest } from "@/plugins/registry"

// No slots: staff-perks renders as full admin routes, not slot injections.
const manifest: PluginManifest = {
  key: "staff-perks",
  name: "Staff Perks",
  slots: {},
}

export default manifest
