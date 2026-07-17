package integration

import (
	"testing"

	"backend/internal/generated/ent"
	"backend/internal/generated/ent/inventoryledger"
	"backend/internal/generated/ent/order"
	"backend/internal/generated/ent/orderline"
	"backend/internal/generated/ent/product"
	"backend/internal/qrsign"
	"backend/internal/repository"
	"backend/internal/service"

	nanoid "backend/internal/id"
	"github.com/stretchr/testify/require"
)

func TestPaymentService_PrepareAndCreateOrder(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)
	cfg := TestConfig()

	svc := NewPaymentSvc(cfg, repos, NewProductSvc(repos))
	ctx := tenantCtx()

	// Setup test products
	category := fixtures.CreateCategory("Drinks", 1, true)
	cola := fixtures.CreateProduct("Cola", category.ID, 350, product.TypeSimple, nil)
	sprite := fixtures.CreateProduct("Sprite", category.ID, 350, product.TypeSimple, nil)

	// Add inventory
	fixtures.AddInventory(cola.ID, 100, inventoryledger.ReasonOpeningBalance)
	fixtures.AddInventory(sprite.ID, 100, inventoryledger.ReasonOpeningBalance)

	t.Run("PrepareAndCreateOrder creates order with simple products", func(t *testing.T) {
		input := service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{
				{ProductID: cola.ID, Quantity: 2},
				{ProductID: sprite.ID, Quantity: 1},
			},
		}

		prep, err := svc.PrepareAndCreateOrder(ctx, input, nil, nil)
		require.NoError(t, err)
		require.NotEqual(t, "", prep.OrderID)
		require.Equal(t, int64(1050), prep.TotalCents) // 2*350 + 1*350
		require.Len(t, prep.LineItems, 2)

		require.NotNil(t, prep.Order, "prep.Order must be populated so handlers can skip the post-create re-fetch")
		require.Equal(t, prep.OrderID, prep.Order.ID)
		require.Equal(t, order.StatusPending, prep.Order.Status)
		require.Equal(t, order.OriginShop, prep.Order.Origin)
		require.Equal(t, int64(1050), prep.Order.TotalCents)

		orderObj, err := repos.Order.GetByID(ctx, prep.OrderID)
		require.NoError(t, err)
		require.Equal(t, order.StatusPending, orderObj.Status)
		require.Equal(t, order.OriginShop, orderObj.Origin)
	})

	t.Run("PrepareAndCreateOrder with user ID", func(t *testing.T) {
		userID := nanoid.New()
		require.NoError(t, repos.Client.User.Create().SetID(userID).Exec(ctx))
		input := service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{
				{ProductID: cola.ID, Quantity: 1},
			},
		}

		prep, err := svc.PrepareAndCreateOrder(ctx, input, &userID, nil)
		require.NoError(t, err)
		require.Equal(t, &userID, prep.UserID)

		require.NotNil(t, prep.Order)
		require.NotNil(t, prep.Order.CustomerID)
		require.Equal(t, userID, *prep.Order.CustomerID)

		orderObj, err := repos.Order.GetByID(ctx, prep.OrderID)
		require.NoError(t, err)
		require.NotNil(t, orderObj.CustomerID)
		require.Equal(t, userID, *orderObj.CustomerID)
	})

	t.Run("PrepareAndCreateOrder with attempt ID", func(t *testing.T) {
		attemptID := "test-attempt-123"
		input := service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{
				{ProductID: cola.ID, Quantity: 1},
			},
		}

		prep, err := svc.PrepareAndCreateOrder(ctx, input, nil, &attemptID)
		require.NoError(t, err)

		orderObj, err := repos.Order.GetByID(ctx, prep.OrderID)
		require.NoError(t, err)
		require.NotNil(t, orderObj.PaymentAttemptID)
		require.Equal(t, attemptID, *orderObj.PaymentAttemptID)
	})

	t.Run("PrepareAndCreateOrder reserves inventory", func(t *testing.T) {
		// Check initial stock
		initialStock, err := repos.Inventory.GetCurrentStock(ctx, cola.ID)
		require.NoError(t, err)

		input := service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{
				{ProductID: cola.ID, Quantity: 5},
			},
		}

		_, err = svc.PrepareAndCreateOrder(ctx, input, nil, nil)
		require.NoError(t, err)

		// Check stock was reduced
		newStock, err := repos.Inventory.GetCurrentStock(ctx, cola.ID)
		require.NoError(t, err)
		require.Equal(t, initialStock-5, newStock)
	})

	t.Run("PrepareAndCreateOrder with no items fails", func(t *testing.T) {
		input := service.CreateCheckoutInput{Items: []service.CheckoutItemInput{}}

		_, err := svc.PrepareAndCreateOrder(ctx, input, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no items")
	})

	t.Run("PrepareAndCreateOrder with invalid product ID fails", func(t *testing.T) {
		input := service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{
				{ProductID: "invalid-uuid", Quantity: 1},
			},
		}

		_, err := svc.PrepareAndCreateOrder(ctx, input, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid productId")
	})

	t.Run("PrepareAndCreateOrder with non-existent product fails", func(t *testing.T) {
		input := service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{
				{ProductID: nanoid.New(), Quantity: 1},
			},
		}

		_, err := svc.PrepareAndCreateOrder(ctx, input, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown product")
	})
}

// Deactivating a product — or its category, or that category's parent — must
// make it unbuyable at the shared checkout choke point, for the shop and POS.
func TestPaymentService_DeactivatedProductsRejected(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)
	cfg := TestConfig()

	svc := NewPaymentSvc(cfg, repos, NewProductSvc(repos))
	ctx := tenantCtx()

	activeCat := fixtures.CreateCategory("Active", 1, true)
	inactiveCat := fixtures.CreateCategory("Inactive", 2, false)

	// An active child nested under an inactive parent — the cascade case.
	inactiveParent := fixtures.CreateCategory("Food", 3, false)
	activeChildOfInactive := fixtures.CreateSubCategory("Burgers", 1, true, inactiveParent.ID)

	// Positive control: an active child under an active parent.
	activeParent := fixtures.CreateCategory("Drinks", 4, true)
	activeChild := fixtures.CreateSubCategory("Softdrinks", 1, true, activeParent.ID)

	inactiveProduct := fixtures.CreateInactiveProduct("Hidden", activeCat.ID, 350)
	productInInactiveCat := fixtures.CreateProduct("InCatOff", inactiveCat.ID, 350, product.TypeSimple, nil)
	productUnderInactiveParent := fixtures.CreateProduct("Burger", activeChildOfInactive.ID, 900, product.TypeSimple, nil)
	buyable := fixtures.CreateProduct("Cola", activeChild.ID, 350, product.TypeSimple, nil)

	for _, id := range []string{inactiveProduct.ID, productInInactiveCat.ID, productUnderInactiveParent.ID, buyable.ID} {
		fixtures.AddInventory(id, 100, inventoryledger.ReasonOpeningBalance)
	}

	assertUnavailable := func(t *testing.T, productID string) {
		t.Helper()
		input := service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{{ProductID: productID, Quantity: 1}},
		}
		_, err := svc.PrepareAndCreateOrder(ctx, input, nil, nil)
		require.Error(t, err)
		var unavailable *service.ProductUnavailableError
		require.ErrorAs(t, err, &unavailable)
		require.Len(t, unavailable.Items, 1)
		require.Equal(t, productID, unavailable.Items[0].ProductID)
	}

	t.Run("inactive product is rejected", func(t *testing.T) {
		assertUnavailable(t, inactiveProduct.ID)
	})

	t.Run("product in an inactive category is rejected", func(t *testing.T) {
		assertUnavailable(t, productInInactiveCat.ID)
	})

	t.Run("product under an inactive parent category is rejected", func(t *testing.T) {
		assertUnavailable(t, productUnderInactiveParent.ID)
	})

	t.Run("buyable product under an active category chain succeeds", func(t *testing.T) {
		input := service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{{ProductID: buyable.ID, Quantity: 1}},
		}
		prep, err := svc.PrepareAndCreateOrder(ctx, input, nil, nil)
		require.NoError(t, err)
		require.NotEqual(t, "", prep.OrderID)
	})
}

func TestPaymentService_BundleCheckout(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)
	cfg := TestConfig()

	svc := NewPaymentSvc(cfg, repos, NewProductSvc(repos))
	ctx := tenantCtx()

	// Setup bundle product
	category := fixtures.CreateCategory("Bundles", 1, true)
	optionCat := fixtures.CreateCategory("Options", 2, true)

	cola := fixtures.CreateProduct("Cola", optionCat.ID, 350, product.TypeSimple, nil)
	fries := fixtures.CreateProduct("Fries", optionCat.ID, 400, product.TypeSimple, nil)

	fixtures.AddInventory(cola.ID, 50, inventoryledger.ReasonOpeningBalance)
	fixtures.AddInventory(fries.ID, 50, inventoryledger.ReasonOpeningBalance)

	bundle := fixtures.CreateProduct("Combo Menu", category.ID, 1500, product.TypeBundle, nil)

	drinkSlot := fixtures.CreateBundleSlot(bundle.ID, "Drink", 1)
	sideSlot := fixtures.CreateBundleSlot(bundle.ID, "Side", 2)

	fixtures.CreateBundleSlotOption(drinkSlot.ID, cola.ID)
	fixtures.CreateBundleSlotOption(sideSlot.ID, fries.ID)

	t.Run("PrepareAndCreateOrder with bundle configuration", func(t *testing.T) {
		input := service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{
				{
					ProductID: bundle.ID,
					Quantity:  1,
					Configuration: map[string]string{
						drinkSlot.ID: cola.ID,
						sideSlot.ID:  fries.ID,
					},
				},
			},
		}

		prep, err := svc.PrepareAndCreateOrder(ctx, input, nil, nil)
		require.NoError(t, err)
		require.Equal(t, int64(1500), prep.TotalCents) // Bundle price only

		// Verify order lines were created
		lines, err := repos.OrderLine.GetByOrderID(ctx, prep.OrderID)
		require.NoError(t, err)

		// Should have 3 lines: bundle parent + 2 components
		require.Len(t, lines, 3)

		var bundleCount, componentCount int
		for _, line := range lines {
			if line.LineType == orderline.LineTypeBundle {
				bundleCount++
				require.Equal(t, "Combo Menu", line.Title)
			}
			if line.LineType == orderline.LineTypeComponent {
				componentCount++
				require.NotNil(t, line.ParentLineID)
			}
		}
		require.Equal(t, 1, bundleCount)
		require.Equal(t, 2, componentCount)
	})

	t.Run("PrepareAndCreateOrder with invalid slot configuration fails", func(t *testing.T) {
		invalidSlotID := nanoid.New()
		input := service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{
				{
					ProductID: bundle.ID,
					Quantity:  1,
					Configuration: map[string]string{
						invalidSlotID: cola.ID,
					},
				},
			},
		}

		_, err := svc.PrepareAndCreateOrder(ctx, input, nil, nil)
		require.Error(t, err)
	})

	t.Run("PrepareAndCreateOrder with invalid option in slot fails", func(t *testing.T) {
		// Try to put fries in the drink slot (not allowed)
		input := service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{
				{
					ProductID: bundle.ID,
					Quantity:  1,
					Configuration: map[string]string{
						drinkSlot.ID: fries.ID, // Fries not allowed in drink slot
					},
				},
			},
		}

		_, err := svc.PrepareAndCreateOrder(ctx, input, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "product not allowed")
	})
}

func TestPaymentService_FindPendingOrderByAttemptID(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)
	cfg := TestConfig()

	svc := NewPaymentSvc(cfg, repos, NewProductSvc(repos))
	ctx := tenantCtx()

	// Setup
	category := fixtures.CreateCategory("Drinks", 1, true)
	cola := fixtures.CreateProduct("Cola", category.ID, 350, product.TypeSimple, nil)
	fixtures.AddInventory(cola.ID, 100, inventoryledger.ReasonOpeningBalance)

	t.Run("FindPendingOrderByAttemptID finds pending order", func(t *testing.T) {
		attemptID := "attempt-123"
		input := service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{
				{ProductID: cola.ID, Quantity: 1},
			},
		}

		prep, err := svc.PrepareAndCreateOrder(ctx, input, nil, &attemptID)
		require.NoError(t, err)

		found, err := svc.FindPendingOrderByAttemptID(ctx, attemptID)
		require.NoError(t, err)
		require.Equal(t, prep.OrderID, found.ID)
	})

	t.Run("FindPendingOrderByAttemptID returns error for non-existent", func(t *testing.T) {
		_, err := svc.FindPendingOrderByAttemptID(ctx, "non-existent-attempt")
		require.Error(t, err)
	})

	t.Run("FindPendingOrderByAttemptID returns error for empty ID", func(t *testing.T) {
		_, err := svc.FindPendingOrderByAttemptID(ctx, "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing attempt id")
	})
}

func TestPaymentService_MarkOrderPaidByPayrexx(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)
	cfg := TestConfig()

	svc := NewPaymentSvc(cfg, repos, NewProductSvc(repos))
	ctx := tenantCtx()

	t.Run("MarkOrderPaidByPayrexx updates order status", func(t *testing.T) {
		orderObj := fixtures.CreateOrder(1000, order.StatusPending, order.OriginShop)
		gatewayID := 12345
		transactionID := 67890
		email := "customer@test.com"

		err := svc.MarkOrderPaidByPayrexx(ctx, orderObj.ID, gatewayID, transactionID, &email)
		require.NoError(t, err)

		updated, err := repos.Order.GetByID(ctx, orderObj.ID)
		require.NoError(t, err)
		require.Equal(t, order.StatusPaid, updated.Status)
		require.NotNil(t, updated.PayrexxGatewayID)
		require.Equal(t, gatewayID, *updated.PayrexxGatewayID)
		require.NotNil(t, updated.PayrexxTransactionID)
		require.Equal(t, transactionID, *updated.PayrexxTransactionID)
		require.NotNil(t, updated.ContactEmail)
		require.Equal(t, email, *updated.ContactEmail)
	})

	t.Run("MarkOrderPaidByPayrexx returns error for non-existent order", func(t *testing.T) {
		err := svc.MarkOrderPaidByPayrexx(ctx, nanoid.New(), 123, 456, nil)
		require.Error(t, err)
	})
}

func TestPaymentService_CleanupPendingOrderByID(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)
	cfg := TestConfig()

	svc := NewPaymentSvc(cfg, repos, NewProductSvc(repos))
	ctx := tenantCtx()

	// Setup
	category := fixtures.CreateCategory("Drinks", 1, true)
	cola := fixtures.CreateProduct("Cola", category.ID, 350, product.TypeSimple, nil)
	fixtures.AddInventory(cola.ID, 100, inventoryledger.ReasonOpeningBalance)

	t.Run("CleanupPendingOrderByID deletes pending order and releases inventory", func(t *testing.T) {
		input := service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{
				{ProductID: cola.ID, Quantity: 5},
			},
		}

		prep, err := svc.PrepareAndCreateOrder(ctx, input, nil, nil)
		require.NoError(t, err)

		// Check inventory was reserved
		stockBefore, err := repos.Inventory.GetCurrentStock(ctx, cola.ID)
		require.NoError(t, err)

		// Cleanup the order
		err = svc.CleanupPendingOrderByID(ctx, prep.OrderID)
		require.NoError(t, err)

		// Order should be deleted
		_, err = repos.Order.GetByID(ctx, prep.OrderID)
		require.Error(t, err)

		// Inventory should be released
		stockAfter, err := repos.Inventory.GetCurrentStock(ctx, cola.ID)
		require.NoError(t, err)
		require.Equal(t, stockBefore+5, stockAfter)
	})

	t.Run("CleanupPendingOrderByID does nothing for paid order", func(t *testing.T) {
		orderObj := fixtures.CreateOrder(1000, order.StatusPaid, order.OriginShop)

		err := svc.CleanupPendingOrderByID(ctx, orderObj.ID)
		require.NoError(t, err)

		// Order should still exist
		found, err := repos.Order.GetByID(ctx, orderObj.ID)
		require.NoError(t, err)
		require.Equal(t, order.StatusPaid, found.Status)
	})
}

func TestPaymentService_TWINTLimits(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)
	cfg := TestConfig()

	svc := NewPaymentSvc(cfg, repos, NewProductSvc(repos))
	ctx := tenantCtx()

	// Setup expensive product
	category := fixtures.CreateCategory("Expensive", 1, true)
	expensive := fixtures.CreateProduct("Expensive Item", category.ID, 600000, product.TypeSimple, nil) // 6000 CHF
	fixtures.AddInventory(expensive.ID, 100, inventoryledger.ReasonOpeningBalance)

	t.Run("PrepareAndCreateOrder fails for single item exceeding TWINT limit", func(t *testing.T) {
		input := service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{
				{ProductID: expensive.ID, Quantity: 1},
			},
		}

		_, err := svc.PrepareAndCreateOrder(ctx, input, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "TWINT max")
	})

	t.Run("PrepareAndCreateOrder fails for total exceeding TWINT limit", func(t *testing.T) {
		affordable := fixtures.CreateProduct("Affordable", category.ID, 100000, product.TypeSimple, nil) // 1000 CHF
		fixtures.AddInventory(affordable.ID, 100, inventoryledger.ReasonOpeningBalance)

		input := service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{
				{ProductID: affordable.ID, Quantity: 6}, // 6000 CHF total
			},
		}

		_, err := svc.PrepareAndCreateOrder(ctx, input, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "TWINT max")
	})
}

func TestPaymentService_DepositCheckout(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)
	cfg := TestConfig()
	svc := NewPaymentSvc(cfg, repos, NewProductSvc(repos))
	keys := NewQRKeySvc(repos)
	ctx := tenantCtx()

	category := fixtures.CreateCategory("Drinks", 1, true)
	kaffee := fixtures.CreateProductWithDeposit("Kaffee", category.ID, 200, 200) // 2.00 CHF + 2.00 CHF deposit
	fixtures.AddInventory(kaffee.ID, 100, inventoryledger.ReasonOpeningBalance)
	wasser := fixtures.CreateProduct("Wasser", category.ID, 150, product.TypeSimple, nil)
	fixtures.AddInventory(wasser.ID, 100, inventoryledger.ReasonOpeningBalance)

	depositAndProductLines := func(t *testing.T, orderID string) (deposit, productLine *ent.OrderLine) {
		t.Helper()
		lines, err := repos.OrderLine.GetByOrderID(ctx, orderID)
		require.NoError(t, err)
		for _, l := range lines {
			switch {
			case l.LineType == orderline.LineTypeDeposit:
				deposit = l
			case l.LineType == orderline.LineTypeSimple && l.ProductID == kaffee.ID:
				productLine = l
			}
		}
		return deposit, productLine
	}

	t.Run("deposit product adds a separate deposit line included in the total", func(t *testing.T) {
		input := service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{{ProductID: kaffee.ID, Quantity: 1}},
		}
		prep, err := svc.PrepareAndCreateOrder(ctx, input, nil, nil)
		require.NoError(t, err)
		require.Equal(t, int64(400), prep.TotalCents) // 200 product + 200 deposit

		deposit, productLine := depositAndProductLines(t, prep.OrderID)
		require.NotNil(t, productLine, "product line present")
		require.NotNil(t, deposit, "deposit line present")
		require.Equal(t, "Depot — Kaffee", deposit.Title)
		require.Equal(t, 1, deposit.Quantity)
		require.Equal(t, int64(200), deposit.UnitPriceCents)
		require.Equal(t, kaffee.ID, deposit.ProductID)
		require.NotNil(t, deposit.ParentLineID)
		require.Equal(t, productLine.ID, *deposit.ParentLineID, "deposit hangs off the product line")

		// The gateway basket carries both lines and sums to the charged total.
		basket := 0
		for _, li := range prep.LineItems {
			basket += li.Amount * li.Quantity
		}
		require.Len(t, prep.LineItems, 2)
		require.Equal(t, int(prep.TotalCents), basket)
	})

	t.Run("deposit scales per unit", func(t *testing.T) {
		input := service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{{ProductID: kaffee.ID, Quantity: 3}},
		}
		prep, err := svc.PrepareAndCreateOrder(ctx, input, nil, nil)
		require.NoError(t, err)
		require.Equal(t, int64(1200), prep.TotalCents) // 3 * (200 + 200)

		deposit, _ := depositAndProductLines(t, prep.OrderID)
		require.NotNil(t, deposit)
		require.Equal(t, 3, deposit.Quantity)
		require.Equal(t, int64(200), deposit.UnitPriceCents) // 6.00 CHF over the line
	})

	t.Run("deposit line is not a redeemable pickup item", func(t *testing.T) {
		input := service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{{ProductID: kaffee.ID, Quantity: 1}},
		}
		prep, err := svc.PrepareAndCreateOrder(ctx, input, nil, nil)
		require.NoError(t, err)

		_, pub, err := keys.EnsureTenantKey(ctx)
		require.NoError(t, err)
		payload, err := qrsign.Verify(pub, prep.QRPayload)
		require.NoError(t, err)

		// Only the handed-over product is redeemable; the deposit never is.
		require.Len(t, payload.Lines, 1)
		require.Equal(t, kaffee.ID, payload.Lines[0].ProductID)
		require.Equal(t, 1, payload.Lines[0].Quantity)
	})

	t.Run("zero-deposit product creates no deposit line", func(t *testing.T) {
		input := service.CreateCheckoutInput{
			Items: []service.CheckoutItemInput{{ProductID: wasser.ID, Quantity: 2}},
		}
		prep, err := svc.PrepareAndCreateOrder(ctx, input, nil, nil)
		require.NoError(t, err)
		require.Equal(t, int64(300), prep.TotalCents) // 2 * 150, no deposit

		lines, err := repos.OrderLine.GetByOrderID(ctx, prep.OrderID)
		require.NoError(t, err)
		for _, l := range lines {
			require.NotEqual(t, orderline.LineTypeDeposit, l.LineType, "no deposit line expected")
		}
	})
}

// TestSettingsRepo_PayrexxStagedFlow exercises the two-stage Payrexx setup at the
// persistence layer: connecting the API leaves the tenant "API connected" (the
// provider path stays unconfigured) until the webhook secret completes it.
func TestSettingsRepo_PayrexxStagedFlow(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	ctx := tenantCtx()

	cfg, err := repos.Settings.GetPayrexxConfig(ctx)
	require.NoError(t, err)
	require.False(t, cfg.HasAPISecret)
	require.False(t, cfg.HasWebhookSecret)

	// Step 1: connect the API — instance + key + token, no webhook secret yet.
	require.NoError(t, repos.Settings.UpsertPayrexxAPICredentials(ctx, "my-instance", "api-enc", "tok-123"))

	cfg, err = repos.Settings.GetPayrexxConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, "my-instance", cfg.Instance)
	require.True(t, cfg.HasAPISecret)
	require.False(t, cfg.HasWebhookSecret)
	require.Equal(t, "tok-123", cfg.WebhookToken)
	require.Nil(t, cfg.ConfiguredAt)

	// The provider/full-creds path stays unconfigured without the webhook secret.
	_, err = repos.Settings.GetPayrexxCredentials(ctx)
	require.ErrorIs(t, err, repository.ErrNotFound)

	// The API-only getter returns the ciphertext used by the test/validate path.
	inst, apiEnc, err := repos.Settings.GetPayrexxAPICredentials(ctx)
	require.NoError(t, err)
	require.Equal(t, "my-instance", inst)
	require.Equal(t, "api-enc", apiEnc)

	// Step 2: set the webhook secret — now fully configured.
	require.NoError(t, repos.Settings.SetPayrexxWebhookSecret(ctx, "wh-enc"))

	cfg, err = repos.Settings.GetPayrexxConfig(ctx)
	require.NoError(t, err)
	require.True(t, cfg.HasWebhookSecret)
	require.NotNil(t, cfg.ConfiguredAt)
	require.Nil(t, cfg.LastWebhookAt)

	creds, err := repos.Settings.GetPayrexxCredentials(ctx)
	require.NoError(t, err)
	require.Equal(t, "my-instance", creds.Instance)
	require.Equal(t, "api-enc", creds.APISecretEnc)
	require.Equal(t, "wh-enc", creds.WebhookSecretEnc)
	require.Equal(t, "tok-123", creds.WebhookToken)

	// Re-connecting the API must not wipe the stored webhook secret.
	require.NoError(t, repos.Settings.UpsertPayrexxAPICredentials(ctx, "my-instance", "api-enc-2", "tok-123"))
	cfg, err = repos.Settings.GetPayrexxConfig(ctx)
	require.NoError(t, err)
	require.True(t, cfg.HasWebhookSecret)

	// Webhook receipt stamp.
	require.NoError(t, repos.Settings.TouchPayrexxWebhookReceived(ctx))
	cfg, err = repos.Settings.GetPayrexxConfig(ctx)
	require.NoError(t, err)
	require.NotNil(t, cfg.LastWebhookAt)
}
