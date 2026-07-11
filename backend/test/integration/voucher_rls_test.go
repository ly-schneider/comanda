package integration

import (
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"backend/internal/plugin/artistvoucher"
	pgRepo "backend/internal/repository"

	nanoid "backend/internal/id"
	"github.com/stretchr/testify/require"
)

func seedVoucher(t *testing.T, env *rlsEnv, tenantID, id string) {
	t.Helper()
	cats, err := json.Marshal([]artistvoucher.Category{{ID: "cat_1", Name: "Drinks"}})
	require.NoError(t, err)
	env.privExec(t,
		`INSERT INTO voucher (id, tenant_id, label, valid_from, valid_until, categories, token)
		 VALUES ($1, $2, 'Live Band', $3, $4, $5, 'tok')`,
		id, tenantID, time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour), cats,
	)
}

func TestVoucherRLS_CrossTenantRejected(t *testing.T) {
	env := newRLSEnv(t)
	tenantA := env.seedTenant(t, "vch-rls-a-"+randSuffix())
	tenantB := env.seedTenant(t, "vch-rls-b-"+randSuffix())

	voucherID := nanoid.New()
	seedVoucher(t, env, tenantA, voucherID)

	t.Run("owning tenant reads its voucher", func(t *testing.T) {
		ctx, client, cleanup := env.appRequestTx(t, tenantA)
		defer cleanup()
		repo := artistvoucher.NewVoucherRepository(client)

		row, err := repo.GetByID(ctx, voucherID)
		require.NoError(t, err)
		require.Equal(t, "Live Band", row.Label)

		all, err := repo.List(ctx)
		require.NoError(t, err)
		require.Len(t, all, 1)
	})

	t.Run("other tenant cannot read the voucher", func(t *testing.T) {
		ctx, client, cleanup := env.appRequestTx(t, tenantB)
		defer cleanup()
		repo := artistvoucher.NewVoucherRepository(client)

		_, err := repo.GetByID(ctx, voucherID)
		require.Truef(t, errors.Is(err, pgRepo.ErrNotFound), "cross-tenant voucher must be invisible, got %v", err)

		all, err := repo.List(ctx)
		require.NoError(t, err)
		require.Empty(t, all)
	})

	t.Run("other tenant cannot revoke the voucher", func(t *testing.T) {
		ctx, client, cleanup := env.appRequestTx(t, tenantB)
		defer cleanup()
		repo := artistvoucher.NewVoucherRepository(client)

		_, err := repo.SetRevoked(ctx, voucherID, time.Now())
		require.Truef(t, errors.Is(err, pgRepo.ErrNotFound), "cross-tenant revoke must fail, got %v", err)
	})

	t.Run("row invisible to the other tenant at the DB layer", func(t *testing.T) {
		idB := tenantB
		env.withTenant(t, &idB, func(tx *sql.Tx) {
			var count int
			require.NoError(t, tx.QueryRow(`SELECT count(*) FROM voucher WHERE id = $1`, voucherID).Scan(&count))
			require.Zero(t, count)
		})
		idA := tenantA
		env.withTenant(t, &idA, func(tx *sql.Tx) {
			var count int
			require.NoError(t, tx.QueryRow(`SELECT count(*) FROM voucher WHERE id = $1`, voucherID).Scan(&count))
			require.Equal(t, 1, count)
		})
	})
}
