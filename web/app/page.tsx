import { redirect } from "next/navigation"
import { PlatformLanding } from "@/components/landing/platform-landing"
import { getRequestHost } from "@/lib/server/host"
import { parseTenantHost } from "@/lib/tenant"

export const dynamic = "force-dynamic"

// Root path: on the platform host show the landing page; on any tenant or
// custom-domain host send visitors straight to the storefront.
export default async function Page() {
  const host = await getRequestHost()
  if (parseTenantHost(host).type !== "platform") {
    redirect("/food")
  }
  return <PlatformLanding />
}
