import { notFound } from "next/navigation"
import { getPlugins } from "@/lib/server/plugins"
import ArtistVoucherAdmin from "@/plugins/artist-voucher/admin"

export default async function AdminArtistVoucherPage() {
  const plugins = await getPlugins()
  if (!plugins.some((p) => p.key === "artist-voucher")) notFound()
  return <ArtistVoucherAdmin />
}
