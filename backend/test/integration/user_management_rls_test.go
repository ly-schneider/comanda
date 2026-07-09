package integration

import (
	"testing"

	"backend/internal/auth"
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
	ctx = auth.WithUserID(ctx, adminA)
	defer cleanup()
	svc := service.NewUserService(client, repository.NewUserRepository(client))

	t.Run("list returns only the bound org's members", func(t *testing.T) {
		users, total, err := svc.List(ctx)
		require.NoError(t, err)
		require.Equal(t, int64(1), total)
		require.Len(t, users, 1)
		require.Equal(t, adminA, users[0].ID)
	})

	t.Run("roles are reported for the bound org's members", func(t *testing.T) {
		roles, err := svc.RolesByUserIDs(ctx, []string{adminA, foreignB})
		require.NoError(t, err)
		require.Equal(t, "owner", roles[adminA])
		_, ok := roles[foreignB]
		require.False(t, ok)
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

	t.Run("role change of a non-member is not found", func(t *testing.T) {
		err := svc.UpdateRole(ctx, foreignB, "staff")
		require.ErrorIs(t, err, repository.ErrNotFound)
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

// TestUserRoleGuards proves the privilege-escalation guards on role changes and
// removals: a member cannot touch themselves, a peer, or a higher role, and can
// only grant a role strictly below their own.
func TestUserRoleGuards(t *testing.T) {
	env := newRLSEnv(t)

	tenant := env.seedTenant(t, "urg-"+randSuffix())
	org := env.seedOrgForTenant(t, tenant, "urg-"+randSuffix())

	owner := env.seedUser(t)
	admin := env.seedUser(t)
	appAdmin := env.seedUser(t)
	staff := env.seedUser(t)
	catalog := env.seedUser(t)
	env.seedMember(t, org, owner, "owner")
	env.seedMember(t, org, admin, "admin")
	env.seedMember(t, org, appAdmin, "app_admin")
	env.seedMember(t, org, staff, "staff")
	env.seedMember(t, org, catalog, "catalog")

	actorCtx := func(actorID string) (service.UserService, func(), func(string) error, func(string, string) error) {
		ctx, client, cleanup := env.appRequestTx(t, tenant)
		ctx = auth.WithUserID(ctx, actorID)
		s := service.NewUserService(client, repository.NewUserRepository(client))
		return s, cleanup,
			func(target string) error { return s.Delete(ctx, target) },
			func(target, role string) error { return s.UpdateRole(ctx, target, role) }
	}

	t.Run("app_admin cannot change its own role", func(t *testing.T) {
		_, cleanup, _, setRole := actorCtx(appAdmin)
		defer cleanup()
		require.ErrorIs(t, setRole(appAdmin, "staff"), service.ErrForbiddenSelf)
	})

	t.Run("app_admin cannot remove itself", func(t *testing.T) {
		_, cleanup, del, _ := actorCtx(appAdmin)
		defer cleanup()
		require.ErrorIs(t, del(appAdmin), service.ErrForbiddenSelf)
	})

	t.Run("app_admin cannot manage a peer app_admin, admin, or owner", func(t *testing.T) {
		peer := env.seedUser(t)
		env.seedMember(t, org, peer, "app_admin")
		_, cleanup, del, setRole := actorCtx(appAdmin)
		defer cleanup()
		require.ErrorIs(t, del(peer), service.ErrForbiddenRole)
		require.ErrorIs(t, del(admin), service.ErrForbiddenRole)
		require.ErrorIs(t, del(owner), service.ErrForbiddenRole)
		require.ErrorIs(t, setRole(admin, "staff"), service.ErrForbiddenRole)
	})

	t.Run("app_admin cannot grant a role at or above its own", func(t *testing.T) {
		_, cleanup, _, setRole := actorCtx(appAdmin)
		defer cleanup()
		require.ErrorIs(t, setRole(staff, "app_admin"), service.ErrForbiddenHigher)
		require.ErrorIs(t, setRole(staff, "admin"), service.ErrForbiddenHigher)
		require.ErrorIs(t, setRole(staff, "owner"), service.ErrForbiddenHigher)
	})

	t.Run("app_admin rejects an unknown role", func(t *testing.T) {
		_, cleanup, _, setRole := actorCtx(appAdmin)
		defer cleanup()
		require.ErrorIs(t, setRole(staff, "superuser"), service.ErrInvalidRole)
	})

	t.Run("app_admin can promote catalog to staff", func(t *testing.T) {
		target := env.seedUser(t)
		env.seedMember(t, org, target, "catalog")
		_, cleanup, _, setRole := actorCtx(appAdmin)
		require.NoError(t, setRole(target, "staff"))
		cleanup()

		var role string
		require.NoError(t, env.priv.QueryRow(
			`SELECT role FROM member WHERE organization_id = $1 AND user_id = $2`, org, target).Scan(&role))
		require.Equal(t, "staff", role)
	})

	t.Run("app_admin can remove a staff member", func(t *testing.T) {
		target := env.seedUser(t)
		env.seedMember(t, org, target, "staff")
		_, cleanup, del, _ := actorCtx(appAdmin)
		require.NoError(t, del(target))
		cleanup()

		var exists bool
		require.NoError(t, env.priv.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM member WHERE organization_id = $1 AND user_id = $2)`, org, target).Scan(&exists))
		require.False(t, exists)
	})

	t.Run("owner can demote an admin", func(t *testing.T) {
		target := env.seedUser(t)
		env.seedMember(t, org, target, "admin")
		_, cleanup, _, setRole := actorCtx(owner)
		require.NoError(t, setRole(target, "staff"))
		cleanup()

		var role string
		require.NoError(t, env.priv.QueryRow(
			`SELECT role FROM member WHERE organization_id = $1 AND user_id = $2`, org, target).Scan(&role))
		require.Equal(t, "staff", role)
	})

	t.Run("owner cannot grant owner", func(t *testing.T) {
		_, cleanup, _, setRole := actorCtx(owner)
		defer cleanup()
		require.ErrorIs(t, setRole(staff, "owner"), service.ErrForbiddenHigher)
	})

	t.Run("member with no actor role in context fails closed", func(t *testing.T) {
		ctx, client, cleanup := env.appRequestTx(t, tenant)
		defer cleanup()
		s := service.NewUserService(client, repository.NewUserRepository(client))
		require.ErrorIs(t, s.Delete(ctx, staff), service.ErrNoActorRole)
	})
}
