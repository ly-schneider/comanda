package integration

import (
	"testing"

	"backend/internal/service"

	entDevice "backend/internal/generated/ent/device"
	entOrder "backend/internal/generated/ent/order"
	"backend/internal/generated/ent/orderline"
	"backend/internal/generated/ent/product"

	nanoid "backend/internal/id"
	"github.com/stretchr/testify/require"
)

// redemptionRowCount counts order_line_redemption rows on the privileged pool
// (RLS bypassed) — used to prove no duplicate rows are written across replays.
func redemptionRowCount(t *testing.T, tdb *TestDB, where string, args ...any) int {
	t.Helper()
	var n int
	q := "SELECT count(*) FROM order_line_redemption"
	if where != "" {
		q += " WHERE " + where
	}
	require.NoError(t, tdb.DB.QueryRow(q, args...).Scan(&n))
	return n
}

// TestStationService_RedeemAssigned_DBDedup proves the bulk INSERT … ON CONFLICT
// (order_line_id) DO NOTHING path redeems each assigned line exactly once: a
// second redeem of the same order (no idempotency key) reports 0 newly-redeemed
// and writes no duplicate order_line_redemption rows.
func TestStationService_RedeemAssigned_DBDedup(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)

	svc := service.NewStationService(
		TestConfig(), tdb.Client,
		repos.Device, repos.DeviceProduct, repos.OrderLine, repos.OrderRedemption, repos.Idempotency,
	)
	ctx := tenantCtx()

	cat := fixtures.CreateCategory("Drinks", 1, true)
	cola := fixtures.CreateProduct("Cola", cat.ID, 350, product.TypeSimple, nil)
	sprite := fixtures.CreateProduct("Sprite", cat.ID, 350, product.TypeSimple, nil)

	station := fixtures.CreateDevice("Drinks", "station-dedup", entDevice.TypeSTATION, entDevice.StatusApproved)
	fixtures.AssignProductToDevice(station.ID, cola.ID)
	fixtures.AssignProductToDevice(station.ID, sprite.ID)

	ord := fixtures.CreateOrder(700, entOrder.StatusPaid, entOrder.OriginShop)
	lineCola := fixtures.CreateOrderLine(ord.ID, cola.ID, "Cola", 1, 350, orderline.LineTypeSimple)
	lineSprite := fixtures.CreateOrderLine(ord.ID, sprite.ID, "Sprite", 1, 350, orderline.LineTypeSimple)

	// The DB was truncated by Cleanup, so the total redemption count is exactly
	// what this test writes.
	require.Equal(t, 0, redemptionRowCount(t, tdb, ""))

	first, err := svc.RedeemAssigned(ctx, station.ID, ord.ID, "")
	require.NoError(t, err)
	require.EqualValues(t, 2, first["matched"])
	require.EqualValues(t, 2, first["redeemed"])

	require.Equal(t, 2, redemptionRowCount(t, tdb, ""))

	second, err := svc.RedeemAssigned(ctx, station.ID, ord.ID, "")
	require.NoError(t, err)
	require.EqualValues(t, 2, second["matched"], "both lines still match the station")
	require.EqualValues(t, 0, second["redeemed"], "already-redeemed lines must not redeem again")

	require.Equal(t, 2, redemptionRowCount(t, tdb, ""),
		"ON CONFLICT DO NOTHING must not create duplicate redemption rows")

	// Each line has exactly one redemption row (unique order_line_id holds).
	require.Equal(t, 1, redemptionRowCount(t, tdb, "order_line_id = $1", lineCola.ID))
	require.Equal(t, 1, redemptionRowCount(t, tdb, "order_line_id = $1", lineSprite.ID))
}

// TestStationService_RedeemAssigned_IdempotencyReplay proves the idempotency
// table path: replaying with the same Idempotency-Key returns the cached
// response and causes no second effect; a different key on an already
// fully-redeemed order recomputes to 0 newly-redeemed.
func TestStationService_RedeemAssigned_IdempotencyReplay(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)

	svc := service.NewStationService(
		TestConfig(), tdb.Client,
		repos.Device, repos.DeviceProduct, repos.OrderLine, repos.OrderRedemption, repos.Idempotency,
	)
	ctx := tenantCtx()

	cat := fixtures.CreateCategory("Drinks", 1, true)
	cola := fixtures.CreateProduct("Cola", cat.ID, 350, product.TypeSimple, nil)
	station := fixtures.CreateDevice("Drinks", "station-idem", entDevice.TypeSTATION, entDevice.StatusApproved)
	fixtures.AssignProductToDevice(station.ID, cola.ID)

	ord := fixtures.CreateOrder(350, entOrder.StatusPaid, entOrder.OriginShop)
	line := fixtures.CreateOrderLine(ord.ID, cola.ID, "Cola", 1, 350, orderline.LineTypeSimple)

	const key = "redeem-idem-key-1"

	first, err := svc.RedeemAssigned(ctx, station.ID, ord.ID, key)
	require.NoError(t, err)
	require.EqualValues(t, 1, first["redeemed"])
	require.Equal(t, 1, redemptionRowCount(t, tdb, "order_line_id = $1", line.ID))

	// Replay with the SAME key: cached response, byte-for-byte redeemedAt, no
	// second effect.
	replay, err := svc.RedeemAssigned(ctx, station.ID, ord.ID, key)
	require.NoError(t, err)
	require.EqualValues(t, 1, replay["redeemed"], "replay returns the cached redeemed count")
	require.EqualValues(t, 1, replay["matched"])
	require.Equal(t, first["redeemedAt"], replay["redeemedAt"], "replay must serve the cached timestamp")
	require.Equal(t, 1, redemptionRowCount(t, tdb, "order_line_id = $1", line.ID),
		"replay under the same key must not write a second redemption row")

	// A DIFFERENT key recomputes (cache miss) but the line is already redeemed,
	// so it reports 0 newly-redeemed and still writes no extra row.
	other, err := svc.RedeemAssigned(ctx, station.ID, ord.ID, "redeem-idem-key-2")
	require.NoError(t, err)
	require.EqualValues(t, 1, other["matched"], "the line still matches the station")
	require.EqualValues(t, 0, other["redeemed"], "an already fully-redeemed order yields 0 newly-redeemed")
	require.Equal(t, 1, redemptionRowCount(t, tdb, "order_line_id = $1", line.ID))
}

// TestStationService_RedeemAssigned_CrossTenant proves RLS isolation on the
// redeem path: a station bound to tenant A, run inside tenant A's request tx,
// cannot redeem an order line that belongs to tenant B — even when that line
// references a product assigned to station A (so only RLS, not the device-match
// query, prevents the redemption). The positive control redeems tenant A's own
// line to guard against a false pass.
func TestStationService_RedeemAssigned_CrossTenant(t *testing.T) {
	env := newRLSEnv(t)

	tenantA, productIDs := env.seedTenantWithProducts(t, "redeem-a-"+randSuffix(), 1)
	productA := productIDs[0]
	tenantB := env.seedTenant(t, "redeem-b-"+randSuffix())

	stationA := nanoid.New()
	env.privExec(t,
		`INSERT INTO device (id, name, device_key, type, status, tenant_id)
		 VALUES ($1, 'St-A', $2, 'STATION', 'approved', $3)`,
		stationA, "station-xt-"+randSuffix(), tenantA,
	)
	env.privExec(t,
		`INSERT INTO device_product (device_id, product_id, tenant_id) VALUES ($1, $2, $3)`,
		stationA, productA, tenantA,
	)

	// Tenant B's order + line referencing productA (FK is tenant-agnostic; RLS is
	// the only thing that should hide this line from station A).
	orderB := nanoid.New()
	lineB := nanoid.New()
	env.privExec(t,
		`INSERT INTO "order" (id, total_cents, status, origin, tenant_id)
		 VALUES ($1, 100, 'paid', 'shop', $2)`,
		orderB, tenantB,
	)
	env.privExec(t,
		`INSERT INTO order_line (id, order_id, line_type, product_id, title, quantity, unit_price_cents, tenant_id)
		 VALUES ($1, $2, 'simple', $3, 'Cola', 1, 100, $4)`,
		lineB, orderB, productA, tenantB,
	)

	// Tenant A's own order + line (positive control).
	orderA := nanoid.New()
	lineA := nanoid.New()
	env.privExec(t,
		`INSERT INTO "order" (id, total_cents, status, origin, tenant_id)
		 VALUES ($1, 100, 'paid', 'shop', $2)`,
		orderA, tenantA,
	)
	env.privExec(t,
		`INSERT INTO order_line (id, order_id, line_type, product_id, title, quantity, unit_price_cents, tenant_id)
		 VALUES ($1, $2, 'simple', $3, 'Cola', 1, 100, $4)`,
		lineA, orderA, productA, tenantA,
	)

	ctx, client, cleanup := env.appRequestTx(t, tenantA)
	defer cleanup()
	repos := NewRepositories(client)
	svc := service.NewStationService(
		TestConfig(), client,
		repos.Device, repos.DeviceProduct, repos.OrderLine, repos.OrderRedemption, repos.Idempotency,
	)

	xt, err := svc.RedeemAssigned(ctx, stationA, orderB, "")
	require.NoError(t, err)
	require.EqualValues(t, 0, xt["matched"], "tenant A must not match tenant B's order line")
	require.EqualValues(t, 0, xt["redeemed"], "tenant A must not redeem tenant B's order line")

	exists, err := repos.OrderRedemption.ExistsByOrderLineID(ctx, lineB)
	require.NoError(t, err)
	require.False(t, exists, "no redemption may exist for tenant B's line from tenant A's context")

	own, err := svc.RedeemAssigned(ctx, stationA, orderA, "")
	require.NoError(t, err)
	require.EqualValues(t, 1, own["matched"])
	require.EqualValues(t, 1, own["redeemed"])

	// Privileged pool (RLS bypassed) confirms the only redemption written is for
	// tenant A's line — tenant B's line was never touched.
	var nB int
	require.NoError(t, env.priv.QueryRow(
		`SELECT count(*) FROM order_line_redemption WHERE order_line_id = $1`, lineB,
	).Scan(&nB))
	require.Equal(t, 0, nB, "tenant B's line must have zero redemption rows")
}
