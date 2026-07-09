package integration

import (
	"context"
	"testing"

	"backend/internal/generated/ent/inventoryledger"
	"backend/internal/generated/ent/order"
	"backend/internal/generated/ent/orderline"
	"backend/internal/generated/ent/product"
	"backend/internal/generated/ent/refund"
	"backend/internal/service"

	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string { return &s }

func currentVariantStock(t *testing.T, ctx context.Context, repos *Repositories, productID, variantID string) int {
	t.Helper()
	stocks, err := repos.Inventory.GetCurrentStockByVariantBatch(ctx, []string{productID})
	require.NoError(t, err)
	return stocks[variantID]
}

func TestProductVariants_CRUD(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)
	svc := NewProductSvc(repos)
	ctx := tenantCtx()

	category := fixtures.CreateCategory("Merch", 1, true)

	t.Run("create product with variants", func(t *testing.T) {
		created, err := svc.Create(ctx, category.ID, product.TypeSimple, "T-Shirt", 2500, 0, true, nil, nil, nil,
			strPtr("Grösse"),
			[]service.VariantInput{
				{Value: "S", IsActive: true, Sequence: 0},
				{Value: "M", IsActive: true, Sequence: 1},
				{Value: "XL", PriceAdjustmentCents: 200, IsActive: true, Sequence: 2},
			})
		require.NoError(t, err)
		require.NotNil(t, created.VariantLabel)
		require.Equal(t, "Grösse", *created.VariantLabel)
		require.Len(t, created.Edges.Variants, 3)
		require.Equal(t, "S", created.Edges.Variants[0].Value)
		require.Equal(t, "XL", created.Edges.Variants[2].Value)
		require.Equal(t, int64(200), created.Edges.Variants[2].PriceAdjustmentCents)
	})

	t.Run("update upserts, creates, and deletes variants", func(t *testing.T) {
		created, err := svc.Create(ctx, category.ID, product.TypeSimple, "Hoodie", 5000, 0, true, nil, nil, nil,
			strPtr("Grösse"),
			[]service.VariantInput{
				{Value: "S", IsActive: true, Sequence: 0},
				{Value: "M", IsActive: true, Sequence: 1},
			})
		require.NoError(t, err)
		sID := created.Edges.Variants[0].ID

		updated, err := svc.Update(ctx, created.ID, category.ID, product.TypeSimple, "Hoodie", 5000, 0, true, nil, nil, nil,
			strPtr("Grösse"),
			[]service.VariantInput{
				{ID: &sID, Value: "S", PriceAdjustmentCents: -100, IsActive: false, Sequence: 0},
				{Value: "L", IsActive: true, Sequence: 1},
			})
		require.NoError(t, err)
		require.Len(t, updated.Edges.Variants, 2)
		require.Equal(t, sID, updated.Edges.Variants[0].ID)
		require.Equal(t, int64(-100), updated.Edges.Variants[0].PriceAdjustmentCents)
		require.False(t, updated.Edges.Variants[0].IsActive)
		require.Equal(t, "L", updated.Edges.Variants[1].Value)
	})

	t.Run("nil variants leaves existing untouched, empty slice removes", func(t *testing.T) {
		created, err := svc.Create(ctx, category.ID, product.TypeSimple, "Cap", 1500, 0, true, nil, nil, nil,
			nil, []service.VariantInput{{Value: "Einheit", IsActive: true}})
		require.NoError(t, err)
		require.Len(t, created.Edges.Variants, 1)

		kept, err := svc.Update(ctx, created.ID, category.ID, product.TypeSimple, "Cap", 1600, 0, true, nil, nil, nil, nil, nil)
		require.NoError(t, err)
		require.Len(t, kept.Edges.Variants, 1, "nil variants must not touch existing rows")

		cleared, err := svc.Update(ctx, created.ID, category.ID, product.TypeSimple, "Cap", 1600, 0, true, nil, nil, nil, nil, []service.VariantInput{})
		require.NoError(t, err)
		require.Empty(t, cleared.Edges.Variants)
	})

	t.Run("duplicate variant values rejected", func(t *testing.T) {
		_, err := svc.Create(ctx, category.ID, product.TypeSimple, "Beanie", 1800, 0, true, nil, nil, nil,
			nil, []service.VariantInput{{Value: "M", IsActive: true}, {Value: "M", IsActive: true}})
		require.Error(t, err)
		require.Contains(t, err.Error(), "duplicate variant value")
	})

	t.Run("variants rejected on bundle products", func(t *testing.T) {
		_, err := svc.Create(ctx, category.ID, product.TypeBundle, "Combo", 1500, 0, true, nil, nil, nil,
			nil, []service.VariantInput{{Value: "M", IsActive: true}})
		require.Error(t, err)
		require.Contains(t, err.Error(), "simple products")
	})

	t.Run("variant of another product rejected on update", func(t *testing.T) {
		a, err := svc.Create(ctx, category.ID, product.TypeSimple, "A", 1000, 0, true, nil, nil, nil,
			nil, []service.VariantInput{{Value: "M", IsActive: true}})
		require.NoError(t, err)
		foreign := a.Edges.Variants[0].ID

		b, err := svc.Create(ctx, category.ID, product.TypeSimple, "B", 1000, 0, true, nil, nil, nil, nil, nil)
		require.NoError(t, err)

		_, err = svc.Update(ctx, b.ID, category.ID, product.TypeSimple, "B", 1000, 0, true, nil, nil, nil,
			nil, []service.VariantInput{{ID: &foreign, Value: "M", IsActive: true}})
		require.Error(t, err)
		require.Contains(t, err.Error(), "does not belong")
	})

	t.Run("swapping a value in one update frees the unique slot", func(t *testing.T) {
		created, err := svc.Create(ctx, category.ID, product.TypeSimple, "Shirt", 2000, 0, true, nil, nil, nil,
			nil, []service.VariantInput{{Value: "M", IsActive: true}})
		require.NoError(t, err)
		oldID := created.Edges.Variants[0].ID

		updated, err := svc.Update(ctx, created.ID, category.ID, product.TypeSimple, "Shirt", 2000, 0, true, nil, nil, nil,
			nil, []service.VariantInput{{Value: "M", PriceAdjustmentCents: 100, IsActive: true}})
		require.NoError(t, err, "removing old M and adding new M in one update must not collide")
		require.Len(t, updated.Edges.Variants, 1)
		require.NotEqual(t, oldID, updated.Edges.Variants[0].ID)
		require.Equal(t, int64(100), updated.Edges.Variants[0].PriceAdjustmentCents)
	})

	t.Run("re-adding a referenced value revives the archived row", func(t *testing.T) {
		created, err := svc.Create(ctx, category.ID, product.TypeSimple, "Longsleeve", 3000, 0, true, nil, nil, nil,
			nil, []service.VariantInput{{Value: "M", IsActive: true}})
		require.NoError(t, err)
		vID := created.Edges.Variants[0].ID

		fixtures := NewFixtures(repos)
		fixtures.AddVariantInventory(created.ID, vID, 3, inventoryledger.ReasonOpeningBalance)

		updated, err := svc.Update(ctx, created.ID, category.ID, product.TypeSimple, "Longsleeve", 3000, 0, true, nil, nil, nil,
			nil, []service.VariantInput{{Value: "M", PriceAdjustmentCents: 50, IsActive: true}})
		require.NoError(t, err, "referenced M cannot be deleted; the id-less input must reuse it")
		require.Len(t, updated.Edges.Variants, 1)
		require.Equal(t, vID, updated.Edges.Variants[0].ID)
		require.True(t, updated.Edges.Variants[0].IsActive)
		require.Equal(t, int64(50), updated.Edges.Variants[0].PriceAdjustmentCents)
	})

	t.Run("referenced variant is archived instead of deleted", func(t *testing.T) {
		created, err := svc.Create(ctx, category.ID, product.TypeSimple, "Pulli", 4000, 0, true, nil, nil, nil,
			nil, []service.VariantInput{{Value: "M", IsActive: true}})
		require.NoError(t, err)
		vID := created.Edges.Variants[0].ID

		fixtures := NewFixtures(repos)
		fixtures.AddVariantInventory(created.ID, vID, 5, inventoryledger.ReasonOpeningBalance)

		updated, err := svc.Update(ctx, created.ID, category.ID, product.TypeSimple, "Pulli", 4000, 0, true, nil, nil, nil,
			nil, []service.VariantInput{})
		require.NoError(t, err)
		require.Len(t, updated.Edges.Variants, 1, "ledger-referenced variant must survive as archived")
		require.Equal(t, vID, updated.Edges.Variants[0].ID)
		require.False(t, updated.Edges.Variants[0].IsActive)
	})

	t.Run("delete product removes unreferenced variants", func(t *testing.T) {
		created, err := svc.Create(ctx, category.ID, product.TypeSimple, "Sticker", 300, 0, true, nil, nil, nil,
			nil, []service.VariantInput{{Value: "Gross", IsActive: true}})
		require.NoError(t, err)

		require.NoError(t, svc.Delete(ctx, created.ID))
		variants, err := repos.ProductVariant.GetByProductID(ctx, created.ID)
		require.NoError(t, err)
		require.Empty(t, variants)
	})
}

func TestProductVariants_BundleInterlock(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)
	svc := NewProductSvc(repos)
	ctx := tenantCtx()

	category := fixtures.CreateCategory("Bundles", 1, true)
	bundle := fixtures.CreateProduct("Combo", category.ID, 1500, product.TypeBundle, nil)
	slot := fixtures.CreateBundleSlot(bundle.ID, "Getränk", 0)

	t.Run("variant product cannot become a slot option", func(t *testing.T) {
		shirt, err := svc.Create(ctx, category.ID, product.TypeSimple, "T-Shirt", 2500, 0, true, nil, nil, nil,
			nil, []service.VariantInput{{Value: "M", IsActive: true}})
		require.NoError(t, err)

		_, err = svc.AddSlotOption(ctx, bundle.ID, slot.ID, shirt.ID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot be bundle options")
	})

	t.Run("slot option product cannot gain variants", func(t *testing.T) {
		cola := fixtures.CreateProduct("Cola", category.ID, 350, product.TypeSimple, nil)
		_, err := svc.AddSlotOption(ctx, bundle.ID, slot.ID, cola.ID)
		require.NoError(t, err)

		_, err = svc.Update(ctx, cola.ID, category.ID, product.TypeSimple, "Cola", 350, 0, true, nil, nil, nil,
			nil, []service.VariantInput{{Value: "M", IsActive: true}})
		require.Error(t, err)
		require.Contains(t, err.Error(), "bundle options cannot have variants")
	})
}

func TestProductVariants_AdjustStock(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)
	svc := NewProductSvc(repos)
	ctx := tenantCtx()

	category := fixtures.CreateCategory("Merch", 1, true)
	shirt := fixtures.CreateProduct("T-Shirt", category.ID, 2500, product.TypeSimple, nil)
	m := fixtures.CreateVariant(shirt.ID, "M", 0, true, 0)
	l := fixtures.CreateVariant(shirt.ID, "L", 0, true, 1)
	plain := fixtures.CreateProduct("Sticker", category.ID, 300, product.TypeSimple, nil)

	t.Run("variantId required for variant products", func(t *testing.T) {
		err := svc.AdjustStock(ctx, shirt.ID, nil, 10, "opening_balance")
		require.Error(t, err)
		require.Contains(t, err.Error(), "variantId is required")
	})

	t.Run("variantId forbidden for plain products", func(t *testing.T) {
		err := svc.AdjustStock(ctx, plain.ID, &m.ID, 10, "opening_balance")
		require.Error(t, err)
		require.Contains(t, err.Error(), "not allowed")
	})

	t.Run("variant must belong to product", func(t *testing.T) {
		other := fixtures.CreateProduct("Hoodie", category.ID, 5000, product.TypeSimple, nil)
		v := fixtures.CreateVariant(other.ID, "M", 0, true, 0)
		err := svc.AdjustStock(ctx, shirt.ID, &v.ID, 10, "opening_balance")
		require.Error(t, err)
		require.Contains(t, err.Error(), "does not belong")
	})

	t.Run("per-variant stock sums independently", func(t *testing.T) {
		require.NoError(t, svc.AdjustStock(ctx, shirt.ID, &m.ID, 12, "opening_balance"))
		require.NoError(t, svc.AdjustStock(ctx, shirt.ID, &l.ID, 4, "opening_balance"))
		require.NoError(t, svc.AdjustStock(ctx, shirt.ID, &m.ID, -2, "manual_adjust"))

		require.Equal(t, 10, currentVariantStock(t, ctx, repos, shirt.ID, m.ID))
		require.Equal(t, 4, currentVariantStock(t, ctx, repos, shirt.ID, l.ID))
		require.Equal(t, 14, currentStock(t, ctx, repos, shirt.ID), "product total spans all variants")
	})
}

func TestPaymentService_VariantCheckout(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)
	svc := NewPaymentSvc(TestConfig(), repos, NewProductSvc(repos))
	ctx := tenantCtx()

	category := fixtures.CreateCategory("Merch", 1, true)
	shirt := fixtures.CreateProduct("T-Shirt", category.ID, 2500, product.TypeSimple, nil)
	m := fixtures.CreateVariant(shirt.ID, "M", 0, true, 0)
	xl := fixtures.CreateVariant(shirt.ID, "XL", 200, true, 1)
	inactive := fixtures.CreateVariant(shirt.ID, "XS", 0, false, 2)
	fixtures.AddVariantInventory(shirt.ID, m.ID, 10, inventoryledger.ReasonOpeningBalance)
	fixtures.AddVariantInventory(shirt.ID, xl.ID, 2, inventoryledger.ReasonOpeningBalance)

	cola := fixtures.CreateProduct("Cola", category.ID, 350, product.TypeSimple, nil)
	fixtures.AddInventory(cola.ID, 100, inventoryledger.ReasonOpeningBalance)

	t.Run("variant checkout snapshots variant and adjusts price", func(t *testing.T) {
		prep, err := svc.PrepareAndCreateOrder(ctx, service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{
				{ProductID: shirt.ID, Quantity: 1, VariantID: &xl.ID},
				{ProductID: cola.ID, Quantity: 1},
			},
		}, nil, nil)
		require.NoError(t, err)
		require.Equal(t, int64(2700+350), prep.TotalCents)

		lines, err := repos.OrderLine.GetByOrderID(ctx, prep.OrderID)
		require.NoError(t, err)
		var shirtLine, colaLine bool
		for _, l := range lines {
			switch l.ProductID {
			case shirt.ID:
				shirtLine = true
				require.NotNil(t, l.VariantID)
				require.Equal(t, xl.ID, *l.VariantID)
				require.NotNil(t, l.VariantValue)
				require.Equal(t, "XL", *l.VariantValue)
				require.Equal(t, int64(2700), l.UnitPriceCents)
				require.Equal(t, "T-Shirt", l.Title)
			case cola.ID:
				colaLine = true
				require.Nil(t, l.VariantID)
				require.Nil(t, l.VariantValue)
			}
		}
		require.True(t, shirtLine)
		require.True(t, colaLine)

		var gatewayTitles []string
		for _, li := range prep.LineItems {
			gatewayTitles = append(gatewayTitles, li.Name)
		}
		require.Contains(t, gatewayTitles, "T-Shirt (XL)")

		require.Equal(t, 1, currentVariantStock(t, ctx, repos, shirt.ID, xl.ID), "variant stock reserved")
		require.Equal(t, 10, currentVariantStock(t, ctx, repos, shirt.ID, m.ID), "sibling variant untouched")
	})

	t.Run("variantId required for variant products", func(t *testing.T) {
		_, err := svc.PrepareAndCreateOrder(ctx, service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{{ProductID: shirt.ID, Quantity: 1}},
		}, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "variant selection required")
	})

	t.Run("variantId forbidden for plain products", func(t *testing.T) {
		_, err := svc.PrepareAndCreateOrder(ctx, service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{{ProductID: cola.ID, Quantity: 1, VariantID: &m.ID}},
		}, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "has no variants")
	})

	t.Run("inactive variant rejected", func(t *testing.T) {
		_, err := svc.PrepareAndCreateOrder(ctx, service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{{ProductID: shirt.ID, Quantity: 1, VariantID: &inactive.ID}},
		}, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "not available")
	})

	t.Run("foreign variant rejected", func(t *testing.T) {
		other := fixtures.CreateProduct("Hoodie", category.ID, 5000, product.TypeSimple, nil)
		v := fixtures.CreateVariant(other.ID, "M", 0, true, 0)
		_, err := svc.PrepareAndCreateOrder(ctx, service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{{ProductID: shirt.ID, Quantity: 1, VariantID: &v.ID}},
		}, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "does not belong")
	})

	t.Run("per-variant stock enforced across items", func(t *testing.T) {
		_, err := svc.PrepareAndCreateOrder(ctx, service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{
				{ProductID: shirt.ID, Quantity: 1, VariantID: &xl.ID},
				{ProductID: shirt.ID, Quantity: 1, VariantID: &xl.ID},
			},
		}, nil, nil)
		require.Error(t, err, "only 1 XL left after the first checkout")
		require.Contains(t, err.Error(), "insufficient inventory for T-Shirt (XL)")

		prep, err := svc.PrepareAndCreateOrder(ctx, service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{
				{ProductID: shirt.ID, Quantity: 1, VariantID: &xl.ID},
				{ProductID: shirt.ID, Quantity: 2, VariantID: &m.ID},
			},
		}, nil, nil)
		require.NoError(t, err, "different variants draw on separate stock")
		require.Equal(t, int64(2700+2*2500), prep.TotalCents)

		lines, err := repos.OrderLine.GetByOrderID(ctx, prep.OrderID)
		require.NoError(t, err)
		require.Len(t, lines, 2, "same product in two variants yields two lines")
	})

	t.Run("cleanup releases variant stock", func(t *testing.T) {
		mBefore := currentVariantStock(t, ctx, repos, shirt.ID, m.ID)
		prep, err := svc.PrepareAndCreateOrder(ctx, service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{{ProductID: shirt.ID, Quantity: 2, VariantID: &m.ID}},
		}, nil, nil)
		require.NoError(t, err)
		require.Equal(t, mBefore-2, currentVariantStock(t, ctx, repos, shirt.ID, m.ID))

		require.NoError(t, svc.CleanupPendingOrderByID(ctx, prep.OrderID))
		require.Equal(t, mBefore, currentVariantStock(t, ctx, repos, shirt.ID, m.ID))
	})

	t.Run("deposit line stays product-level for variant items", func(t *testing.T) {
		cup := fixtures.CreateProductWithDeposit("Becher", category.ID, 500, 200)
		v := fixtures.CreateVariant(cup.ID, "Gross", 100, true, 0)
		fixtures.AddVariantInventory(cup.ID, v.ID, 5, inventoryledger.ReasonOpeningBalance)

		prep, err := svc.PrepareAndCreateOrder(ctx, service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{{ProductID: cup.ID, Quantity: 1, VariantID: &v.ID}},
		}, nil, nil)
		require.NoError(t, err)
		require.Equal(t, int64(600+200), prep.TotalCents, "variant-adjusted price plus unadjusted deposit")

		lines, err := repos.OrderLine.GetByOrderID(ctx, prep.OrderID)
		require.NoError(t, err)
		for _, l := range lines {
			if l.LineType == orderline.LineTypeDeposit {
				require.Nil(t, l.VariantID, "deposit lines carry no variant")
				require.Equal(t, int64(200), l.UnitPriceCents)
			}
		}
	})
}

func TestRefundService_VariantRestock(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)
	ctx := tenantCtx()
	paymentSvc := NewPaymentSvc(TestConfig(), repos, NewProductSvc(repos))
	svc := newRefundSvc(repos, &stubRefundProvider{})

	category := fixtures.CreateCategory("Merch-"+randSuffix(), 1, true)
	shirt := fixtures.CreateProduct("T-Shirt", category.ID, 2500, product.TypeSimple, nil)
	m := fixtures.CreateVariant(shirt.ID, "M", 0, true, 0)
	fixtures.AddVariantInventory(shirt.ID, m.ID, 10, inventoryledger.ReasonOpeningBalance)

	prep, err := paymentSvc.PrepareAndCreateOrder(ctx, service.CreateCheckoutInput{
		Items: []service.CheckoutItemInput{{ProductID: shirt.ID, Quantity: 3, VariantID: &m.ID}},
	}, nil, nil)
	require.NoError(t, err)

	ord, err := repos.Order.GetByID(ctx, prep.OrderID)
	require.NoError(t, err)
	_, err = repos.Order.Update(ctx, ord.ID, ord.TotalCents, order.StatusPaid, ord.Origin, ord.CustomerID, ord.ContactEmail, ord.PaymentAttemptID, ord.PayrexxGatewayID, nil)
	require.NoError(t, err)

	require.Equal(t, 7, currentVariantStock(t, ctx, repos, shirt.ID, m.ID))
	lineID := firstLineID(t, ctx, repos, ord.ID)

	_, err = svc.RefundLines(ctx, service.RefundLinesInput{
		OrderID: ord.ID,
		Lines:   []service.RefundLineQty{{OrderLineID: lineID, Quantity: 1}},
		Actor:   service.RefundActor{Type: refund.ActorTypeOperator, ID: "op_1"},
	})
	require.NoError(t, err)

	require.Equal(t, 8, currentVariantStock(t, ctx, repos, shirt.ID, m.ID), "refund restocks the exact variant")

	entries, err := repos.Inventory.GetByProductID(ctx, shirt.ID)
	require.NoError(t, err)
	var refundEntry bool
	for _, e := range entries {
		if e.Reason == inventoryledger.ReasonRefund {
			refundEntry = true
			require.NotNil(t, e.VariantID)
			require.Equal(t, m.ID, *e.VariantID)
		}
	}
	require.True(t, refundEntry, "refund ledger row present")
}
