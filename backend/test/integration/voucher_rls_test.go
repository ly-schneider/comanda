package integration

import (
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"backend/internal/generated/ent/voucher"
	"backend/internal/plugin/artistvoucher"
	pgRepo "backend/internal/repository"

	nanoid "backend/internal/id"
	"github.com/stretchr/testify/require"
)

func seedVoucher(t *testing.T, env *rlsEnv, tenantID, id string) {
	seedVoucherUntil(t, env, tenantID, id, time.Now().Add(24*time.Hour))
}

func seedVoucherUntil(t *testing.T, env *rlsEnv, tenantID, id string, validUntil time.Time) {
	t.Helper()
	cats, err := json.Marshal([]artistvoucher.Category{{ID: "cat_1", Name: "Drinks"}})
	require.NoError(t, err)
	env.privExec(t,
		`INSERT INTO voucher (id, tenant_id, label, valid_from, valid_until, categories, token)
		 VALUES ($1, $2, 'Live Band', $3, $4, $5, 'tok')`,
		id, tenantID, time.Now().Add(-2*time.Hour), validUntil, cats,
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

		all, err := repo.ListByKind(ctx, voucher.KindPass)
		require.NoError(t, err)
		require.Len(t, all, 1)
	})

	t.Run("other tenant cannot read the voucher", func(t *testing.T) {
		ctx, client, cleanup := env.appRequestTx(t, tenantB)
		defer cleanup()
		repo := artistvoucher.NewVoucherRepository(client)

		_, err := repo.GetByID(ctx, voucherID)
		require.Truef(t, errors.Is(err, pgRepo.ErrNotFound), "cross-tenant voucher must be invisible, got %v", err)

		all, err := repo.ListByKind(ctx, voucher.KindPass)
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

	t.Run("other tenant cannot delete the voucher", func(t *testing.T) {
		ctx, client, cleanup := env.appRequestTxRollback(t, tenantB)
		defer cleanup()
		repo := artistvoucher.NewVoucherRepository(client)

		err := repo.Delete(ctx, voucherID)
		require.Truef(t, errors.Is(err, pgRepo.ErrNotFound), "cross-tenant delete must fail, got %v", err)

		idA := tenantA
		env.withTenant(t, &idA, func(tx *sql.Tx) {
			var count int
			require.NoError(t, tx.QueryRow(`SELECT count(*) FROM voucher WHERE id = $1`, voucherID).Scan(&count))
			require.Equal(t, 1, count, "owner's row must survive a cross-tenant delete")
		})
	})
}

func TestVoucherService_DeleteExpiryGate(t *testing.T) {
	env := newRLSEnv(t)
	tenantID := env.seedTenant(t, "vch-del-"+randSuffix())

	liveID := nanoid.New()
	expiredID := nanoid.New()
	seedVoucherUntil(t, env, tenantID, liveID, time.Now().Add(time.Hour))
	seedVoucherUntil(t, env, tenantID, expiredID, time.Now().Add(-time.Hour))

	ctx, client, cleanup := env.appRequestTx(t, tenantID)
	defer cleanup()
	svc := artistvoucher.NewService(artistvoucher.NewVoucherRepository(client), nil, nil, nil, nil, nil, nil)

	err := svc.Delete(ctx, liveID)
	require.Truef(t, errors.Is(err, artistvoucher.ErrNotExpired), "live voucher must not be deletable, got %v", err)
	_, err = svc.Get(ctx, liveID)
	require.NoError(t, err, "live voucher must survive a rejected delete")

	require.NoError(t, svc.Delete(ctx, expiredID), "expired voucher must delete")
	_, err = svc.Get(ctx, expiredID)
	require.Truef(t, errors.Is(err, pgRepo.ErrNotFound), "expired voucher must be gone, got %v", err)
}
