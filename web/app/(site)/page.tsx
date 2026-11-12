import { Metadata } from "next"
import { FloatingBottomNav } from "@/components/cart/floating-bottom-nav"
import { CatalogGridLive } from "@/components/catalog/catalog-grid-live"
import { MarketingLanding } from "@/components/marketing/marketing-landing"
import { listCategories } from "@/lib/api/categories"
import { listProducts } from "@/lib/api/products"
import { getSystemStatus } from "@/lib/api/system"
import { effectivelyActiveCategories } from "@/lib/categories"
import { getRequestHost } from "@/lib/server/host"
import { parseTenantHost } from "@/lib/tenant"
import { CategoryDTO, ListResponse, ProductDTO } from "@/types"

export async function generateMetadata(): Promise<Metadata> {
  const host = await getRequestHost()
  if (parseTenantHost(host).type === "platform") {
    return {
      title: { absolute: "Comanda: Kasse und Handy-Bestellung für Stände" },
      description:
        "Comanda ist Kasse und Handy-Bestellung für Stände an Anlässen und Festivals. Gäste bestellen am Handy und zahlen mit TWINT, oder sie gehen an die Kasse.",
      openGraph: {
        title: "Comanda: Kasse und Handy-Bestellung für Stände",
        description: "Gäste bestellen am Handy oder an der Kasse. Gebaut und eingerichtet von Levyn Schneider.",
        url: "/",
        images: [{ url: "/assets/images/marketing/kasse-festival.jpg", width: 2000, height: 1333 }],
      },
    }
  }
  return {
    title: "Sortiment",
    description: "Entdecke unser Angebot und bestelle direkt online.",
    openGraph: {
      title: "Sortiment",
      description: "Unser Angebot entdecken und online bestellen bei Comanda.",
      url: "/",
    },
  }
}

export const dynamic = "force-dynamic"

export default async function HomePage() {
  const host = await getRequestHost()
  if (parseTenantHost(host).type === "platform") {
    return <MarketingLanding />
  }

  const hostHeaders: Record<string, string> = host ? { "X-Forwarded-Host": host } : {}
  const { enabled } = await getSystemStatus({ headers: hostHeaders })

  if (!enabled) {
    return (
      <div className="bg-background flex min-h-screen items-center justify-center">
        <div className="text-center">
          <h2 className="text-2xl font-semibold">Aktuell geschlossen</h2>
          <p className="text-muted-foreground mt-2">Das Bestellsystem ist momentan nicht verfügbar.</p>
        </div>
      </div>
    )
  }

  let products: ListResponse<ProductDTO>
  try {
    products = await listProducts({ visible: true }, { headers: hostHeaders })
  } catch (error) {
    console.error("Failed to fetch products:", error)
    products = { items: [], count: 0 }
  }

  let categories: CategoryDTO[] = []
  try {
    categories = effectivelyActiveCategories(await listCategories({ headers: hostHeaders }))
  } catch (error) {
    console.error("Failed to fetch categories:", error)
  }

  return (
    <div className="bg-background min-h-screen">
      <main className="container mx-auto p-4">
        <h2 className="mb-2 text-2xl font-semibold">Alle Produkte</h2>
        <CatalogGridLive initialProducts={products} categories={categories} />
      </main>

      <FloatingBottomNav />
    </div>
  )
}
