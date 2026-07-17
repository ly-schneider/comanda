package integration

import (
	"context"
	"testing"

	"backend/internal/generated/ent/inventoryledger"
	"backend/internal/generated/ent/product"
	"backend/internal/service"

	"github.com/stretchr/testify/require"
)

func TestProductService_ListProducts(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)

	svc := service.NewProductService(
		repos.Product,
		repos.ProductVariant,
		repos.Category,
		repos.BundleSlot,
		repos.BundleSlotOption,
		repos.Inventory,
		repos.Jeton,
		nil,
		service.NewCatalogCache(),
	)
	ctx := context.Background()

	// Setup test data
	category := fixtures.CreateCategory("Drinks", 1, true)
	jeton := fixtures.CreateJeton("Red", "#EF4444")

	fixtures.CreateProduct("Cola", category.ID, 350, product.TypeSimple, &jeton.ID)
	fixtures.CreateProduct("Sprite", category.ID, 350, product.TypeSimple, &jeton.ID)

	// Add inventory
	fixtures.AddInventory(fixtures.CreateProduct("InvTest", category.ID, 100, product.TypeSimple, nil).ID, 100, inventoryledger.ReasonOpeningBalance)

	t.Run("List all products", func(t *testing.T) {
		products, err := svc.ListProducts(ctx, nil, 50, 0)
		require.NoError(t, err)
		require.Len(t, products, 3)
	})

	t.Run("List products with category filter", func(t *testing.T) {
		// Create another category with products
		otherCat := fixtures.CreateCategory("Food", 2, true)
		fixtures.CreateProduct("Burger", otherCat.ID, 1200, product.TypeSimple, nil)

		catIDStr := category.ID
		products, err := svc.ListProducts(ctx, &catIDStr, 50, 0)
		require.NoError(t, err)
		// Only products from the Drinks category
		for _, p := range products {
			require.Equal(t, category.ID, p.CategoryID)
		}
	})

	t.Run("List products eager-loads jeton edge", func(t *testing.T) {
		products, err := svc.ListProducts(ctx, nil, 50, 0)
		require.NoError(t, err)

		var foundJeton bool
		for _, p := range products {
			if p.JetonID != nil && *p.JetonID == jeton.ID {
				foundJeton = true
				require.NotNil(t, p.Edges.Jeton)
				require.Equal(t, "Red", p.Edges.Jeton.Name)
				require.Equal(t, "#EF4444", p.Edges.Jeton.Color)
			}
		}
		require.True(t, foundJeton, "Expected to find product with jeton")
	})

	t.Run("List with invalid category ID returns error", func(t *testing.T) {
		invalidID := "not-a-uuid"
		_, err := svc.ListProducts(ctx, &invalidID, 50, 0)
		require.Error(t, err)
	})
}

func TestProductService_GetAllVisible(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)

	svc := service.NewProductService(
		repos.Product,
		repos.ProductVariant,
		repos.Category,
		repos.BundleSlot,
		repos.BundleSlotOption,
		repos.Inventory,
		repos.Jeton,
		nil,
		service.NewCatalogCache(),
	)
	ctx := context.Background()

	activeCat := fixtures.CreateCategory("Active", 1, true)
	inactiveCat := fixtures.CreateCategory("Inactive", 2, false)
	inactiveParent := fixtures.CreateCategory("Food", 3, false)
	activeChildOfInactive := fixtures.CreateSubCategory("Burgers", 1, true, inactiveParent.ID)
	activeParent := fixtures.CreateCategory("Drinks", 4, true)
	inactiveChild := fixtures.CreateSubCategory("Juices", 1, false, activeParent.ID)
	activeChild := fixtures.CreateSubCategory("Softdrinks", 2, true, activeParent.ID)

	shown := fixtures.CreateProduct("Shown", activeCat.ID, 350, product.TypeSimple, nil)
	nested := fixtures.CreateProduct("Cola", activeChild.ID, 350, product.TypeSimple, nil)
	fixtures.CreateInactiveProduct("HiddenProduct", activeCat.ID, 350)
	fixtures.CreateProduct("HiddenCategory", inactiveCat.ID, 350, product.TypeSimple, nil)
	fixtures.CreateProduct("HiddenParent", activeChildOfInactive.ID, 900, product.TypeSimple, nil)
	hiddenSubcat := fixtures.CreateProduct("HiddenSubcat", inactiveChild.ID, 350, product.TypeSimple, nil)

	products, err := svc.GetAllVisible(ctx)
	require.NoError(t, err)

	ids := make(map[string]bool, len(products))
	for _, p := range products {
		ids[p.ID] = true
	}
	require.Len(t, products, 2)
	require.True(t, ids[shown.ID], "active product in active category should be visible")
	require.True(t, ids[nested.ID], "product under active child of active parent should be visible")
	require.False(t, ids[hiddenSubcat.ID], "product under an inactive subcategory should be hidden")
}

func TestProductService_BundleProducts(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)

	svc := service.NewProductService(
		repos.Product,
		repos.ProductVariant,
		repos.Category,
		repos.BundleSlot,
		repos.BundleSlotOption,
		repos.Inventory,
		repos.Jeton,
		nil,
		service.NewCatalogCache(),
	)
	ctx := context.Background()

	// Setup test data
	category := fixtures.CreateCategory("Bundles", 1, true)

	// Create simple products for bundle options
	optionCat := fixtures.CreateCategory("Options", 2, true)
	cola := fixtures.CreateProduct("Cola", optionCat.ID, 350, product.TypeSimple, nil)
	sprite := fixtures.CreateProduct("Sprite", optionCat.ID, 350, product.TypeSimple, nil)
	fries := fixtures.CreateProduct("Fries", optionCat.ID, 400, product.TypeSimple, nil)

	fixtures.AddInventory(cola.ID, 50, inventoryledger.ReasonOpeningBalance)
	fixtures.AddInventory(sprite.ID, 50, inventoryledger.ReasonOpeningBalance)
	fixtures.AddInventory(fries.ID, 30, inventoryledger.ReasonOpeningBalance)

	// Create bundle product
	bundle := fixtures.CreateProduct("Combo Menu", category.ID, 1500, product.TypeBundle, nil)

	// Create bundle slots
	drinkSlot := fixtures.CreateBundleSlot(bundle.ID, "Drink", 1)
	sideSlot := fixtures.CreateBundleSlot(bundle.ID, "Side", 2)

	// Add options to slots
	fixtures.CreateBundleSlotOption(drinkSlot.ID, cola.ID)
	fixtures.CreateBundleSlotOption(drinkSlot.ID, sprite.ID)
	fixtures.CreateBundleSlotOption(sideSlot.ID, fries.ID)

	t.Run("GetBundles returns bundle products", func(t *testing.T) {
		bundles, err := svc.GetBundles(ctx)
		require.NoError(t, err)
		require.Len(t, bundles, 1)
		require.Equal(t, "Combo Menu", bundles[0].Name)
		require.Equal(t, product.TypeBundle, bundles[0].Type)
	})

	t.Run("Bundle slot operations", func(t *testing.T) {
		// Create a new slot
		newSlot, err := svc.CreateBundleSlot(ctx, bundle.ID, "Dessert")
		require.NoError(t, err)
		require.Equal(t, "Dessert", newSlot.Name)

		// Update the slot
		updated, err := svc.UpdateBundleSlot(ctx, bundle.ID, newSlot.ID, "Sweet Treat")
		require.NoError(t, err)
		require.Equal(t, "Sweet Treat", updated.Name)

		// Delete the slot
		err = svc.DeleteBundleSlot(ctx, bundle.ID, newSlot.ID)
		require.NoError(t, err)
	})
}

func TestProductService_EmptyResults(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)

	svc := service.NewProductService(
		repos.Product,
		repos.ProductVariant,
		repos.Category,
		repos.BundleSlot,
		repos.BundleSlotOption,
		repos.Inventory,
		repos.Jeton,
		nil,
		service.NewCatalogCache(),
	)
	ctx := context.Background()

	t.Run("List products returns empty for no products", func(t *testing.T) {
		products, err := svc.ListProducts(ctx, nil, 50, 0)
		require.NoError(t, err)
		require.Empty(t, products)
	})
}
