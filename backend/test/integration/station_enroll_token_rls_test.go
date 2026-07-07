package integration

import (
	"database/sql"
	"errors"
	"testing"

	"backend/internal/plugin/stationqr"
	pgRepo "backend/internal/repository"

	nanoid "backend/internal/id"
	"github.com/stretchr/testify/require"
)

func TestStationEnrollTokenRLS_CrossTenantRejected(t *testing.T) {
	env := newRLSEnv(t)
	tenantA := env.seedTenant(t, "sqr-rls-a-"+randSuffix())
	tenantB := env.seedTenant(t, "sqr-rls-b-"+randSuffix())

	env.privExec(t,
		`INSERT INTO station_enroll_token (id, tenant_id, token_enc, issued_by)
		 VALUES ($1, $2, $3, 'user_admin')`,
		nanoid.New(), tenantA, []byte("enc-blob-a"),
	)

	t.Run("issuing tenant sees its code", func(t *testing.T) {
		ctx, client, cleanup := env.appRequestTx(t, tenantA)
		defer cleanup()
		repo := stationqr.NewEnrollCodeRepository(client)

		row, err := repo.Get(ctx)
		require.NoError(t, err)
		require.Equal(t, "user_admin", row.IssuedBy)
	})

	t.Run("other tenant cannot resolve the code", func(t *testing.T) {
		ctx, client, cleanup := env.appRequestTx(t, tenantB)
		defer cleanup()
		repo := stationqr.NewEnrollCodeRepository(client)

		_, err := repo.Get(ctx)
		require.Truef(t, errors.Is(err, pgRepo.ErrNotFound), "cross-tenant code must be invisible, got %v", err)
	})

	t.Run("row invisible to the other tenant at the DB layer", func(t *testing.T) {
		idB := tenantB
		env.withTenant(t, &idB, func(tx *sql.Tx) {
			var count int
			require.NoError(t, tx.QueryRow(`SELECT count(*) FROM station_enroll_token WHERE issued_by = 'user_admin'`).Scan(&count))
			require.Zero(t, count)
		})
		idA := tenantA
		env.withTenant(t, &idA, func(tx *sql.Tx) {
			var count int
			require.NoError(t, tx.QueryRow(`SELECT count(*) FROM station_enroll_token WHERE issued_by = 'user_admin'`).Scan(&count))
			require.Equal(t, 1, count)
		})
	})
}
