type PostAuthInput = {
  isPlatformAdmin: boolean
  activeOrganizationId?: string | null
}

/**
 * Determines where to send a user after a successful sign-in.
 * Validates `next` so only safe same-origin paths are accepted.
 */
export function postAuthPath(user: PostAuthInput, next?: string | null): string {
  if (user.isPlatformAdmin) return "/platform-admin"
  if (user.activeOrganizationId) return "/account"

  if (next && next.startsWith("/") && !next.startsWith("//")) return next
  return "/"
}
