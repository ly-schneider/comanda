import { cleanup, fireEvent, render, screen, within } from "@testing-library/react"
import { afterEach, describe, expect, it } from "vitest"
import { TopProductsChart } from "./top-products-chart"

afterEach(cleanup)

function makeData(n: number) {
  return Array.from({ length: n }, (_, i) => ({ title: `Produkt ${i + 1}`, value: (n - i) * 100 }))
}

describe("TopProductsChart", () => {
  it("hides the 'Alle anzeigen' button while loading", () => {
    render(<TopProductsChart title="Top" data={makeData(3)} loading />)
    expect(screen.queryByRole("button", { name: /Alle anzeigen/ })).not.toBeInTheDocument()
    expect(screen.getByText("Lade…")).toBeInTheDocument()
  })

  it("shows an empty state and no button when there is no data", () => {
    render(<TopProductsChart title="Top" data={[]} loading={false} />)
    expect(screen.queryByRole("button", { name: /Alle anzeigen/ })).not.toBeInTheDocument()
    expect(screen.getByText("Keine Daten vorhanden.")).toBeInTheDocument()
  })

  it("labels the button with the full product count, not the top-15 cap", () => {
    render(<TopProductsChart title="Top" data={makeData(17)} loading={false} />)
    expect(screen.getByRole("button", { name: "Alle anzeigen (17)" })).toBeInTheDocument()
  })

  it("lists the complete ranking in the dialog, including products beyond the top 15", () => {
    render(<TopProductsChart title="Top Produkte nach Menge" data={makeData(17)} loading={false} />)
    fireEvent.click(screen.getByRole("button", { name: /Alle anzeigen/ }))

    const dialog = screen.getByRole("dialog")
    const items = within(dialog).getAllByRole("listitem")
    expect(items).toHaveLength(17)
    expect(within(items[0]!).getByText("Produkt 1")).toBeInTheDocument()
    expect(within(items[16]!).getByText("Produkt 17")).toBeInTheDocument()
  })

  it("renders quantities as N× in the menge variant", () => {
    render(<TopProductsChart title="Menge" data={[{ title: "Cola", value: 42 }]} loading={false} />)
    fireEvent.click(screen.getByRole("button", { name: /Alle anzeigen/ }))
    expect(within(screen.getByRole("dialog")).getByText("42×")).toBeInTheDocument()
  })

  it("renders revenue as CHF in the currency variant", () => {
    render(<TopProductsChart title="Umsatz" data={[{ title: "Cola", value: 1500 }]} loading={false} formatAsCurrency />)
    fireEvent.click(screen.getByRole("button", { name: /Alle anzeigen/ }))
    expect(within(screen.getByRole("dialog")).getByText("CHF 15.-")).toBeInTheDocument()
  })
})
