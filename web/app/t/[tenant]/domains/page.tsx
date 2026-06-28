import { DomainsManager } from "@/components/domains/domains-manager"

export default async function Page({ params }: { params: Promise<{ tenant: string }> }) {
  const { tenant } = await params
  return <DomainsManager basePath={`/api/v1/account/${tenant}/domains`} />
}
