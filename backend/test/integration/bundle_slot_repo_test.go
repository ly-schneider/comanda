package integration

import (
	"context"
	"testing"

	"backend/internal/generated/ent/product"

	nanoid "backend/internal/id"
	"github.com/stretchr/testify/require"
)

func TestBundleSlotRepository_GetByBundleProductIDs(t *testing.T) {
	tdb := NewTestDB(t)
	defer tdb.Close()
	tdb.Cleanup(t)

	repos := NewRepositories(tdb.Client)
	fixtures := NewFixtures(repos)
	ctx := context.Background()

	category := fixtures.CreateCategory("Bundles", 1, true)
	optionCat := fixtures.CreateCategory("Options", 2, true)

	cola := fixtures.CreateProduct("Cola", optionCat.ID, 350, product.TypeSimple, nil)
	fries := fixtures.CreateProduct("Fries", optionCat.ID, 400, product.TypeSimple, nil)

	bundleA := fixtures.CreateProduct("Combo A", category.ID, 1500, product.TypeBundle, nil)
	bundleB := fixtures.CreateProduct("Combo B", category.ID, 1800, product.TypeBundle, nil)
	bundleC := fixtures.CreateProduct("Combo C", category.ID, 2000, product.TypeBundle, nil)

	slotA1 := fixtures.CreateBundleSlot(bundleA.ID, "Drink", 1)
	slotA2 := fixtures.CreateBundleSlot(bundleA.ID, "Side", 2)
	slotB1 := fixtures.CreateBundleSlot(bundleB.ID, "Drink", 1)

	fixtures.CreateBundleSlotOption(slotA1.ID, cola.ID)
	fixtures.CreateBundleSlotOption(slotA2.ID, fries.ID)
	fixtures.CreateBundleSlotOption(slotB1.ID, cola.ID)

	t.Run("returns slots across multiple bundle products", func(t *testing.T) {
		slots, err := repos.BundleSlot.GetByBundleProductIDs(ctx, []string{bundleA.ID, bundleB.ID})
		require.NoError(t, err)
		require.Len(t, slots, 3)

		byBundle := map[string]int{}
		for _, s := range slots {
			byBundle[s.BundleProductID]++
		}
		require.Equal(t, 2, byBundle[bundleA.ID])
		require.Equal(t, 1, byBundle[bundleB.ID])
	})

	t.Run("loads options edge", func(t *testing.T) {
		slots, err := repos.BundleSlot.GetByBundleProductIDs(ctx, []string{bundleA.ID})
		require.NoError(t, err)
		require.Len(t, slots, 2)
		for _, s := range slots {
			require.Len(t, s.Edges.Options, 1, "slot %s should have its option loaded", s.Name)
			require.NotNil(t, s.Edges.Options[0].Edges.OptionProduct)
		}
	})

	t.Run("excludes unrelated bundles", func(t *testing.T) {
		slots, err := repos.BundleSlot.GetByBundleProductIDs(ctx, []string{bundleA.ID})
		require.NoError(t, err)
		for _, s := range slots {
			require.Equal(t, bundleA.ID, s.BundleProductID)
		}
	})

	t.Run("empty input returns empty slice", func(t *testing.T) {
		slots, err := repos.BundleSlot.GetByBundleProductIDs(ctx, nil)
		require.NoError(t, err)
		require.Empty(t, slots)
	})

	t.Run("unknown ids return no rows", func(t *testing.T) {
		slots, err := repos.BundleSlot.GetByBundleProductIDs(ctx, []string{nanoid.New()})
		require.NoError(t, err)
		require.Empty(t, slots)
	})

	t.Run("matches single-id method", func(t *testing.T) {
		batch, err := repos.BundleSlot.GetByBundleProductIDs(ctx, []string{bundleC.ID})
		require.NoError(t, err)
		single, err := repos.BundleSlot.GetByBundleProductID(ctx, bundleC.ID)
		require.NoError(t, err)
		require.Len(t, batch, len(single))
	})
}
