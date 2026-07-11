import { notFound } from "next/navigation"
import { getPlugins } from "@/lib/server/plugins"

export default async function StaffPerksLayout({ children }: { children: React.ReactNode }) {
  const plugins = await getPlugins()
  if (!plugins.some((p) => p.key === "staff-perks")) notFound()
  return children
}
