import dynamic from "next/dynamic"
import type { PluginManifest } from "@/plugins/registry"

const manifest: PluginManifest = {
  key: "club100",
  name: "100 Club",
  slots: {
    "pos.gratis.actions": dynamic(() => import("./gratis-action")),
    "admin.settings.section": dynamic(() => import("./admin-settings")),
    "platform.plugin.config": dynamic(() => import("./platform-config")),
  },
}

export default manifest
