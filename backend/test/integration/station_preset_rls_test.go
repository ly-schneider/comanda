package integration

import (
	"database/sql"
	"testing"

	"backend/internal/plugin/stationqr"
	pgRepo "backend/internal/repository"

	nanoid "backend/internal/id"
	"github.com/stretchr/testify/require"
)

func TestStationEnrollPresetRLS_CrossTenantRejected(t *testing.T) {
	env := newRLSEnv(t)
	tenantA, productsA := env.seedTenantWithProducts(t, "sqp-rls-a-"+randSuffix(), 1)
	tenantB := env.seedTenant(t, "sqp-rls-b-"+randSuffix())

	presetID := nanoid.New()
	env.privExec(t,
		`INSERT INTO station_enroll_preset (id, tenant_id, name, token_enc, issued_by)
		 VALUES ($1, $2, 'Burger', $3, 'user_admin')`,
		presetID, tenantA, []byte("enc-blob-a"),
	)
	env.privExec(t,
		`INSERT INTO station_preset_product (preset_id, product_id, tenant_id)
		 VALUES ($1, $2, $3)`,
		presetID, productsA[0], tenantA,
	)

	t.Run("issuing tenant sees its preset and product", func(t *testing.T) {
		ctx, client, cleanup := env.appRequestTx(t, tenantA)
		defer cleanup()
		repo := stationqr.NewPresetRepository(client)

		presets, err := repo.List(ctx)
		require.NoError(t, err)
		require.Len(t, presets, 1)
		require.Equal(t, "Burger", presets[0].Name)

		links, err := repo.ListProducts(ctx, presetID)
		require.NoError(t, err)
		require.Len(t, links, 1)
		require.Equal(t, productsA[0], links[0].ProductID)
	})

	t.Run("other tenant resolves nothing", func(t *testing.T) {
		ctx, client, cleanup := env.appRequestTx(t, tenantB)
		defer cleanup()
		repo := stationqr.NewPresetRepository(client)

		presets, err := repo.List(ctx)
		require.NoError(t, err)
		require.Empty(t, presets, "cross-tenant preset must be invisible")

		_, err = repo.GetByID(ctx, presetID)
		require.ErrorIs(t, err, pgRepo.ErrNotFound, "cross-tenant preset must not be fetchable by id")

		links, err := repo.ListAllProducts(ctx)
		require.NoError(t, err)
		require.Empty(t, links, "cross-tenant preset products must be invisible")
	})

	t.Run("rows invisible to the other tenant at the DB layer", func(t *testing.T) {
		idB := tenantB
		env.withTenant(t, &idB, func(tx *sql.Tx) {
			var presetCount, productCount int
			require.NoError(t, tx.QueryRow(`SELECT count(*) FROM station_enroll_preset WHERE issued_by = 'user_admin'`).Scan(&presetCount))
			require.Zero(t, presetCount)
			require.NoError(t, tx.QueryRow(`SELECT count(*) FROM station_preset_product WHERE preset_id = $1`, presetID).Scan(&productCount))
			require.Zero(t, productCount)
		})
		idA := tenantA
		env.withTenant(t, &idA, func(tx *sql.Tx) {
			var presetCount, productCount int
			require.NoError(t, tx.QueryRow(`SELECT count(*) FROM station_enroll_preset WHERE issued_by = 'user_admin'`).Scan(&presetCount))
			require.Equal(t, 1, presetCount)
			require.NoError(t, tx.QueryRow(`SELECT count(*) FROM station_preset_product WHERE preset_id = $1`, presetID).Scan(&productCount))
			require.Equal(t, 1, productCount)
		})
	})
}
