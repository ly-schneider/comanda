export function extractEnrollToken(text: string): string | null {
  const trimmed = text.trim()
  if (!trimmed) return null
  if (/^https?:\/\//i.test(trimmed)) {
    try {
      const code = new URL(trimmed).searchParams.get("code")
      return code?.trim() || null
    } catch {
      return null
    }
  }
  return trimmed
}
