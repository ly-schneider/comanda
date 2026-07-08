/** RFC 4648 §5 base64url (no padding) <-> bytes, browser-native atob/btoa. */

export function bytesToBase64Url(bytes: Uint8Array): string {
  let bin = ""
  for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]!)
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "")
}

export function base64UrlToBytes(s: string): Uint8Array | null {
  let std = s.replace(/-/g, "+").replace(/_/g, "/")
  const pad = std.length % 4
  if (pad === 1) return null // never a valid base64 length
  if (pad) std += "=".repeat(4 - pad)
  try {
    const binary = atob(std)
    const bytes = new Uint8Array(binary.length)
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
    return bytes
  } catch {
    return null
  }
}
