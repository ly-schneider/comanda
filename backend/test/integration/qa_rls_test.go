package integration

import (
	"database/sql"
	"testing"

	nanoid "backend/internal/id"
	"github.com/stretchr/testify/require"
)

// TestQaTablesPlatformOnlyRLS proves the QA checklist tables are reachable
// exclusively from platform-admin contexts.
func TestQaTablesPlatformOnlyRLS(t *testing.T) {
	env := newRLSEnv(t)

	tenantA := env.seedTenant(t, "qa-a-"+randSuffix())
	runID := nanoid.New()
	env.privExec(t, `INSERT INTO qa_test_run (id, name) VALUES ($1, 'RLS run')`, runID)
	env.privExec(t,
		`INSERT INTO qa_test_result (id, run_id, test_key, status) VALUES ($1, $2, 'smoke.health', 'success')`,
		nanoid.New(), runID)

	t.Run("platform admin reads and writes", func(t *testing.T) {
		env.withPlatform(t, true, func(tx *sql.Tx) {
			require.Equal(t, 1, count(t, tx, `SELECT count(*) FROM qa_test_run WHERE id = $1`, runID))
			require.Equal(t, 1, count(t, tx, `SELECT count(*) FROM qa_test_result WHERE run_id = $1`, runID))
			_, err := tx.Exec(
				`INSERT INTO qa_test_result (id, run_id, test_key, status) VALUES ($1, $2, 'smoke.platform-admin-gate', 'error')`,
				nanoid.New(), runID)
			require.NoError(t, err)
		})
	})

	t.Run("non-admin platform context sees nothing", func(t *testing.T) {
		env.withPlatform(t, false, func(tx *sql.Tx) {
			require.Equal(t, 0, count(t, tx, `SELECT count(*) FROM qa_test_run`))
			require.Equal(t, 0, count(t, tx, `SELECT count(*) FROM qa_test_result`))
			_, err := tx.Exec(`INSERT INTO qa_test_run (id, name) VALUES ($1, 'nope')`, nanoid.New())
			require.Error(t, err, "non-admin insert must violate the platform-only policy")
		})
	})

	t.Run("tenant-bound context sees nothing", func(t *testing.T) {
		env.withTenant(t, &tenantA, func(tx *sql.Tx) {
			require.Equal(t, 0, count(t, tx, `SELECT count(*) FROM qa_test_run`))
			require.Equal(t, 0, count(t, tx, `SELECT count(*) FROM qa_test_result`))
		})
	})
}
