package integration

import (
	"database/sql"
	"testing"

	nanoid "backend/internal/id"
	"github.com/stretchr/testify/require"
)

// TestRLSIsolation proves cross-tenant isolation on representative tables under
// the RLS-bound app role: a tenant sees only its own rows, cannot read/update/
// delete another tenant's rows, cannot insert into another tenant, and sees
// nothing with no tenant set (fail-closed).
func TestRLSIsolation(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "rls-a-"+randSuffix())
	tenantB := env.seedTenant(t, "rls-b-"+randSuffix())

	// Seed one category per tenant via the privileged pool.
	catA := nanoid.New()
	catB := nanoid.New()
	env.privExec(t, `INSERT INTO category (id, name, tenant_id) VALUES ($1, 'A', $2)`, catA, tenantA)
	env.privExec(t, `INSERT INTO category (id, name, tenant_id) VALUES ($1, 'B', $2)`, catB, tenantB)

	t.Run("SELECT sees only own tenant", func(t *testing.T) {
		env.withTenant(t, &tenantA, func(tx *sql.Tx) {
			names := scanNames(t, tx, `SELECT name FROM category ORDER BY name`)
			require.Equal(t, []string{"A"}, names)
		})
		env.withTenant(t, &tenantB, func(tx *sql.Tx) {
			names := scanNames(t, tx, `SELECT name FROM category ORDER BY name`)
			require.Equal(t, []string{"B"}, names)
		})
	})

	t.Run("UPDATE of other tenant affects 0 rows", func(t *testing.T) {
		env.withTenant(t, &tenantA, func(tx *sql.Tx) {
			res, err := tx.Exec(`UPDATE category SET name = 'hacked' WHERE id = $1`, catB)
			require.NoError(t, err)
			require.Equal(t, int64(0), rowsAffected(t, res))
		})
		// Confirm B's row is untouched.
		env.withTenant(t, &tenantB, func(tx *sql.Tx) {
			names := scanNames(t, tx, `SELECT name FROM category`)
			require.Equal(t, []string{"B"}, names)
		})
	})

	t.Run("DELETE of other tenant affects 0 rows", func(t *testing.T) {
		env.withTenant(t, &tenantA, func(tx *sql.Tx) {
			res, err := tx.Exec(`DELETE FROM category WHERE id = $1`, catB)
			require.NoError(t, err)
			require.Equal(t, int64(0), rowsAffected(t, res))
		})
	})

	t.Run("INSERT with other tenant id blocked by WITH CHECK", func(t *testing.T) {
		env.withTenant(t, &tenantA, func(tx *sql.Tx) {
			_, err := tx.Exec(
				`INSERT INTO category (id, name, tenant_id) VALUES ($1, 'evil', $2)`,
				nanoid.New(), tenantB,
			)
			require.Error(t, err, "WITH CHECK must reject inserting another tenant's row")
		})
	})

	t.Run("no tenant set is fail-closed", func(t *testing.T) {
		env.withTenant(t, nil, func(tx *sql.Tx) {
			names := scanNames(t, tx, `SELECT name FROM category`)
			require.Empty(t, names, "unset tenant GUC must return zero rows")
		})
	})

	t.Run("order/settings/idempotency isolation", func(t *testing.T) {
		// order
		ordA := nanoid.New()
		ordB := nanoid.New()
		env.privExec(t, `INSERT INTO "order" (id, total_cents, status, origin, tenant_id) VALUES ($1, 100, 'paid', 'pos', $2)`, ordA, tenantA)
		env.privExec(t, `INSERT INTO "order" (id, total_cents, status, origin, tenant_id) VALUES ($1, 200, 'paid', 'pos', $2)`, ordB, tenantB)

		// settings (one row per tenant; id = tenant uuid string)
		env.privExec(t, `INSERT INTO settings (id, tenant_id, sumup_merchant_code) VALUES ($1, $2, 'SUMUP-A')`, tenantA, tenantA)
		env.privExec(t, `INSERT INTO settings (id, tenant_id, sumup_merchant_code) VALUES ($1, $2, 'SUMUP-B')`, tenantB, tenantB)

		// idempotency: same (scope,key) under each tenant — re-scoped uniqueness
		// means this no longer collides.
		env.privExec(t, `INSERT INTO idempotency (id, scope, key, expires_at, tenant_id) VALUES ($1, 'pay', 'k1', now()+interval '1 day', $2)`, nanoid.New(), tenantA)
		env.privExec(t, `INSERT INTO idempotency (id, scope, key, expires_at, tenant_id) VALUES ($1, 'pay', 'k1', now()+interval '1 day', $2)`, nanoid.New(), tenantB)

		env.withTenant(t, &tenantA, func(tx *sql.Tx) {
			require.Equal(t, 1, count(t, tx, `SELECT count(*) FROM "order"`))
			require.Equal(t, 1, count(t, tx, `SELECT count(*) FROM settings`))
			require.Equal(t, 1, count(t, tx, `SELECT count(*) FROM idempotency`))
			require.Equal(t, 0, count(t, tx, `SELECT count(*) FROM "order" WHERE id = $1`, ordB))
			require.Equal(t, []string{"SUMUP-A"}, scanNames(t, tx, `SELECT sumup_merchant_code FROM settings`))
		})
	})
}

func scanNames(t *testing.T, tx *sql.Tx, query string, args ...any) []string {
	t.Helper()
	rows, err := tx.Query(query, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		out = append(out, s)
	}
	require.NoError(t, rows.Err())
	return out
}

func count(t *testing.T, tx *sql.Tx, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, tx.QueryRow(query, args...).Scan(&n))
	return n
}

func rowsAffected(t *testing.T, res sql.Result) int64 {
	t.Helper()
	n, err := res.RowsAffected()
	require.NoError(t, err)
	return n
}
