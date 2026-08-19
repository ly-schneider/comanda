// Command devsalescheck builds the sales report for a tenant through the real
// service — RLS-bound, off the request path — and writes the workbook to disk.
// Local dev tool for eyeballing the db/seed/dev fixtures; not wired into the app.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/internal/xlsx"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
)

func main() {
	tenant := flag.String("tenant", "", "tenant id")
	envFile := flag.String("env", ".env.local", "env file holding DATABASE_URL_APP")
	fromStr := flag.String("from", "2026-08-14", "inclusive start (local date)")
	toStr := flag.String("to", "2026-08-17", "exclusive end (local date)")
	out := flag.String("out", "", "optional path to write the .xlsx to")
	flag.Parse()

	if *tenant == "" {
		log.Fatal("-tenant is required")
	}
	if err := godotenv.Overload(*envFile); err != nil {
		log.Printf("warning: could not load %s: %v", *envFile, err)
	}
	dsn := os.Getenv("DATABASE_URL_APP")
	if dsn == "" {
		log.Fatal("DATABASE_URL_APP is not set")
	}

	loc, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		log.Fatal(err)
	}
	from, err := time.ParseInLocation("2006-01-02", *fromStr, loc)
	if err != nil {
		log.Fatal(err)
	}
	to, err := time.ParseInLocation("2006-01-02", *toStr, loc)
	if err != nil {
		log.Fatal(err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	fallback := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, db)))

	var report xlsx.SalesReport
	err = repository.RunInTenantTx(context.Background(), db, *tenant, func(ctx context.Context) error {
		svc := service.NewSalesExportService(
			repository.NewOrderRepository(fallback),
			repository.NewRefundRepository(fallback),
			repository.NewProductRepository(fallback),
			repository.NewCategoryRepository(fallback),
		)
		var err error
		report, err = svc.Build(ctx, from, to)
		return err
	})
	if err != nil {
		log.Fatalf("build: %v", err)
	}

	t := report.Totals
	fmt.Printf("Zeitraum        %s bis %s\n", from.Format("02.01.2006"), to.Add(-time.Nanosecond).Format("02.01.2006"))
	fmt.Printf("Bestellungen    %d\n", t.OrderCount)
	fmt.Printf("Artikel         %d\n", t.ItemCount)
	fmt.Printf("Warenumsatz     %10.2f\n", chf(t.RevenueCents))
	fmt.Printf("Depot           %10.2f\n", chf(t.DepositCents))
	fmt.Printf("Erstattungen    %10.2f  (davon ohne Artikelbezug %.2f)\n", -chf(t.RefundedCents), -chf(t.UnlinkedRefunds))
	fmt.Printf("Netto           %10.2f\n", chf(report.NetCents()))
	fmt.Printf("Gratis          %10.2f  (%d Bestellungen, %d Artikel)\n\n", chf(t.GratisCents), t.GratisOrderCount, t.GratisItemCount)

	fmt.Printf("%-22s %6s %12s %10s %12s\n", "Zahlungsart", "Best.", "Umsatz", "Depot", "Netto")
	for _, m := range report.Methods {
		fmt.Printf("%-22s %6d %12.2f %10.2f %12.2f\n", m.Method, m.OrderCount, chf(m.RevenueCents), chf(m.DepositCents), chf(m.NetCents()))
	}

	fmt.Printf("\n%-26s %-6s %6s %7s %7s %12s\n", "Artikel", "Var.", "Menge", "Bundle", "Gratis", "Umsatz")
	for _, a := range report.Articles {
		fmt.Printf("%-26s %-6s %6d %7d %7d %12.2f\n", a.Product, a.Variant, a.Quantity, a.BundleQty, a.GratisQty, chf(a.RevenueCents))
	}

	if *out != "" {
		body, err := xlsx.RenderSalesReport(report)
		if err != nil {
			log.Fatalf("render: %v", err)
		}
		if err := os.WriteFile(*out, body, 0o644); err != nil {
			log.Fatalf("write: %v", err)
		}
		fmt.Printf("\nworkbook: %s (%d KB, %d Verkaufszeilen)\n", *out, len(body)/1024, len(report.Sales))
	}
}

func chf(cents int64) float64 { return float64(cents) / 100 }
