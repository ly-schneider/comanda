# Web plugins

Tenant-optional features are in-tree plugins with per-tenant runtime enablement. The backend is
authoritative: `GET /v1/plugins` returns the plugins enabled for the host-resolved tenant, and the
platform-admin API (`/v1/platform/tenants/{id}/plugins`) toggles and configures them. A disabled
plugin is invisible — its slot content never renders and its JS chunk is never loaded.

## Manifest

Each plugin exports a manifest into `registry.ts`:

```ts
import dynamic from "next/dynamic"

const manifest: PluginManifest = {
  key: "membership",
  name: "Mitgliederkarten",
  slots: {
    "pos.gratis.actions": dynamic(() => import("./membership/gratis-action")),
    "shop.home.actions": dynamic(() => import("./membership/home-action")),
    "shop.claim.page": dynamic(() => import("./membership/claim-page")),
    "admin.settings.section": dynamic(() => import("./membership/admin-settings")),
    "platform.plugin.config": dynamic(() => import("./membership/platform-config")),
  },
}
```

Slot components are loaded with `next/dynamic`, so a tenant without the plugin enabled never ships
the plugin's code. `registry` is the single source of truth; keep it empty when no plugins exist.

## Enablement gating

`PluginProvider` (mounted in `app/layout.tsx`) is fed the server-fetched enabled-plugin list from
`getPlugins()`. `usePlugins()` exposes `enabledPlugins` and `getPluginConfig(key)`.

`<PluginSlot name="..." ctx={...} />` renders, for every enabled plugin whose manifest defines that
slot, the slot component with the typed `ctx` plus the plugin's public `config`. It renders nothing
when no enabled plugin provides the slot, unless it is given a `fallback` — which a slot that owns a
whole page needs, so the page is not blank.

## Slots (v1)

| Slot | Where | `ctx` |
| --- | --- | --- |
| `pos.gratis.actions` | POS Gratis dialog (behind "Jetzt bezahlen") | `{ cartItems: CartItem[]; applyDiscount: (d: PluginDiscount \| null) => void }` |
| `shop.home.actions` | Storefront, above the catalog | `{}` |
| `shop.claim.page` | The whole `/mitglied` page in the shop | `{}` |
| `admin.settings.section` | Tenant admin settings page | `{ token: string }` |
| `device.pairing.actions` | `/station` + `/pos` pairing screens | `{}` |
| `platform.plugin.config` | Platform-admin plugin manager | see below (props, not a `PluginSlot`) |

`PluginDiscount = { pluginKey: string; label: string; amountCents: number; paymentData: unknown }`.

`platform.plugin.config` is not gated by the current host's enabled plugins — the platform admin
manages arbitrary tenants. Its component is looked up directly from the registry with
`getPlatformConfigComponent(key)` and rendered with these props:

```ts
{
  tenantId: string
  pluginKey: string
  config: Record<string, unknown>
  secretsConfiguredAt: string | null
  onSave: (patch: { enabled?: boolean; config?: Record<string, unknown>; secrets?: Record<string, string> }) => Promise<void>
  saving: boolean
}
```

## Payment mechanism

A `pos.gratis.actions` component renders a tile next to Gäste / VIP / Mitarbeiter and, on selection,
calls `applyDiscount({ pluginKey, label, amountCents, paymentData })`. The basket renders a generic
discount line from `label`/`amountCents`. On submit it attaches
`PaymentCreate.plugin = { key: pluginKey, data: paymentData }`:

- a discount covering the basket pays with method `gratis_plugin` and the plugin payload right away,
- a partial discount drops back to the tender grid and attaches the same plugin payload to the
  cash/card/twint payment.

The backend's `PaymentHook` validates and records the plugin payload and persists `plugin_key`
alongside the payment method for analytics.
