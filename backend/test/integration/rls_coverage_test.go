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
