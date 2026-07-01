import { PayrexxBindingForm } from "@/components/payrexx/payrexx-binding-form"

export default async function Page({ params }: { params: Promise<{ tenant: string }> }) {
  const { tenant } = await params
  return <PayrexxBindingForm basePath={`/api/v1/account/${tenant}/settings/payrexx`} heading="Payrexx" />
}
