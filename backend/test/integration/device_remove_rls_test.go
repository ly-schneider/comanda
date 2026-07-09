package integration

import (
	"testing"

	"backend/internal/service"

	nanoid "backend/internal/id"
	"github.com/stretchr/testify/require"
)

// deviceExists reports whether a device row exists, via the privileged pool (RLS
// bypassed) so the assertion sees the true state regardless of tenant.
func (e *rlsEnv) deviceExists(t *testing.T, id string) bool {
	t.Helper()
	var exists bool
	if err := e.priv.QueryRow(`SELECT EXISTS(SELECT 1 FROM device WHERE id = $1)`, id).Scan(&exists); err != nil {
		t.Fatalf("device exists check: %v", err)
	}
	return exists
}

// bindingActive reports whether an un-revoked binding row still exists, via the
// privileged pool. After Remove the binding is gone (device_id FK cascades on
// device delete) or revoked — either way it must not be active.
func (e *rlsEnv) bindingActive(t *testing.T, id string) bool {
	t.Helper()
	var active bool
	if err := e.priv.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM device_binding WHERE id = $1 AND revoked_at IS NULL)`, id,
	).Scan(&active); err != nil {
		t.Fatalf("binding active check: %v", err)
	}
	return active
}

// TestDeviceService_Remove_RLSBound exercises Remove through the RLS-bound app
// pool: it must delete the device and revoke its binding only within the owning
// tenant's request tx, and must not reach across tenants.
func TestDeviceService_Remove_RLSBound(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "device-rm-a-"+randSuffix())
	tenantB := env.seedTenant(t, "device-rm-b-"+randSuffix())

	// A global user to satisfy the binding's created_by_user_id FK.
	seedUser := nanoid.New()
	env.privExec(t, `INSERT INTO "user" (id) VALUES ($1)`, seedUser)

	// tenant A: a station and its active binding.
	deviceA := nanoid.New()
	env.privExec(t,
		`INSERT INTO device (id, name, device_key, type, status, tenant_id)
		 VALUES ($1, 'Station A', $2, 'STATION', 'approved', $3)`,
		deviceA, "stn-"+randSuffix(), tenantA)
	bindingA := nanoid.New()
	env.privExec(t,
		`INSERT INTO device_binding (id, device_type, token_hash, created_at, last_seen_at, created_by_user_id, device_id, tenant_id)
		 VALUES ($1, 'STATION', $2, now(), now(), $3, $4, $5)`,
		bindingA, "hash-"+randSuffix(), seedUser, deviceA, tenantA)

	// tenant B: an unrelated device that must survive.
	deviceB := nanoid.New()
	env.privExec(t,
		`INSERT INTO device (id, name, device_key, type, status, tenant_id)
		 VALUES ($1, 'Station B', $2, 'STATION', 'approved', $3)`,
		deviceB, "stn-"+randSuffix(), tenantB)

	// Static, FX-style app-pool client, exactly as production wires the service:
	// the service must reach the request-scoped client via ctx.
	staticClient := env.appEntClient()
	repos := NewRepositories(staticClient)
	svc := service.NewDeviceService(staticClient, repos.Device, repos.DeviceProduct, repos.DeviceBinding, nil)

	t.Run("another tenant cannot remove the device", func(t *testing.T) {
		ctx, _, cleanup := env.appRequestTx(t, tenantB)
		defer cleanup()

		require.Error(t, svc.Remove(ctx, deviceA), "tenant B must not remove tenant A's device")

		require.True(t, env.deviceExists(t, deviceA), "device A must still exist")
		require.True(t, env.bindingActive(t, bindingA), "binding A must still be active")
	})

	t.Run("owning tenant removes the device and cuts off its binding", func(t *testing.T) {
		ctx, _, cleanup := env.appRequestTx(t, tenantA)

		require.NoError(t, svc.Remove(ctx, deviceA))
		cleanup() // commit the request tx before asserting via the privileged pool

		require.False(t, env.deviceExists(t, deviceA), "device A must be deleted")
		require.False(t, env.bindingActive(t, bindingA), "binding A must no longer authorize")
		require.True(t, env.deviceExists(t, deviceB), "tenant B's device must be untouched")
	})
}
