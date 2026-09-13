import { notFound } from "next/navigation"
import { getPlugins } from "@/lib/server/plugins"
import ArtistVoucherAdmin from "@/plugins/artist-voucher/admin"
import VoucherBatchAdmin from "@/plugins/artist-voucher/batch-admin"

export default async function AdminArtistVoucherPage() {
  const plugins = await getPlugins()
  if (!plugins.some((p) => p.key === "artist-voucher")) notFound()
  return (
    <div className="space-y-6">
      <VoucherBatchAdmin />
      <ArtistVoucherAdmin />
    </div>
  )
}
