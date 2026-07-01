import { SumUpSettingsForm } from "@/components/sumup/sumup-settings-form"

export default async function Page({ params }: { params: Promise<{ tenant: string }> }) {
  const { tenant } = await params
  return <SumUpSettingsForm basePath={`/api/v1/account/${tenant}/settings`} />
}
