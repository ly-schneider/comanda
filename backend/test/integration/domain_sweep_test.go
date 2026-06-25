package integration

import (
	"context"
	"testing"
	"time"

	"backend/internal/repository"

	nanoid "backend/internal/id"
	"github.com/stretchr/testify/require"
)

// domainExists reports whether a tenant_domain row with the given id is present.
func (e *rlsEnv) domainExists(t *testing.T, id string) bool {
	t.Helper()
	var n int
	if err := e.priv.QueryRow(`SELECT count(*) FROM tenant_domain WHERE id = $1`, id).Scan(&n); err != nil {
		t.Fatalf("count domain %s: %v", id, err)
	}
	return n > 0
}

// TestDeleteStaleUnverified asserts the sweep drops only BYO custom-domain rows
// that are both unverified and older than the cutoff, and never touches verified
// rows, recent rows, or tokenless platform-assigned rows.
func TestDeleteStaleUnverified(t *testing.T) {
	env := newRLSEnv(t)
	tenant := env.seedTenant(t, "sweep-"+randSuffix())

	old := time.Now().Add(-10 * 24 * time.Hour)
	recent := time.Now().Add(-24 * time.Hour)

	staleID := nanoid.New()     // unverified + old + token -> swept
	recentID := nanoid.New()    // unverified + recent + token -> kept
	verifiedID := nanoid.New()  // verified + old -> kept
	platformID := nanoid.New()  // tokenless (platform-assigned) + old -> kept
	tokenlessID := nanoid.New() // unverified + old but NO token -> kept (guard)

	env.privExec(t,
		`INSERT INTO tenant_domain (id, tenant_id, hostname, verification_token, created_at) VALUES ($1, $2, $3, 'tok', $4)`,
		staleID, tenant, "stale-"+randSuffix()+".comanda.test", old)
	env.privExec(t,
		`INSERT INTO tenant_domain (id, tenant_id, hostname, verification_token, created_at) VALUES ($1, $2, $3, 'tok', $4)`,
		recentID, tenant, "recent-"+randSuffix()+".comanda.test", recent)
	env.privExec(t,
		`INSERT INTO tenant_domain (id, tenant_id, hostname, verification_token, verified_at, created_at) VALUES ($1, $2, $3, 'tok', now(), $4)`,
		verifiedID, tenant, "verified-"+randSuffix()+".comanda.test", old)
	env.privExec(t,
		`INSERT INTO tenant_domain (id, tenant_id, hostname, verified_at, created_at) VALUES ($1, $2, $3, now(), $4)`,
		platformID, tenant, "platform-"+randSuffix()+".comanda.test", old)
	env.privExec(t,
		`INSERT INTO tenant_domain (id, tenant_id, hostname, created_at) VALUES ($1, $2, $3, $4)`,
		tokenlessID, tenant, "tokenless-"+randSuffix()+".comanda.test", old)

	repo := repository.NewTenantDomainRepository(env.privClient())
	n, err := repo.DeleteStaleUnverified(context.Background(), env.privClient(), time.Now().Add(-7*24*time.Hour))
	require.NoError(t, err)
	require.GreaterOrEqual(t, n, 1)

	require.False(t, env.domainExists(t, staleID), "stale unverified BYO domain must be swept")
	require.True(t, env.domainExists(t, recentID), "recent unverified domain must survive")
	require.True(t, env.domainExists(t, verifiedID), "verified domain must survive")
	require.True(t, env.domainExists(t, platformID), "platform-assigned domain must survive")
	require.True(t, env.domainExists(t, tokenlessID), "tokenless unverified row must survive")
}
