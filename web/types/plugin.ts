export interface PublicPlugin {
  key: string
  name: string
  config: Record<string, unknown>
}

export interface PublicPluginList {
  items: PublicPlugin[]
}

export interface TenantPluginState {
  key: string
  name: string
  enabled: boolean
  config: Record<string, unknown>
  secretsConfiguredAt?: string | null
}

export interface TenantPluginList {
  items: TenantPluginState[]
}

export interface TenantPluginUpdate {
  enabled?: boolean
  config?: Record<string, unknown>
  secrets?: Record<string, string>
}
