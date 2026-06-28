import { TenantAccountShell } from "@/components/account/tenant-account-shell"

export default async function TenantLayout({
  children,
  params,
}: {
  children: React.ReactNode
  params: Promise<{ tenant: string }>
}) {
  const { tenant } = await params
  return <TenantAccountShell tenant={tenant}>{children}</TenantAccountShell>
}
