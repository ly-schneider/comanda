type PostAuthInput = {
  isPlatformAdmin: boolean
  activeOrganizationId?: string | null
}

/**
 * Determines where to send a user after a successful sign-in.
 * An explicit safe same-origin `next` wins over the role defaults — e.g. an
 * invited admin logging in on the tenant host with next=/admin must not be
 * bounced to /account.
 */
export function postAuthPath(user: PostAuthInput, next?: string | null): string {
  if (next && next.startsWith("/") && !next.startsWith("//")) return next

  if (user.isPlatformAdmin) return "/platform-admin"
  if (user.activeOrganizationId) return "/account"
  return "/"
}
