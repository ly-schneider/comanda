package integration

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// allowlistNoRLS lists tables intentionally exempt from the RLS coverage
// assertion. The 6 global auth tables carry permissive policies (so they DO
// have rowsecurity + a policy and are not actually exempt); the only true
// exemption is Atlas's own bookkeeping table.
var allowlistNoRLS = map[string]struct{}{
	"atlas_schema_revisions": {},
}

// TestRLSCoverage asserts every public table (minus the allowlist) has RLS
// enabled and at least one policy. A new tenant-scoped table added without a
// policy fails here — the guard against silent leaks.
func TestRLSCoverage(t *testing.T) {
	env := newRLSEnv(t)

	rows, err := env.priv.Query(`
		SELECT c.relname,
		       c.relrowsecurity,
		       (SELECT count(*) FROM pg_policies p WHERE p.schemaname = 'public' AND p.tablename = c.relname)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'r'
		ORDER BY c.relname
	`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	var checked int
	for rows.Next() {
		var name string
		var rowSecurity bool
		var policies int
		require.NoError(t, rows.Scan(&name, &rowSecurity, &policies))

		if _, ok := allowlistNoRLS[name]; ok {
			continue
		}
		checked++
		require.Truef(t, rowSecurity, "table %q has no ROW LEVEL SECURITY enabled", name)
		require.GreaterOrEqualf(t, policies, 1, "table %q has RLS enabled but no policy", name)
	}
	require.NoError(t, rows.Err())
	require.Greater(t, checked, 20, "expected to verify RLS on >20 tables; schema may not be migrated")
}

// scopedRLSTables are the tenant-scoped tables whose isolation policy must carry
// the platform-admin bypass (OR app_is_platform_admin()). Mirrors the scoped
// array in the migrations plus the tenant root and tenant_domain.
var scopedRLSTables = []string{
	"product", "category", "jeton", "menu_slot", "menu_slot_option", "order",
	"order_line", "order_payment", "order_line_redemption", "inventory_ledger",
	"club100_redemption", "club100_free_product", "settings", "idempotency",
	"device", "device_binding", "device_product", "admin_invite",
	"volunteer_campaign", "volunteer_campaign_product", "volunteer_redemption",
	"tenant", "tenant_domain",
}

// TestRLSPlatformAdminBypass asserts every tenant-scoped isolation policy grants
// platform admins a cross-tenant bypass on both USING (qual) and WITH CHECK.
// Without this, a platform admin could not read across tenants; its presence is
// the DB-layer half of the two-gate platform-admin design.
func TestRLSPlatformAdminBypass(t *testing.T) {
	env := newRLSEnv(t)

	for _, table := range scopedRLSTables {
		var qual, withCheck *string
		err := env.priv.QueryRow(`
			SELECT qual, with_check
			FROM pg_policies
			WHERE schemaname = 'public' AND tablename = $1 AND policyname = $2
		`, table, table+"_isolation").Scan(&qual, &withCheck)
		require.NoErrorf(t, err, "policy %s_isolation not found", table)

		require.NotNilf(t, qual, "policy %s_isolation has no USING clause", table)
		require.Containsf(t, *qual, "app_is_platform_admin()",
			"policy %s_isolation USING must include the platform-admin bypass", table)

		require.NotNilf(t, withCheck, "policy %s_isolation has no WITH CHECK clause", table)
		require.Containsf(t, *withCheck, "app_is_platform_admin()",
			"policy %s_isolation WITH CHECK must include the platform-admin bypass", table)
	}
}
