package artistvoucher

import (
	"context"
	"errors"
	"testing"
	"time"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/product"
	"backend/internal/generated/ent/voucher"
	"backend/internal/qrsign"
	"backend/internal/repository"
	coreservice "backend/internal/service"

	"github.com/stretchr/testify/require"
)

type fakeProducts struct {
	byID      map[string]*ent.Product
	unbuyable map[string]struct{}
}

func (f *fakeProducts) FilterBuyable(_ context.Context, ids []string) (map[string]struct{}, error) {
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, unavailable := f.unbuyable[id]; unavailable {
			continue
		}
		if _, ok := f.byID[id]; ok {
			out[id] = struct{}{}
		}
	}
	return out, nil
}

func (f *fakeProducts) GetByIDs(_ context.Context, ids []string) ([]*ent.Product, error) {
	out := make([]*ent.Product, 0, len(ids))
	for _, id := range ids {
		if p, ok := f.byID[id]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

type mintedOrder struct {
	items  []coreservice.CheckoutItemInput
	origin string
}

type fakeMinter struct {
	minted []mintedOrder
	err    error
	total  int64
	nextID string
}

func (f *fakeMinter) PrepareAndCreateOrder(_ context.Context, in coreservice.CreateCheckoutInput, _ *string, _ *string) (*coreservice.CheckoutPreparation, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.minted = append(f.minted, mintedOrder{items: in.Items, origin: string(in.Origin)})
	id := f.nextID
	if id == "" {
		id = "order_000001"
	}
	return &coreservice.CheckoutPreparation{OrderID: id, TotalCents: f.total}, nil
}

type settlement struct {
	orderID     string
	deviceID    string
	amountCents int64
	pluginKey   string
}

type fakeSettler struct {
	calls []settlement
}

func (f *fakeSettler) SetPosPaymentGratisPlugin(_ context.Context, orderID string, deviceID *string, amountCents int64, pluginKey string) error {
	dev := ""
	if deviceID != nil {
		dev = *deviceID
	}
	f.calls = append(f.calls, settlement{orderID: orderID, deviceID: dev, amountCents: amountCents, pluginKey: pluginKey})
	return nil
}

type fakeStock struct {
	levels map[string]int
}

func (f *fakeStock) GetCurrentStockBatch(_ context.Context, productIDs []string) (map[string]int, error) {
	out := make(map[string]int, len(productIDs))
	for _, id := range productIDs {
		out[id] = f.levels[id]
	}
	return out, nil
}

func productFixtures() map[string]*ent.Product {
	withVariants := &ent.Product{ID: "prod_shirt01", Name: "T-Shirt", Type: product.TypeSimple}
	withVariants.Edges.Variants = []*ent.ProductVariant{{ID: "vrnt_size_xl", Value: "XL"}}
	return map[string]*ent.Product{
		"prod_burger1": {ID: "prod_burger1", Name: "Hamburger", Type: product.TypeSimple},
		"prod_cola001": {ID: "prod_cola001", Name: "Cola", Type: product.TypeSimple},
		"prod_menu001": {ID: "prod_menu001", Name: "Menü", Type: product.TypeBundle},
		"prod_shirt01": withVariants,
	}
}

func validBatchInput() CreateBatchInput {
	now := time.Now()
	return CreateBatchInput{
		Label:      "Strassenaktion",
		ValidFrom:  now.Add(-time.Hour),
		ValidUntil: now.Add(24 * time.Hour),
		Products:   []BatchProductInput{{ProductID: "prod_burger1", Quantity: 1}},
		Count:      3,
	}
}

func TestCreateBatchMintsDistinctSignedVouchers(t *testing.T) {
	env := newTestEnv(t, categoryFixtures())
	in := validBatchInput()
	in.Count = 4
	in.Products = []BatchProductInput{
		{ProductID: "prod_burger1", Quantity: 2},
		{ProductID: "prod_cola001", Quantity: 1},
	}
	env.stock.levels["prod_burger1"] = 200
	env.stock.levels["prod_cola001"] = 50

	batch, err := env.svc.CreateBatch(tenantCtx(), in)
	require.NoError(t, err)
	require.Equal(t, 4, batch.Issued)
	require.Equal(t, 4, batch.Outstanding)
	require.Equal(t, 0, batch.Redeemed)
	require.Equal(t, StatusActive, batch.Status)
	require.Equal(t, []BatchProduct{
		{Product: Product{ID: "prod_burger1", Name: "Hamburger", Quantity: 2}, Stock: 200},
		{Product: Product{ID: "prod_cola001", Name: "Cola", Quantity: 1}, Stock: 50},
	}, batch.Products)

	rows, err := env.repo.ListByBatch(tenantCtx(), batch.ID)
	require.NoError(t, err)
	require.Len(t, rows, 4)

	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		require.Equal(t, voucher.KindEntitlement, row.Kind)
		require.NotContains(t, seen, row.ID)
		require.NotContains(t, seen, row.Token)
		seen[row.ID] = struct{}{}
		seen[row.Token] = struct{}{}

		payload, err := qrsign.VerifyEntitlement(env.keys.pub, row.Token)
		require.NoError(t, err)
		require.Equal(t, testTenantID, payload.TenantID)
		require.Equal(t, row.ID, payload.VoucherID)
		require.Equal(t, "Strassenaktion", payload.Label)
		require.Equal(t, []qrsign.EntitlementLine{
			{ProductID: "prod_burger1", Quantity: 2},
			{ProductID: "prod_cola001", Quantity: 1},
		}, payload.Lines)
	}
}

func TestCreateBatchValidation(t *testing.T) {
	cases := map[string]struct {
		mutate func(CreateBatchInput) CreateBatchInput
		want   error
	}{
		"empty label": {
			func(in CreateBatchInput) CreateBatchInput { in.Label = ""; return in }, ErrEmptyLabel,
		},
		"label too long": {
			func(in CreateBatchInput) CreateBatchInput { in.Label = string(make([]rune, 81)); return in }, ErrLabelTooLong,
		},
		"window not forward": {
			func(in CreateBatchInput) CreateBatchInput { in.ValidUntil = in.ValidFrom; return in }, ErrInvalidWindow,
		},
		"no products": {
			func(in CreateBatchInput) CreateBatchInput { in.Products = nil; return in }, ErrNoProducts,
		},
		"count zero": {
			func(in CreateBatchInput) CreateBatchInput { in.Count = 0; return in }, ErrInvalidCount,
		},
		"count over max": {
			func(in CreateBatchInput) CreateBatchInput { in.Count = maxBatchCount + 1; return in }, ErrInvalidCount,
		},
		"quantity zero": {
			func(in CreateBatchInput) CreateBatchInput {
				in.Products = []BatchProductInput{{ProductID: "prod_burger1", Quantity: 0}}
				return in
			}, ErrInvalidQuantity,
		},
		"quantity over max": {
			func(in CreateBatchInput) CreateBatchInput {
				in.Products = []BatchProductInput{{ProductID: "prod_burger1", Quantity: maxBatchQuantity + 1}}
				return in
			}, ErrInvalidQuantity,
		},
		"duplicate product": {
			func(in CreateBatchInput) CreateBatchInput {
				in.Products = []BatchProductInput{
					{ProductID: "prod_burger1", Quantity: 1},
					{ProductID: "prod_burger1", Quantity: 1},
				}
				return in
			}, ErrDuplicateProduct,
		},
		"unknown product": {
			func(in CreateBatchInput) CreateBatchInput {
				in.Products = []BatchProductInput{{ProductID: "prod_missing", Quantity: 1}}
				return in
			}, ErrUnknownProduct,
		},
		"bundle product": {
			func(in CreateBatchInput) CreateBatchInput {
				in.Products = []BatchProductInput{{ProductID: "prod_menu001", Quantity: 1}}
				return in
			}, ErrProductNotSimple,
		},
		"product with variants": {
			func(in CreateBatchInput) CreateBatchInput {
				in.Products = []BatchProductInput{{ProductID: "prod_shirt01", Quantity: 1}}
				return in
			}, ErrProductHasVariants,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := newTestEnv(t, categoryFixtures())
			_, err := env.svc.CreateBatch(tenantCtx(), tc.mutate(validBatchInput()))
			require.ErrorIs(t, err, tc.want)
			require.Empty(t, env.repo.rows, "nothing may be minted for invalid input")
		})
	}
}

func TestCreateBatchRequiresTenant(t *testing.T) {
	env := newTestEnv(t, categoryFixtures())
	_, err := env.svc.CreateBatch(context.Background(), validBatchInput())
	require.ErrorIs(t, err, ErrNoTenant)
}

func issueBatch(t *testing.T, env *testEnv, mutate func(CreateBatchInput) CreateBatchInput) Batch {
	t.Helper()
	in := validBatchInput()
	if mutate != nil {
		in = mutate(in)
	}
	batch, err := env.svc.CreateBatch(tenantCtx(), in)
	require.NoError(t, err)
	return batch
}

func listBatchVouchers(t *testing.T, env *testEnv, batchID string) []*ent.Voucher {
	t.Helper()
	rows, err := env.repo.ListByBatch(tenantCtx(), batchID)
	require.NoError(t, err)
	require.NotEmpty(t, rows)
	return rows
}

func firstVoucherID(t *testing.T, env *testEnv, batchID string) string {
	t.Helper()
	rows, err := env.repo.ListByBatch(tenantCtx(), batchID)
	require.NoError(t, err)
	require.NotEmpty(t, rows)
	return rows[0].ID
}

func TestRedeemMintsGratisOrderAtNominalTotal(t *testing.T) {
	env := newTestEnv(t, categoryFixtures())
	env.minter.total = 1250
	env.minter.nextID = "order_abc123"
	batch := issueBatch(t, env, func(in CreateBatchInput) CreateBatchInput {
		in.Products = []BatchProductInput{
			{ProductID: "prod_burger1", Quantity: 2},
			{ProductID: "prod_cola001", Quantity: 1},
		}
		return in
	})
	vid := firstVoucherID(t, env, batch.ID)

	result, err := env.svc.Redeem(tenantCtx(), vid, "dev_station1")
	require.NoError(t, err)
	require.False(t, result.AlreadyRedeemed)
	require.Equal(t, "order_abc123", result.OrderID)
	require.Equal(t, "Strassenaktion", result.Label)
	require.Equal(t, []Product{
		{ID: "prod_burger1", Name: "Hamburger", Quantity: 2},
		{ID: "prod_cola001", Name: "Cola", Quantity: 1},
	}, result.Items)

	require.Len(t, env.minter.minted, 1)
	require.Equal(t, "pos", env.minter.minted[0].origin)
	require.Equal(t, []coreservice.CheckoutItemInput{
		{ProductID: "prod_burger1", Quantity: 2},
		{ProductID: "prod_cola001", Quantity: 1},
	}, env.minter.minted[0].items)

	require.Equal(t, []settlement{{
		orderID:     "order_abc123",
		deviceID:    "dev_station1",
		amountCents: 1250,
		pluginKey:   Key,
	}}, env.settler.calls)

	row, err := env.repo.GetByID(tenantCtx(), vid)
	require.NoError(t, err)
	require.NotNil(t, row.RedeemedAt)
	require.Equal(t, "order_abc123", row.RedeemedOrderID)
	require.Equal(t, "dev_station1", row.RedeemedDeviceID)
	require.Equal(t, StatusRedeemed, computeStatus(time.Now(), row))
}

func TestRedeemIsIdempotent(t *testing.T) {
	env := newTestEnv(t, categoryFixtures())
	env.minter.nextID = "order_first1"
	batch := issueBatch(t, env, nil)
	vid := firstVoucherID(t, env, batch.ID)

	first, err := env.svc.Redeem(tenantCtx(), vid, "dev_station1")
	require.NoError(t, err)

	env.minter.nextID = "order_secnd1"
	second, err := env.svc.Redeem(tenantCtx(), vid, "dev_station2")
	require.NoError(t, err)

	require.True(t, second.AlreadyRedeemed)
	require.Equal(t, first.OrderID, second.OrderID)
	require.Equal(t, first.RedeemedAt.UnixNano(), second.RedeemedAt.UnixNano())
	require.Equal(t, first.Items, second.Items)
	require.Len(t, env.minter.minted, 1, "a retry must not mint a second order")
	require.Len(t, env.settler.calls, 1)
	require.Equal(t, 1, env.repo.redeemCalls)
}

func TestRedeemRejectsUnredeemableVouchers(t *testing.T) {
	now := time.Now()
	cases := map[string]struct {
		mutate func(CreateBatchInput) CreateBatchInput
		revoke bool
		want   error
	}{
		"revoked": {nil, true, ErrRevoked},
		"scheduled": {func(in CreateBatchInput) CreateBatchInput {
			in.ValidFrom = now.Add(time.Hour)
			in.ValidUntil = now.Add(2 * time.Hour)
			return in
		}, false, ErrNotYetValid},
		"expired": {func(in CreateBatchInput) CreateBatchInput {
			in.ValidFrom = now.Add(-2 * time.Hour)
			in.ValidUntil = now.Add(-time.Hour)
			return in
		}, false, ErrExpired},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := newTestEnv(t, categoryFixtures())
			batch := issueBatch(t, env, tc.mutate)
			vid := firstVoucherID(t, env, batch.ID)
			if tc.revoke {
				_, err := env.svc.RevokeBatch(tenantCtx(), batch.ID)
				require.NoError(t, err)
			}

			_, err := env.svc.Redeem(tenantCtx(), vid, "dev_station1")
			require.ErrorIs(t, err, tc.want)
			require.Empty(t, env.minter.minted, "a rejected voucher must not mint an order")
		})
	}
}

func TestRedeemRejectsPassVoucher(t *testing.T) {
	env := newTestEnv(t, categoryFixtures())
	pass, err := env.svc.Create(tenantCtx(), validCreateInput())
	require.NoError(t, err)

	_, err = env.svc.Redeem(tenantCtx(), pass.ID, "dev_station1")
	require.ErrorIs(t, err, ErrNotEntitlement)
	require.Empty(t, env.minter.minted)
}

func TestRedeemNotFound(t *testing.T) {
	env := newTestEnv(t, categoryFixtures())
	_, err := env.svc.Redeem(tenantCtx(), "missing12345", "dev_station1")
	require.ErrorIs(t, err, repository.ErrNotFound)
}

func TestRedeemSurfacesSoldOut(t *testing.T) {
	env := newTestEnv(t, categoryFixtures())
	env.minter.err = &coreservice.InsufficientInventoryError{
		Items: []coreservice.InsufficientInventoryItem{
			{ProductID: "prod_burger1", Name: "Hamburger", Requested: 1, Available: 0},
		},
	}
	batch := issueBatch(t, env, nil)
	vid := firstVoucherID(t, env, batch.ID)

	_, err := env.svc.Redeem(tenantCtx(), vid, "dev_station1")
	var soldOut *coreservice.InsufficientInventoryError
	require.True(t, errors.As(err, &soldOut))
	require.Equal(t, "Hamburger", soldOut.Items[0].Name)

	require.Empty(t, env.settler.calls)
	row, err := env.repo.GetByID(tenantCtx(), vid)
	require.NoError(t, err)
	require.Nil(t, row.RedeemedAt, "a sold-out redemption must leave the voucher spendable")
}

func TestListBatchesReportsCountsAndStock(t *testing.T) {
	env := newTestEnv(t, categoryFixtures())
	env.stock.levels["prod_burger1"] = 200
	batch := issueBatch(t, env, func(in CreateBatchInput) CreateBatchInput { in.Count = 20; return in })

	rows, err := env.repo.ListByBatch(tenantCtx(), batch.ID)
	require.NoError(t, err)
	_, err = env.svc.Redeem(tenantCtx(), rows[0].ID, "dev_station1")
	require.NoError(t, err)
	_, err = env.repo.SetRevoked(tenantCtx(), rows[1].ID, time.Now())
	require.NoError(t, err)

	batches, err := env.svc.ListBatches(tenantCtx())
	require.NoError(t, err)
	require.Len(t, batches, 1)
	require.Equal(t, 20, batches[0].Issued)
	require.Equal(t, 1, batches[0].Redeemed)
	require.Equal(t, 1, batches[0].Revoked)
	require.Equal(t, 18, batches[0].Outstanding)
	require.Equal(t, 200, batches[0].Products[0].Stock)
}

func TestListsAreSeparatedByKind(t *testing.T) {
	env := newTestEnv(t, categoryFixtures())
	_, err := env.svc.Create(tenantCtx(), validCreateInput())
	require.NoError(t, err)
	issueBatch(t, env, nil)

	passes, err := env.svc.List(tenantCtx())
	require.NoError(t, err)
	require.Len(t, passes, 1)

	batches, err := env.svc.ListBatches(tenantCtx())
	require.NoError(t, err)
	require.Len(t, batches, 1)
}

func TestRevokeBatchSparesRedeemedVouchers(t *testing.T) {
	env := newTestEnv(t, categoryFixtures())
	batch := issueBatch(t, env, func(in CreateBatchInput) CreateBatchInput { in.Count = 3; return in })
	rows, err := env.repo.ListByBatch(tenantCtx(), batch.ID)
	require.NoError(t, err)
	_, err = env.svc.Redeem(tenantCtx(), rows[0].ID, "dev_station1")
	require.NoError(t, err)

	revoked, err := env.svc.RevokeBatch(tenantCtx(), batch.ID)
	require.NoError(t, err)
	require.Equal(t, 1, revoked.Redeemed)
	require.Equal(t, 2, revoked.Revoked)
	require.Equal(t, 0, revoked.Outstanding)
	require.Equal(t, StatusRevoked, revoked.Status)

	after, err := env.repo.GetByID(tenantCtx(), rows[0].ID)
	require.NoError(t, err)
	require.Nil(t, after.RevokedAt, "a redeemed voucher is history and stays untouched")
}

func TestGetBatchNotFound(t *testing.T) {
	env := newTestEnv(t, categoryFixtures())
	_, err := env.svc.GetBatch(tenantCtx(), "batch_miss01")
	require.ErrorIs(t, err, repository.ErrNotFound)
}

func TestRevokeBatchNotFound(t *testing.T) {
	env := newTestEnv(t, categoryFixtures())
	_, err := env.svc.RevokeBatch(tenantCtx(), "batch_miss01")
	require.ErrorIs(t, err, repository.ErrNotFound)
}

func TestGetBatchListsItsVouchers(t *testing.T) {
	env := newTestEnv(t, categoryFixtures())
	batch := issueBatch(t, env, func(in CreateBatchInput) CreateBatchInput { in.Count = 2; return in })

	detail, err := env.svc.GetBatch(tenantCtx(), batch.ID)
	require.NoError(t, err)
	require.Equal(t, batch.ID, detail.ID)
	require.Len(t, detail.Vouchers, 2)
	for _, v := range detail.Vouchers {
		require.Equal(t, StatusActive, v.Status)
		require.NotEmpty(t, v.Token)
		require.Nil(t, v.RedeemedAt)
	}
}

func TestRedeemLocksTheVoucherRow(t *testing.T) {
	env := newTestEnv(t, categoryFixtures())
	batch := issueBatch(t, env, nil)
	vid := firstVoucherID(t, env, batch.ID)

	_, err := env.svc.Redeem(tenantCtx(), vid, "dev_station1")
	require.NoError(t, err)

	require.Contains(t, env.repo.locked, vid,
		"without the row lock two concurrent redemptions both mint an order and both move stock")
}

func TestCreateBatchRejectsUnbuyableProduct(t *testing.T) {
	env := newTestEnv(t, categoryFixtures())
	in := validBatchInput()
	env.products.unbuyable = map[string]struct{}{in.Products[0].ProductID: {}}

	_, err := env.svc.CreateBatch(tenantCtx(), in)

	require.ErrorIs(t, err, ErrProductUnavailable,
		"slips are printed long before redemption, so an unsellable product must fail at issue time")
}

func TestBatchStatusExhaustedWhenAllRedeemed(t *testing.T) {
	env := newTestEnv(t, categoryFixtures())
	in := validBatchInput()
	in.Count = 2
	batch, err := env.svc.CreateBatch(tenantCtx(), in)
	require.NoError(t, err)

	for _, v := range listBatchVouchers(t, env, batch.ID) {
		_, err := env.svc.Redeem(tenantCtx(), v.ID, "dev_station1")
		require.NoError(t, err)
	}

	got, err := env.svc.GetBatch(tenantCtx(), batch.ID)
	require.NoError(t, err)
	require.Equal(t, 0, got.Outstanding)
	require.NotEqual(t, StatusActive, got.Status, "a batch with nothing left in the field is not active")
}

func TestPassEndpointsRejectBatchSlips(t *testing.T) {
	env := newTestEnv(t, categoryFixtures())
	batch := issueBatch(t, env, nil)
	vid := firstVoucherID(t, env, batch.ID)

	_, err := env.svc.Get(tenantCtx(), vid)
	require.ErrorIs(t, err, repository.ErrNotFound)

	_, err = env.svc.Revoke(tenantCtx(), vid)
	require.ErrorIs(t, err, repository.ErrNotFound,
		"revoking one slip outside its batch would desync the batch counts")

	require.ErrorIs(t, env.svc.Delete(tenantCtx(), vid), repository.ErrNotFound)
}
