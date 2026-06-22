package integration

import (
	"testing"

	"backend/internal/config"
	"backend/internal/generated/ent/tenantdomain"
	"backend/internal/repository"
	"backend/internal/service"

	"github.com/stretchr/testify/require"
)

// TestPlatformAdminProvisionsTenant exercises the P7 tenant-CRUD path end to end
// against RLS: a platform-admin context creates a tenant plus its settings
// singleton and verified primary domain in one tx, then reads them back.
func TestPlatformAdminProvisionsTenant(t *testing.T) {
	env := newRLSEnv(t)
	ctx, client, cleanup := env.appPlatformTx(t, true)
	defer cleanup()

	repo := repository.NewTenantRepository(client)
	svc := service.NewTenantService(repo, config.Config{
		CustomDomain: config.CustomDomainConfig{BaseDomain: "comanda.leys.ch"},
	})

	slug := "prov-" + randSuffix()
	created, err := svc.Create(ctx, slug, "Provisioned Co")
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)
	require.Equal(t, slug, created.Slug)

	// Settings singleton seeded (PK = tenant id, neutral white-base defaults).
	settings, err := client.Settings.Get(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, settings.TenantID)
	require.Equal(t, "#0a0a0a", settings.ColorPrimary)
	require.Equal(t, "inter", settings.FontKey)

	// Primary domain seeded, verified, platform-owned (no verification token).
	dom, err := client.TenantDomain.Query().
		Where(tenantdomain.HostnameEQ(slug + ".comanda.leys.ch")).
		Only(ctx)
	require.NoError(t, err)
	require.True(t, dom.IsPrimary)
	require.NotNil(t, dom.VerifiedAt)
	require.Empty(t, dom.VerificationToken)
	require.Equal(t, created.ID, dom.TenantID)

	// The platform admin sees the new tenant in the cross-tenant list.
	list, err := svc.List(ctx)
	require.NoError(t, err)
	var found bool
	for _, tn := range list {
		if tn.ID == created.ID {
			found = true
		}
	}
	require.True(t, found, "created tenant must appear in the platform list")
}

// TestNonPlatformAdminCannotCreateTenant proves the RLS WITH CHECK boundary: a
// no-tenant context without the platform-admin GUC cannot insert a tenant row.
func TestNonPlatformAdminCannotCreateTenant(t *testing.T) {
	env := newRLSEnv(t)
	ctx, client, cleanup := env.appPlatformTx(t, false)
	defer cleanup()

	repo := repository.NewTenantRepository(client)
	svc := service.NewTenantService(repo, config.Config{})

	_, err := svc.Create(ctx, "deny-"+randSuffix(), "Denied Co")
	require.Error(t, err, "non platform-admin must not be able to create a tenant")
}
