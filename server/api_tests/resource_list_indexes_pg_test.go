//go:build postgres

package api_tests

import (
	"mahresources/models"
	"testing"

	"github.com/stretchr/testify/require"
)

// The owner index is built CONCURRENTLY on PostgreSQL, and an index left under
// its name on other columns, or by a failed concurrent build, is rebuilt rather
// than trusted.
func TestResourceListIndexesOnPostgres(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	indexDef := func() (bool, string) {
		t.Helper()
		var found []struct {
			Valid bool
			Def   string
		}
		require.NoError(t, tc.DB.Raw(`SELECT i.indisvalid AS valid, pg_get_indexdef(i.indexrelid) AS def
			FROM pg_class c JOIN pg_index i ON i.indexrelid = c.oid
			WHERE c.relname = 'idx_resources_owner_created'`).Scan(&found).Error)
		require.Len(t, found, 1, "idx_resources_owner_created must exist")
		return found[0].Valid, found[0].Def
	}

	require.NoError(t, models.EnsureResourceListIndexes(tc.DB))
	valid, def := indexDef()
	require.True(t, valid)
	require.Contains(t, def, "resources USING btree (owner_id, created_at)")

	require.NoError(t, tc.DB.Exec("DROP INDEX idx_resources_owner_created").Error)
	require.NoError(t, tc.DB.Exec("CREATE INDEX idx_resources_owner_created ON resources (name)").Error)
	require.NoError(t, models.EnsureResourceListIndexes(tc.DB))
	valid, def = indexDef()
	require.True(t, valid)
	require.Contains(t, def, "resources USING btree (owner_id, created_at)")
}
