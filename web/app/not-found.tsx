import { StorefrontNotFound } from "@/components/layout/storefront-not-found"
import { MarketingNotFound } from "@/components/marketing/marketing-not-found"
import { isPlatformRequest } from "@/lib/server/host"

export default async function NotFound() {
  return (await isPlatformRequest()) ? <MarketingNotFound /> : <StorefrontNotFound />
}
