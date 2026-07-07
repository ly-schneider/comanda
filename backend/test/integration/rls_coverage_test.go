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

// globalPermissiveTables are auth-plane tables whose RLS is enabled for
// uniformity but permissive (USING true). Isolation for these is the
// privileged-role split + app routing, not tenant_id/platform-admin. The Better
// Auth organization-plugin tables (organization/member/invitation) are NOT here:
// once the org↔tenant link exists they carry org-keyed policies (see
// orgScopedRLSTables / TestOrgScopedRLSPolicies).
var globalPermissiveTables = []string{
	"user", "session", "account", "verification", "jwks", "api_key",
}

// TestRLSGlobalPermissive asserts the auth-plane tables carry a permissive
// USING (true) policy and do NOT gate on tenant_id / app_is_platform_admin().
// This pins the classification so a future change can't silently turn one into
// a (broken) tenant-scoped policy or vice versa.
func TestRLSGlobalPermissive(t *testing.T) {
	env := newRLSEnv(t)

	for _, table := range globalPermissiveTables {
		var rowSecurity bool
		err := env.priv.QueryRow(
			`SELECT relrowsecurity FROM pg_class c
			 JOIN pg_namespace n ON n.oid = c.relnamespace
			 WHERE n.nspname = 'public' AND c.relname = $1`, table,
		).Scan(&rowSecurity)
		require.NoErrorf(t, err, "table %q not found", table)
		require.Truef(t, rowSecurity, "table %q must have RLS enabled", table)

		rows, err := env.priv.Query(
			`SELECT qual, with_check FROM pg_policies
			 WHERE schemaname = 'public' AND tablename = $1`, table,
		)
		require.NoError(t, err)

		var policyCount int
		for rows.Next() {
			policyCount++
			var qual, withCheck *string
			require.NoError(t, rows.Scan(&qual, &withCheck))
			for _, clause := range []*string{qual, withCheck} {
				if clause == nil {
					continue
				}
				require.NotContainsf(t, *clause, "app_current_tenant_id()",
					"global table %q must not gate on tenant_id", table)
				require.NotContainsf(t, *clause, "app_is_platform_admin()",
					"global table %q must not gate on platform-admin", table)
			}
		}
		require.NoError(t, rows.Err())
		_ = rows.Close()
		require.GreaterOrEqualf(t, policyCount, 1, "global table %q must have a policy", table)
	}
}

// orgScopedRLSTables are the Better Auth org-plugin tables. Their isolation
// policy keys on the current tenant's organization (app_current_org_id()) with
// the platform-admin bypass — distinct from the tenant_id-keyed scoped tables.
var orgScopedRLSTables = []string{"organization", "member", "invitation"}

// TestOrgScopedRLSPolicies asserts each org-plugin table carries an _isolation
// policy gated on app_current_org_id() with the platform-admin bypass on both
// USING and WITH CHECK. This pins the hardened classification so a regression
// back to USING(true) (or a wrong key) fails loudly.
func TestOrgScopedRLSPolicies(t *testing.T) {
	env := newRLSEnv(t)

	for _, table := range orgScopedRLSTables {
		var qual, withCheck *string
		err := env.priv.QueryRow(`
			SELECT qual, with_check
			FROM pg_policies
			WHERE schemaname = 'public' AND tablename = $1 AND policyname = $2
		`, table, table+"_isolation").Scan(&qual, &withCheck)
		require.NoErrorf(t, err, "policy %s_isolation not found", table)

		for _, clause := range []*string{qual, withCheck} {
			require.NotNilf(t, clause, "policy %s_isolation missing a USING/WITH CHECK clause", table)
			require.Containsf(t, *clause, "app_current_org_id()",
				"policy %s_isolation must key on the current org", table)
			require.Containsf(t, *clause, "app_is_platform_admin()",
				"policy %s_isolation must include the platform-admin bypass", table)
		}
	}
}

// platformOnlyRLSTables are platform-plane tables (no tenant_id) whose every
// access requires app_is_platform_admin().
var platformOnlyRLSTables = []string{"qa_test_run", "qa_test_result"}

// TestRLSPlatformOnlyPolicies pins the platform-only classification: gate
// solely on app_is_platform_admin(), never on a tenant leg.
func TestRLSPlatformOnlyPolicies(t *testing.T) {
	env := newRLSEnv(t)

	for _, table := range platformOnlyRLSTables {
		var qual, withCheck *string
		err := env.priv.QueryRow(`
			SELECT qual, with_check
			FROM pg_policies
			WHERE schemaname = 'public' AND tablename = $1 AND policyname = $2
		`, table, table+"_platform_only").Scan(&qual, &withCheck)
		require.NoErrorf(t, err, "policy %s_platform_only not found", table)

		for _, clause := range []*string{qual, withCheck} {
			require.NotNilf(t, clause, "policy %s_platform_only missing a USING/WITH CHECK clause", table)
			require.Containsf(t, *clause, "app_is_platform_admin()",
				"policy %s_platform_only must gate on platform-admin", table)
			require.NotContainsf(t, *clause, "app_current_tenant_id()",
				"platform-only table %q must not gate on tenant_id", table)
		}
	}
}

// scopedRLSTables are the tenant-scoped tables whose isolation policy must carry
// the platform-admin bypass (OR app_is_platform_admin()). Mirrors the scoped
// array in the migrations plus the tenant root and tenant_domain.
var scopedRLSTables = []string{
	"product", "category", "jeton", "menu_slot", "menu_slot_option", "order",
	"order_line", "order_payment", "order_line_redemption", "inventory_ledger",
	"club100_redemption", "settings", "idempotency",
	"device", "device_binding", "device_product", "admin_invite",
	"volunteer_campaign", "volunteer_campaign_product", "volunteer_redemption",
	"tenant", "tenant_domain", "refund", "refund_line", "product_variant",
	"tenant_plugin",
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
