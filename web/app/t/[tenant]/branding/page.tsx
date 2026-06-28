import { BrandingSettingsForm } from "@/components/branding/branding-settings-form"

export default async function Page({ params }: { params: Promise<{ tenant: string }> }) {
  const { tenant } = await params
  return <BrandingSettingsForm basePath={`/api/v1/account/${tenant}/settings`} heading="Marke & Rechtliches" />
}
