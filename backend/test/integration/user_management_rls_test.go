package integration

import (
	"testing"

	"backend/internal/repository"
	"backend/internal/service"

	"github.com/stretchr/testify/require"
)

// TestUserManagementOrgScoped proves the /v1/users surface is confined to the
// bound tenant's organization members. The user table is global (RLS USING
// true), so without the service-level membership gate a tenant admin could read
// or mutate any user platform-wide by ID.
func TestUserManagementOrgScoped(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "umrls-a-"+randSuffix())
	tenantB := env.seedTenant(t, "umrls-b-"+randSuffix())
	orgA := env.seedOrgForTenant(t, tenantA, "umrls-a-"+randSuffix())
	orgB := env.seedOrgForTenant(t, tenantB, "umrls-b-"+randSuffix())

	adminA := env.seedUser(t) // member (owner) of org A
	foreignB := env.seedUser(t)
	env.seedMember(t, orgA, adminA, "owner")
	env.seedMember(t, orgB, foreignB, "owner")

	ctx, client, cleanup := env.appRequestTx(t, tenantA)
	defer cleanup()
	svc := service.NewUserService(client, repository.NewUserRepository(client))

	t.Run("list returns only the bound org's members", func(t *testing.T) {
		users, total, err := svc.List(ctx)
		require.NoError(t, err)
		require.Equal(t, int64(1), total)
		require.Len(t, users, 1)
		require.Equal(t, adminA, users[0].ID)
	})

	t.Run("get of a non-member is not found", func(t *testing.T) {
		_, err := svc.GetByID(ctx, foreignB)
		require.ErrorIs(t, err, repository.ErrNotFound)
	})

	t.Run("get of an own member succeeds", func(t *testing.T) {
		u, err := svc.GetByID(ctx, adminA)
		require.NoError(t, err)
		require.Equal(t, adminA, u.ID)
	})

	t.Run("rename of a non-member is refused and does not write", func(t *testing.T) {
		err := svc.UpdateName(ctx, foreignB, "hijacked")
		require.ErrorIs(t, err, repository.ErrNotFound)

		var name *string
		require.NoError(t, env.priv.QueryRow(
			`SELECT name FROM "user" WHERE id = $1`, foreignB).Scan(&name))
		require.Nil(t, name)
	})

	t.Run("delete of a non-member is refused and does not delete", func(t *testing.T) {
		err := svc.Delete(ctx, foreignB)
		require.ErrorIs(t, err, repository.ErrNotFound)

		var exists bool
		require.NoError(t, env.priv.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM "user" WHERE id = $1)`, foreignB).Scan(&exists))
		require.True(t, exists)
	})

	t.Run("delete of an own member removes the membership but keeps the global user", func(t *testing.T) {
		staffA := env.seedUser(t)
		env.seedMember(t, orgA, staffA, "staff")

		_, total, err := svc.List(ctx)
		require.NoError(t, err)
		require.Equal(t, int64(2), total)

		require.NoError(t, svc.Delete(ctx, staffA))

		_, total, err = svc.List(ctx)
		require.NoError(t, err)
		require.Equal(t, int64(1), total)

		tx, ok := repository.RequestTxFromContext(ctx)
		require.True(t, ok)
		var userExists bool
		require.NoError(t, tx.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM "user" WHERE id = $1)`, staffA).Scan(&userExists))
		require.True(t, userExists)
	})
}
