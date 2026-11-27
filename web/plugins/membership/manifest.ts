import dynamic from "next/dynamic"
import type { PluginManifest } from "@/plugins/registry"

const manifest: PluginManifest = {
  key: "membership",
  name: "Mitgliederkarten",
  slots: {
    "pos.gratis.actions": dynamic(() => import("./gratis-action")),
    "shop.home.actions": dynamic(() => import("./home-action")),
    "shop.claim.page": dynamic(() => import("./claim-page")),
    "admin.settings.section": dynamic(() => import("./admin-settings")),
    "platform.plugin.config": dynamic(() => import("./platform-config")),
  },
}

export default manifest
