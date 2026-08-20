# Dev sales fixtures

A three-day event's worth of fake-but-shaped-right sales for one tenant, so the
sales and inventory reports have something real to show locally. Local dev only —
never point these at staging or production.

The window is **14.–16.08.2026** (Fri–Sun, Europe/Zurich). Everything the scripts
write lives inside it, which is what makes the reset exact.

```bash
cd backend
TID=$(psql "$DATABASE_URL" -tAc "SELECT id FROM tenant WHERE slug = 'blessthun'")

psql "$DATABASE_URL" -v seed_tenant="$TID" -f db/seed/dev/01_catalog_extras.sql
psql "$DATABASE_URL" -v seed_tenant="$TID" -f db/seed/dev/02_sales.sql
psql "$DATABASE_URL" -v seed_tenant="$TID" -f db/seed/dev/03_refunds.sql
psql "$DATABASE_URL" -v seed_tenant="$TID" -f db/seed/dev/04_stock.sql

# Pickup tokens can only be signed in Go (Ed25519 over the tenant key).
go run ./cmd/devseedqr -tenant "$TID"

# Optional: build the report off the request path and write the workbook out.
go run ./cmd/devsalescheck -tenant "$TID" -out /tmp/verkaeufe.xlsx

# Undo everything
psql "$DATABASE_URL" -v seed_tenant="$TID" -f db/seed/dev/99_reset.sql
```

Roughly what lands: ~2550 orders over three days on an evening-peaked curve,
about CHF 42k gross, TWINT-heavy from the shop with cash and card at two tills,
a handful of gratis orders per type, deposits on the cold drinks, a shirt with
four sizes, line refunds with their released deposit, deposit returns,
cancellations and a couple of arbitrary refunds.

Two things are deliberate rather than sloppy:

- Sale ledger rows carry no `order_line_id`, because checkout doesn't set one
  either. Only the restore paths do.
- A refunded bundle line restores no component stock, which is what the app
  currently does.

Abandoned `pending` orders are seeded too, but a running dev backend sweeps them
within half an hour — the same reclaim that happens in production, which leaves
their reservations behind as expired ones.
