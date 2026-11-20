// The tenant names the benefit ("100er Club", "Gönner", "Stammkarte"); until
// then it is simply what it is. Mirrors the backend's default.
export const DEFAULT_LABEL = "Mitgliederkarte"

export function pluginLabel(config: Record<string, unknown>): string {
  const label = config.label
  return typeof label === "string" && label.trim() !== "" ? label : DEFAULT_LABEL
}
