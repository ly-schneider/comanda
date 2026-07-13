package integration

import (
	"context"
	"testing"
	"time"

	"backend/internal/repository"

	nanoid "backend/internal/id"
	"github.com/stretchr/testify/require"
)

// bindingLastSeen reads a binding's last_seen_at via the privileged pool (RLS
// bypassed) so the assertion sees the true persisted value regardless of tenant.
func (e *rlsEnv) bindingLastSeen(t *testing.T, id string) time.Time {
	t.Helper()
	var ts time.Time
	if err := e.priv.QueryRow(
		`SELECT last_seen_at FROM device_binding WHERE id = $1`, id,
	).Scan(&ts); err != nil {
		t.Fatalf("read last_seen_at: %v", err)
	}
	return ts
}

// TestRunInTenantTx_RLSBound exercises the real RunInTenantTx path: it opens an
// app-pool tx, sets the app.tenant_id GUC, and runs the write through the
// RLS-bound Ent client — the exact background flush the debounced last_seen fix
// depends on. It must persist the write for the owning tenant and be denied by
// RLS for any other tenant.
func TestRunInTenantTx_RLSBound(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "ttx-a-"+randSuffix())
	tenantB := env.seedTenant(t, "ttx-b-"+randSuffix())

	// A global user to satisfy the binding's created_by_user_id FK.
	seedUser := nanoid.New()
	env.privExec(t, `INSERT INTO "user" (id) VALUES ($1)`, seedUser)

	// A binding owned by tenant A, seeded with a stale last_seen so an update is
	// observable.
	bindingA := nanoid.New()
	env.privExec(t,
		`INSERT INTO device_binding (id, device_type, token_hash, created_at, last_seen_at, created_by_user_id, tenant_id)
		 VALUES ($1, 'POS', $2, now(), now() - interval '1 hour', $3, $4)`,
		bindingA, "hash-"+randSuffix(), seedUser, tenantA)

	before := env.bindingLastSeen(t, bindingA)

	// bindingRepo relies entirely on the ctx-scoped client RunInTenantTx wires
	// in; the nil fallback must never be reached.
	bindingRepo := repository.NewDeviceBindingRepository(nil)

	t.Run("owning tenant: write lands", func(t *testing.T) {
		err := repository.RunInTenantTx(context.Background(), env.app, tenantA, func(ctx context.Context) error {
			return bindingRepo.UpdateLastSeen(ctx, bindingA)
		})
		require.NoError(t, err)
		require.True(t, env.bindingLastSeen(t, bindingA).After(before),
			"the write must persist (tx committed under the app pool)")
	})

	t.Run("different tenant: RLS denies the write", func(t *testing.T) {
		afterA := env.bindingLastSeen(t, bindingA)

		err := repository.RunInTenantTx(context.Background(), env.app, tenantB, func(ctx context.Context) error {
			return bindingRepo.UpdateLastSeen(ctx, bindingA)
		})
		require.ErrorIs(t, err, repository.ErrNotFound,
			"tenant B's GUC hides tenant A's binding, so the update matches no row")
		require.Equal(t, afterA, env.bindingLastSeen(t, bindingA),
			"tenant B must not modify tenant A's binding")
	})
}
